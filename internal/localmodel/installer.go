// Package localmodel installs and adopts local inference runtimes and model
// weights managed by Conduit.
package localmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	DefaultRootName = ".conduit"
	ModelsDirName   = "models"
)

// RuntimeKind names the supported local inference runtime families.
type RuntimeKind string

const (
	RuntimeOllama   RuntimeKind = "ollama"
	RuntimeLlamaCPP RuntimeKind = "llama.cpp"
	RuntimeLMStudio RuntimeKind = "lm_studio"
)

// Artifact is a downloadable file with an expected SHA-256 checksum.
type Artifact struct {
	Name   string
	URL    string
	SHA256 string
}

// RuntimeSpec describes the runtime Conduit should adopt or install.
type RuntimeSpec struct {
	Name        string
	Kind        RuntimeKind
	BinaryNames []string
	Artifact    Artifact
}

// ModelSpec describes one model-weight artifact managed by Conduit.
type ModelSpec struct {
	Name     string
	Runtime  RuntimeKind
	Artifact Artifact
}

// InstallSpec is the full local inference setup plan.
type InstallSpec struct {
	Runtime RuntimeSpec
	Models  []ModelSpec
}

// Event reports background installation progress to callers such as the TUI.
type Event struct {
	Stage string
	Name  string
	Path  string
	Bytes int64
	Total int64
}

// Result summarizes what the installer did.
type Result struct {
	ManagedRoot         string
	RuntimePath         string
	RuntimeAdopted      bool
	DownloadedArtifacts []string
	VerifiedArtifacts   []string
}

// Installer owns the local model installation pipeline.
type Installer struct {
	HomeDir  string
	RootDir  string
	Client   *http.Client
	LookPath func(string) (string, error)
	Progress func(Event)
}

// DefaultRoot returns ~/.conduit for the supplied home directory.
func DefaultRoot(home string) string {
	return filepath.Join(home, DefaultRootName)
}

// ManagedModelsDir returns the canonical Conduit-managed model directory.
func ManagedModelsDir(root string) string {
	return filepath.Join(root, ModelsDirName)
}

// Install installs or adopts a runtime and downloads model weights under
// ~/.conduit/models. It is resumable when a previous .part file exists.
func (i Installer) Install(ctx context.Context, spec InstallSpec) (Result, error) {
	if spec.Runtime.Name == "" {
		return Result{}, errors.New("local model install requires a runtime name")
	}
	root, err := i.root()
	if err != nil {
		return Result{}, err
	}
	result := Result{ManagedRoot: root}

	if err := os.MkdirAll(ManagedModelsDir(root), 0o700); err != nil {
		return Result{}, fmt.Errorf("creating managed models dir: %w", err)
	}

	if path, ok := i.detectRuntime(spec.Runtime); ok {
		result.RuntimePath = path
		result.RuntimeAdopted = true
		i.emit(Event{Stage: "runtime_adopted", Name: spec.Runtime.Name, Path: path})
	} else if spec.Runtime.Artifact.URL != "" {
		path, downloaded, err := i.downloadAndVerify(ctx, spec.Runtime.Artifact, filepath.Join(root, "runtimes", spec.Runtime.Name))
		if err != nil {
			return Result{}, fmt.Errorf("installing runtime %s: %w", spec.Runtime.Name, err)
		}
		result.RuntimePath = path
		if downloaded {
			result.DownloadedArtifacts = append(result.DownloadedArtifacts, path)
		}
		result.VerifiedArtifacts = append(result.VerifiedArtifacts, path)
	} else {
		return Result{}, fmt.Errorf("runtime %s not found and no artifact URL configured", spec.Runtime.Name)
	}

	for _, model := range spec.Models {
		if model.Artifact.URL == "" {
			return Result{}, fmt.Errorf("model %s has no artifact URL", model.Name)
		}
		path, downloaded, err := i.downloadAndVerify(ctx, model.Artifact, ManagedModelsDir(root))
		if err != nil {
			return Result{}, fmt.Errorf("installing model %s: %w", model.Name, err)
		}
		if downloaded {
			result.DownloadedArtifacts = append(result.DownloadedArtifacts, path)
		}
		result.VerifiedArtifacts = append(result.VerifiedArtifacts, path)
	}
	return result, nil
}

func (i Installer) root() (string, error) {
	if i.RootDir != "" {
		return i.RootDir, nil
	}
	home := i.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
	}
	return DefaultRoot(home), nil
}

func (i Installer) detectRuntime(spec RuntimeSpec) (string, bool) {
	lookPath := i.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	for _, binary := range spec.BinaryNames {
		if strings.TrimSpace(binary) == "" {
			continue
		}
		path, err := lookPath(binary)
		if err == nil && path != "" {
			return path, true
		}
	}
	return "", false
}

func (i Installer) downloadAndVerify(ctx context.Context, artifact Artifact, dir string) (string, bool, error) {
	if err := validateArtifactName(artifact.Name); err != nil {
		return "", false, err
	}
	if artifact.SHA256 == "" {
		return "", false, fmt.Errorf("artifact %s is missing sha256", artifact.Name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("creating artifact dir: %w", err)
	}
	finalPath := filepath.Join(dir, artifact.Name)
	if err := verifySHA256(finalPath, artifact.SHA256); err == nil {
		i.emit(Event{Stage: "verified", Name: artifact.Name, Path: finalPath})
		return finalPath, false, nil
	}

	partPath := finalPath + ".part"
	offset := existingSize(partPath)
	if err := i.download(ctx, artifact, partPath, offset); err != nil {
		return "", false, err
	}
	if err := verifySHA256(partPath, artifact.SHA256); err != nil {
		return "", true, err
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		return "", true, fmt.Errorf("finalizing %s: %w", artifact.Name, err)
	}
	i.emit(Event{Stage: "verified", Name: artifact.Name, Path: finalPath})
	return finalPath, true, nil
}

func (i Installer) download(ctx context.Context, artifact Artifact, partPath string, offset int64) error {
	client := i.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return fmt.Errorf("building download request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", artifact.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("downloading %s: HTTP %d", artifact.Name, resp.StatusCode)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 && resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		offset = 0
	}
	f, err := os.OpenFile(partPath, flags, 0o600)
	if err != nil {
		return fmt.Errorf("opening partial artifact: %w", err)
	}
	defer f.Close()

	i.emit(Event{Stage: "download_started", Name: artifact.Name, Path: partPath, Bytes: offset, Total: totalBytes(resp, offset)})
	written, err := io.Copy(&progressWriter{
		w: f,
		onWrite: func(n int64) {
			i.emit(Event{Stage: "download_progress", Name: artifact.Name, Path: partPath, Bytes: offset + n, Total: totalBytes(resp, offset)})
		},
	}, resp.Body)
	if err != nil {
		return fmt.Errorf("writing partial artifact: %w", err)
	}
	i.emit(Event{Stage: "download_finished", Name: artifact.Name, Path: partPath, Bytes: offset + written, Total: totalBytes(resp, offset)})
	return nil
}

func validateArtifactName(name string) error {
	if name == "" {
		return errors.New("artifact name is required")
	}
	if filepath.Base(name) != name || strings.Contains(name, string(filepath.Separator)) {
		return fmt.Errorf("artifact name %q must not contain path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("artifact name %q is invalid", name)
	}
	return nil
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return err
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", path, got, expected)
	}
	return nil
}

func existingSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func totalBytes(resp *http.Response, offset int64) int64 {
	if resp.ContentLength < 0 {
		return -1
	}
	return offset + resp.ContentLength
}

func (i Installer) emit(event Event) {
	if i.Progress != nil {
		i.Progress(event)
	}
}

type progressWriter struct {
	w       io.Writer
	written int64
	onWrite func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.written += int64(n)
	if p.onWrite != nil {
		p.onWrite(p.written)
	}
	return n, err
}

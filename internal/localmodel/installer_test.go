package localmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestInstallAdoptsExistingRuntimeAndStoresModelsUnderManagedDir(t *testing.T) {
	modelBytes := []byte("model weights")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(modelBytes)
	}))
	defer srv.Close()

	root := filepath.Join(t.TempDir(), ".conduit")
	result, err := Installer{
		RootDir: root,
		LookPath: func(name string) (string, error) {
			if name == "ollama" {
				return "/usr/local/bin/ollama", nil
			}
			return "", os.ErrNotExist
		},
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{Name: "ollama", Kind: RuntimeOllama, BinaryNames: []string{"ollama"}},
		Models: []ModelSpec{{
			Name:    "tiny",
			Runtime: RuntimeOllama,
			Artifact: Artifact{
				Name:   "tiny.gguf",
				URL:    srv.URL,
				SHA256: checksum(modelBytes),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.RuntimeAdopted || result.RuntimePath != "/usr/local/bin/ollama" {
		t.Fatalf("runtime adoption = (%v, %q), want existing ollama", result.RuntimeAdopted, result.RuntimePath)
	}
	modelPath := filepath.Join(root, "models", "tiny.gguf")
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("managed model missing at %s: %v", modelPath, err)
	}
}

func TestInstallVerifiesChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer srv.Close()

	root := filepath.Join(t.TempDir(), ".conduit")
	_, err := Installer{
		RootDir:  root,
		LookPath: func(string) (string, error) { return "/bin/ollama", nil },
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{Name: "ollama", BinaryNames: []string{"ollama"}},
		Models: []ModelSpec{{
			Name: "bad",
			Artifact: Artifact{
				Name:   "bad.gguf",
				URL:    srv.URL,
				SHA256: checksum([]byte("expected")),
			},
		}},
	})
	if err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestInstallRejectsArtifactPathTraversal(t *testing.T) {
	_, err := Installer{
		RootDir:  filepath.Join(t.TempDir(), ".conduit"),
		LookPath: func(string) (string, error) { return "/bin/ollama", nil },
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{Name: "ollama", BinaryNames: []string{"ollama"}},
		Models: []ModelSpec{{
			Name: "escape",
			Artifact: Artifact{
				Name:   "../escape.gguf",
				URL:    "http://example.test/model",
				SHA256: checksum([]byte("model")),
			},
		}},
	})
	if err == nil {
		t.Fatal("expected artifact path traversal error")
	}
}

func TestInstallResumesPartialDownload(t *testing.T) {
	full := []byte("abcdef")
	var sawRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRange = r.Header.Get("Range")
		if sawRange != "bytes=3-" {
			t.Fatalf("Range = %q, want bytes=3-", sawRange)
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(full[3:])
	}))
	defer srv.Close()

	root := filepath.Join(t.TempDir(), ".conduit")
	modelDir := ManagedModelsDir(root)
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "resume.gguf.part"), full[:3], 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Installer{
		RootDir:  root,
		LookPath: func(string) (string, error) { return "/bin/ollama", nil },
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{Name: "ollama", BinaryNames: []string{"ollama"}},
		Models: []ModelSpec{{
			Name: "resume",
			Artifact: Artifact{
				Name:   "resume.gguf",
				URL:    srv.URL,
				SHA256: checksum(full),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(modelDir, "resume.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(full) {
		t.Fatalf("resumed file = %q, want %q", got, full)
	}
	if _, err := os.Stat(filepath.Join(modelDir, "resume.gguf.part")); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
}

func TestInstallSkipsAlreadyVerifiedModel(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".conduit")
	modelDir := ManagedModelsDir(root)
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("already here")
	if err := os.WriteFile(filepath.Join(modelDir, "cached.gguf"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	result, err := Installer{
		RootDir:  root,
		LookPath: func(string) (string, error) { return "/bin/ollama", nil },
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{Name: "ollama", BinaryNames: []string{"ollama"}},
		Models: []ModelSpec{{
			Name: "cached",
			Artifact: Artifact{
				Name:   "cached.gguf",
				URL:    srv.URL,
				SHA256: checksum(data),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("download server was called for an already verified model")
	}
	if len(result.DownloadedArtifacts) != 0 {
		t.Fatalf("DownloadedArtifacts = %v, want none", result.DownloadedArtifacts)
	}
}

func TestInstallDownloadsRuntimeWhenNoCompatibleInstallExists(t *testing.T) {
	runtimeBytes := []byte("runtime binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(runtimeBytes)
	}))
	defer srv.Close()

	root := filepath.Join(t.TempDir(), ".conduit")
	result, err := Installer{
		RootDir: root,
		LookPath: func(string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}.Install(context.Background(), InstallSpec{
		Runtime: RuntimeSpec{
			Name:        "llama.cpp",
			Kind:        RuntimeLlamaCPP,
			BinaryNames: []string{"llama-server"},
			Artifact: Artifact{
				Name:   "llama-server",
				URL:    srv.URL,
				SHA256: checksum(runtimeBytes),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RuntimeAdopted {
		t.Fatal("runtime should have been downloaded, not adopted")
	}
	wantPath := filepath.Join(root, "runtimes", "llama.cpp", "llama-server")
	if result.RuntimePath != wantPath {
		t.Fatalf("RuntimePath = %q, want %q", result.RuntimePath, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("downloaded runtime missing: %v", err)
	}
}

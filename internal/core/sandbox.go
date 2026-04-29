package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

var (
	ErrMountHostPathRequired    = errors.New("sandbox mount requires host path")
	ErrMountSandboxPathRequired = errors.New("sandbox mount requires sandbox path")
	ErrMountSandboxPathAbsolute = errors.New("sandbox mount sandbox path must be absolute")
	ErrMountModeUnsupported     = errors.New("sandbox mount mode is unsupported")
	ErrMountSensitivePath       = errors.New("sandbox mount targets a sensitive host path")
)

// SandboxConfig captures the filesystem-isolation policy that is enforced
// before a sandbox runtime receives any host mount.
type SandboxConfig struct {
	DefaultMountMode contracts.MountMode
	SensitiveMarkers []string
	SensitivePaths   []string
}

// SandboxMountManager validates host-to-sandbox filesystem grants.
type SandboxMountManager struct {
	config  SandboxConfig
	homeDir string
}

// DefaultSandboxConfig follows PRD 15.2: copy-in by default, with credential
// and system paths protected unless the caller supplies an explicit override.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		DefaultMountMode: contracts.MountModeCopyIn,
		SensitiveMarkers: []string{
			"credential",
			"password",
			"secret",
			"token",
		},
		SensitivePaths: []string{
			"~/.aws",
			"~/.gcloud",
			"~/.gnupg",
			"~/.ssh",
			"~/Library/Keychains",
			"/etc",
		},
	}
}

// NewSandboxMountManager creates the default host mount policy manager.
func NewSandboxMountManager(config SandboxConfig) (*SandboxMountManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewSandboxMountManagerForHome(config, home), nil
}

// NewSandboxMountManagerForHome creates a mount manager with an injected home
// directory for deterministic tests.
func NewSandboxMountManagerForHome(config SandboxConfig, homeDir string) *SandboxMountManager {
	if config.DefaultMountMode == "" {
		config.DefaultMountMode = contracts.MountModeCopyIn
	}
	return &SandboxMountManager{
		config:  config,
		homeDir: filepath.Clean(homeDir),
	}
}

// NormalizeMount expands user paths, fills the default copy-in mode, and
// validates the policy gate for a static sandbox mount.
func (m *SandboxMountManager) NormalizeMount(mount contracts.SandboxMount) (contracts.SandboxMount, error) {
	normalized, err := m.normalizeMountShape(mount)
	if err != nil {
		return contracts.SandboxMount{}, err
	}
	if m.isSensitiveHostPath(normalized.HostPath) && !normalized.AllowSensitivePathOverride {
		return contracts.SandboxMount{}, fmt.Errorf("%w: %s", ErrMountSensitivePath, normalized.HostPath)
	}
	return normalized, nil
}

// RequestDynamicMount records an agent-requested mount as requiring user
// approval. Sensitive requests are blocked until an explicit override is
// supplied by the approval surface.
func (m *SandboxMountManager) RequestDynamicMount(mount contracts.SandboxMount) (contracts.DynamicMountRequest, error) {
	normalized, err := m.normalizeMountShape(mount)
	if err != nil {
		return contracts.DynamicMountRequest{}, err
	}

	req := contracts.DynamicMountRequest{
		Mount:                normalized,
		RequiresUserApproval: true,
	}
	if m.isSensitiveHostPath(normalized.HostPath) && !normalized.AllowSensitivePathOverride {
		req.Blocked = true
		req.BlockReason = ErrMountSensitivePath.Error()
	}
	return req, nil
}

func (m *SandboxMountManager) normalizeMountShape(mount contracts.SandboxMount) (contracts.SandboxMount, error) {
	if strings.TrimSpace(mount.HostPath) == "" {
		return contracts.SandboxMount{}, ErrMountHostPathRequired
	}
	if strings.TrimSpace(mount.SandboxPath) == "" {
		return contracts.SandboxMount{}, ErrMountSandboxPathRequired
	}

	mode := mount.Mode
	if mode == "" {
		mode = m.config.DefaultMountMode
	}
	if !isSupportedMountMode(mode) {
		return contracts.SandboxMount{}, fmt.Errorf("%w: %s", ErrMountModeUnsupported, mode)
	}

	sandboxPath := filepath.Clean(mount.SandboxPath)
	if !filepath.IsAbs(sandboxPath) {
		return contracts.SandboxMount{}, ErrMountSandboxPathAbsolute
	}

	hostPath, err := m.cleanHostPath(mount.HostPath)
	if err != nil {
		return contracts.SandboxMount{}, err
	}

	mount.HostPath = hostPath
	mount.SandboxPath = sandboxPath
	mount.Mode = mode
	return mount, nil
}

func (m *SandboxMountManager) cleanHostPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "~" {
		return m.homeDir, nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		return filepath.Join(m.homeDir, strings.TrimPrefix(trimmed, "~/")), nil
	}
	if !filepath.IsAbs(trimmed) {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", err
		}
		trimmed = abs
	}
	return filepath.Clean(trimmed), nil
}

func (m *SandboxMountManager) isSensitiveHostPath(path string) bool {
	cleaned := filepath.Clean(path)
	for _, sensitivePath := range m.config.SensitivePaths {
		normalized, err := m.cleanHostPath(sensitivePath)
		if err != nil {
			continue
		}
		if cleaned == normalized || strings.HasPrefix(cleaned, normalized+string(os.PathSeparator)) {
			return true
		}
	}

	for _, part := range strings.Split(strings.ToLower(cleaned), string(os.PathSeparator)) {
		for _, marker := range m.config.SensitiveMarkers {
			if strings.Contains(part, strings.ToLower(marker)) {
				return true
			}
		}
	}
	return false
}

func isSupportedMountMode(mode contracts.MountMode) bool {
	switch mode {
	case contracts.MountModeReadOnly,
		contracts.MountModeReadWrite,
		contracts.MountModeCopyIn,
		contracts.MountModeCopyOut:
		return true
	default:
		return false
	}
}

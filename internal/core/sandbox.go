package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const warmSandboxStartupBudget = 2 * time.Second

var (
	requiredSandboxShells       = []string{"bash", "sh"}
	ErrMountHostPathRequired    = errors.New("sandbox mount requires host path")
	ErrMountSandboxPathRequired = errors.New("sandbox mount requires sandbox path")
	ErrMountSandboxPathAbsolute = errors.New("sandbox mount sandbox path must be absolute")
	ErrMountModeUnsupported     = errors.New("sandbox mount mode is unsupported")
	ErrMountSensitivePath       = errors.New("sandbox mount targets a sensitive host path")
)

// SandboxManager owns the core sandbox architecture and mount policy. Runtime
// adapters can use this contract to launch Apple Virtualization.framework
// microVMs or OCI containers without weakening Conduit's isolation guarantees.
type SandboxManager struct {
	architecture     contracts.SandboxArchitecture
	homeDir          string
	defaultMountMode contracts.SandboxMountMode
	sensitiveMarkers []string
	sensitivePaths   []string
}

// DefaultSandboxArchitecture captures the PRD 15.1 baseline for agent tool
// execution: a warm Ubuntu userspace with explicit host access gates.
func DefaultSandboxArchitecture() contracts.SandboxArchitecture {
	return contracts.SandboxArchitecture{
		Backend:                   contracts.SandboxBackendAppleVirtualization,
		BaseImage:                 "ubuntu-24.04",
		ImagePrecached:            true,
		WarmStartBudget:           warmSandboxStartupBudget,
		Shells:                    []string{"bash", "zsh", "sh"},
		PreinstalledRuntimes:      []string{"go", "node", "python"},
		NetworkPolicy:             contracts.SandboxNetworkPolicyControlledEgress,
		DenyHostFilesystem:        true,
		DenyHostNetwork:           true,
		DenyHostProcesses:         true,
		DenyPrivilegeEscalation:   true,
		DenyDockerSocket:          true,
		DiscardUnexportedChanges:  true,
		RequiresExplicitMounts:    true,
		RequiresExplicitEgress:    true,
		RequiresExplicitPortFwd:   true,
		RequiresNonRootUser:       true,
		RequiresProcessNamespace:  true,
		RequiresFilesystemOverlay: true,
	}
}

// NewSandboxManager creates a sandbox policy manager. Invalid architecture
// policies are accepted so callers can surface all validation errors together.
func NewSandboxManager(architecture contracts.SandboxArchitecture) *SandboxManager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	return NewSandboxManagerForHome(architecture, home)
}

// NewSandboxManagerForHome creates a mount-aware sandbox manager with an
// injected home directory for deterministic tests.
func NewSandboxManagerForHome(architecture contracts.SandboxArchitecture, homeDir string) *SandboxManager {
	return &SandboxManager{
		architecture:     architecture,
		homeDir:          filepath.Clean(homeDir),
		defaultMountMode: contracts.SandboxMountCopyIn,
		sensitiveMarkers: []string{
			"credential",
			"password",
			"secret",
			"token",
		},
		sensitivePaths: []string{
			"~/.aws",
			"~/.gcloud",
			"~/.gnupg",
			"~/.ssh",
			"~/Library/Keychains",
			"/etc",
		},
	}
}

// Architecture returns a deep copy of the sandbox contract.
func (m *SandboxManager) Architecture() contracts.SandboxArchitecture {
	return copySandboxArchitecture(m.architecture)
}

// Validate checks whether the sandbox policy satisfies Conduit's minimum
// isolation and startup guarantees.
func (m *SandboxManager) Validate() error {
	var problems []string

	if m.architecture.BaseImage == "" {
		problems = append(problems, "base image is required")
	}
	if !m.architecture.ImagePrecached {
		problems = append(problems, "sandbox image must be pre-cached")
	}
	if m.architecture.WarmStartBudget <= 0 || m.architecture.WarmStartBudget > warmSandboxStartupBudget {
		problems = append(problems, "warm start budget must be at most 2s")
	}
	for _, shell := range requiredSandboxShells {
		if !slices.Contains(m.architecture.Shells, shell) {
			problems = append(problems, fmt.Sprintf("required shell %q is missing", shell))
		}
	}
	if len(m.architecture.PreinstalledRuntimes) == 0 {
		problems = append(problems, "at least one runtime must be pre-installed")
	}
	if m.architecture.NetworkPolicy == "" {
		problems = append(problems, "network policy is required")
	}

	requiredBooleans := map[string]bool{
		"host filesystem access must be denied by default":          m.architecture.DenyHostFilesystem,
		"host network access must be denied by default":             m.architecture.DenyHostNetwork,
		"host process access must be denied by default":             m.architecture.DenyHostProcesses,
		"privilege escalation must be denied":                       m.architecture.DenyPrivilegeEscalation,
		"docker socket access must be denied":                       m.architecture.DenyDockerSocket,
		"unexported sandbox changes must be discarded":              m.architecture.DiscardUnexportedChanges,
		"host mounts must require explicit grants":                  m.architecture.RequiresExplicitMounts,
		"network egress must require explicit grants":               m.architecture.RequiresExplicitEgress,
		"inbound ports must require explicit forwarding":            m.architecture.RequiresExplicitPortFwd,
		"agent commands must run as a non-root user":                m.architecture.RequiresNonRootUser,
		"host processes must be hidden by a process namespace":      m.architecture.RequiresProcessNamespace,
		"host changes must be isolated behind a filesystem overlay": m.architecture.RequiresFilesystemOverlay,
	}
	for message, ok := range requiredBooleans {
		if !ok {
			problems = append(problems, message)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid sandbox architecture: %s", strings.Join(problems, "; "))
	}
	return nil
}

// NormalizeMount expands user paths, fills the default copy-in mode, and
// validates the policy gate for a static sandbox mount.
func (m *SandboxManager) NormalizeMount(mount contracts.SandboxMount) (contracts.SandboxMount, error) {
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
func (m *SandboxManager) RequestDynamicMount(mount contracts.SandboxMount) (contracts.DynamicMountRequest, error) {
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

func (m *SandboxManager) normalizeMountShape(mount contracts.SandboxMount) (contracts.SandboxMount, error) {
	if strings.TrimSpace(mount.HostPath) == "" {
		return contracts.SandboxMount{}, ErrMountHostPathRequired
	}
	if strings.TrimSpace(mount.SandboxPath) == "" {
		return contracts.SandboxMount{}, ErrMountSandboxPathRequired
	}

	mode := mount.Mode
	if mode == "" {
		mode = m.defaultMountMode
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

func (m *SandboxManager) cleanHostPath(path string) (string, error) {
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

func (m *SandboxManager) isSensitiveHostPath(path string) bool {
	cleaned := filepath.Clean(path)
	for _, sensitivePath := range m.sensitivePaths {
		normalized, err := m.cleanHostPath(sensitivePath)
		if err != nil {
			continue
		}
		if cleaned == normalized || strings.HasPrefix(cleaned, normalized+string(os.PathSeparator)) {
			return true
		}
	}

	for _, part := range strings.Split(strings.ToLower(cleaned), string(os.PathSeparator)) {
		for _, marker := range m.sensitiveMarkers {
			if strings.Contains(part, strings.ToLower(marker)) {
				return true
			}
		}
	}
	return false
}

func isSupportedMountMode(mode contracts.SandboxMountMode) bool {
	switch mode {
	case contracts.SandboxMountReadOnly,
		contracts.SandboxMountReadWrite,
		contracts.SandboxMountCopyIn,
		contracts.SandboxMountCopyOut:
		return true
	default:
		return false
	}
}

func copySandboxArchitecture(architecture contracts.SandboxArchitecture) contracts.SandboxArchitecture {
	architecture.Shells = append([]string(nil), architecture.Shells...)
	architecture.PreinstalledRuntimes = append([]string(nil), architecture.PreinstalledRuntimes...)
	architecture.Mounts = append([]contracts.SandboxMount(nil), architecture.Mounts...)
	return architecture
}

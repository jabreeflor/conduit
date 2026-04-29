package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const warmSandboxStartupBudget = 2 * time.Second

var requiredSandboxShells = []string{"bash", "sh"}

// SandboxManager owns the core sandbox architecture policy. Runtime adapters
// can use this contract to launch Apple Virtualization.framework microVMs or
// OCI containers without weakening Conduit's isolation guarantees.
type SandboxManager struct {
	architecture contracts.SandboxArchitecture
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

// NewSandboxManager creates a sandbox policy manager. Invalid policies are
// accepted so callers can surface all validation errors together.
func NewSandboxManager(architecture contracts.SandboxArchitecture) *SandboxManager {
	return &SandboxManager{architecture: architecture}
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

func copySandboxArchitecture(architecture contracts.SandboxArchitecture) contracts.SandboxArchitecture {
	architecture.Shells = append([]string(nil), architecture.Shells...)
	architecture.PreinstalledRuntimes = append([]string(nil), architecture.PreinstalledRuntimes...)
	architecture.Mounts = append([]contracts.SandboxMount(nil), architecture.Mounts...)
	return architecture
}

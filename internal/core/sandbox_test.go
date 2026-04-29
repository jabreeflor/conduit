package core

import (
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestDefaultSandboxArchitectureMatchesPRDGuarantees(t *testing.T) {
	manager := NewSandboxManager(DefaultSandboxArchitecture())

	if err := manager.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	architecture := manager.Architecture()
	if architecture.BaseImage != "ubuntu-24.04" {
		t.Fatalf("BaseImage = %q, want ubuntu-24.04", architecture.BaseImage)
	}
	if !architecture.ImagePrecached {
		t.Fatal("ImagePrecached = false, want true")
	}
	if architecture.WarmStartBudget > 2*time.Second {
		t.Fatalf("WarmStartBudget = %s, want <= 2s", architecture.WarmStartBudget)
	}
	if architecture.NetworkPolicy != contracts.SandboxNetworkPolicyControlledEgress {
		t.Fatalf("NetworkPolicy = %q, want controlled egress", architecture.NetworkPolicy)
	}
	if !architecture.DenyHostFilesystem || !architecture.DenyHostNetwork || !architecture.DenyHostProcesses {
		t.Fatalf("host isolation flags = filesystem:%t network:%t processes:%t, want all true",
			architecture.DenyHostFilesystem,
			architecture.DenyHostNetwork,
			architecture.DenyHostProcesses,
		)
	}
	if !architecture.DenyPrivilegeEscalation || !architecture.DenyDockerSocket || !architecture.RequiresNonRootUser {
		t.Fatalf("privilege controls = escalation:%t docker:%t non-root:%t, want all true",
			architecture.DenyPrivilegeEscalation,
			architecture.DenyDockerSocket,
			architecture.RequiresNonRootUser,
		)
	}
}

func TestSandboxArchitectureReturnsCopies(t *testing.T) {
	engine := New("test")

	first := engine.SandboxArchitecture()
	first.Shells[0] = "fish"
	first.PreinstalledRuntimes[0] = "ruby"

	next := engine.SandboxArchitecture()
	if next.Shells[0] != "bash" {
		t.Fatalf("Shells were mutated through Architecture: %v", next.Shells)
	}
	if next.PreinstalledRuntimes[0] != "go" {
		t.Fatalf("PreinstalledRuntimes were mutated through Architecture: %v", next.PreinstalledRuntimes)
	}
}

func TestSandboxValidationRejectsWeakenedHostIsolation(t *testing.T) {
	architecture := DefaultSandboxArchitecture()
	architecture.DenyHostFilesystem = false
	architecture.DenyHostNetwork = false
	architecture.DenyHostProcesses = false
	architecture.DenyPrivilegeEscalation = false
	architecture.ImagePrecached = false
	architecture.WarmStartBudget = 3 * time.Second

	err := NewSandboxManager(architecture).Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want isolation errors")
	}

	message := err.Error()
	for _, want := range []string{
		"pre-cached",
		"warm start budget",
		"host filesystem",
		"host network",
		"host process",
		"privilege escalation",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("Validate error %q does not contain %q", message, want)
		}
	}
}

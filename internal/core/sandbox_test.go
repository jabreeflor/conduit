package core

import (
	"errors"
	"path/filepath"
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

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
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

func TestSandboxMountDefaultsToCopyIn(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	mount, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "~/Projects/conduit",
		SandboxPath: "/workspace",
	})
	if err != nil {
		t.Fatalf("NormalizeMount returned error: %v", err)
	}

	wantHost := filepath.Join("/Users/alex", "Projects", "conduit")
	if mount.HostPath != wantHost {
		t.Fatalf("HostPath = %q, want %q", mount.HostPath, wantHost)
	}
	if mount.Mode != contracts.SandboxMountCopyIn {
		t.Fatalf("Mode = %q, want %q", mount.Mode, contracts.SandboxMountCopyIn)
	}
}

func TestSandboxMountAcceptsAllPRDModes(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")
	modes := []contracts.SandboxMountMode{
		contracts.SandboxMountReadOnly,
		contracts.SandboxMountReadWrite,
		contracts.SandboxMountCopyIn,
		contracts.SandboxMountCopyOut,
	}

	for _, mode := range modes {
		_, err := manager.NormalizeMount(contracts.SandboxMount{
			HostPath:    "/Users/alex/Projects/conduit",
			SandboxPath: "/workspace",
			Mode:        mode,
		})
		if err != nil {
			t.Fatalf("NormalizeMount(%q) returned error: %v", mode, err)
		}
	}
}

func TestSandboxMountRejectsSensitivePathsWithoutOverride(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "~/.ssh",
		SandboxPath: "/keys",
		Mode:        contracts.SandboxMountReadOnly,
	})
	if !errors.Is(err, ErrMountSensitivePath) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSensitivePath", err)
	}
}

func TestSandboxMountAllowsSensitivePathsWithExplicitOverride(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	mount, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:                   "~/Library/Keychains/login.keychain-db",
		SandboxPath:                "/keychains",
		Mode:                       contracts.SandboxMountReadOnly,
		AllowSensitivePathOverride: true,
	})
	if err != nil {
		t.Fatalf("NormalizeMount returned error: %v", err)
	}
	if mount.HostPath != filepath.Join("/Users/alex", "Library", "Keychains", "login.keychain-db") {
		t.Fatalf("HostPath = %q, want expanded keychain path", mount.HostPath)
	}
}

func TestSandboxMountRejectsCredentialNameHeuristic(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "/Users/alex/project/access-token-cache",
		SandboxPath: "/workspace",
	})
	if !errors.Is(err, ErrMountSensitivePath) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSensitivePath", err)
	}
}

func TestDynamicMountRequestsAlwaysRequireApproval(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	req, err := manager.RequestDynamicMount(contracts.SandboxMount{
		HostPath:    "~/Downloads",
		SandboxPath: "/downloads",
		Mode:        contracts.SandboxMountReadWrite,
	})
	if err != nil {
		t.Fatalf("RequestDynamicMount returned error: %v", err)
	}
	if !req.RequiresUserApproval {
		t.Fatal("RequiresUserApproval = false, want true")
	}
	if req.Blocked {
		t.Fatalf("Blocked = true with reason %q, want false", req.BlockReason)
	}
}

func TestDynamicMountBlocksSensitiveRequestsUntilOverride(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	req, err := manager.RequestDynamicMount(contracts.SandboxMount{
		HostPath:    "/etc",
		SandboxPath: "/host-etc",
		Mode:        contracts.SandboxMountReadOnly,
	})
	if err != nil {
		t.Fatalf("RequestDynamicMount returned error: %v", err)
	}
	if !req.RequiresUserApproval {
		t.Fatal("RequiresUserApproval = false, want true")
	}
	if !req.Blocked {
		t.Fatal("Blocked = false, want true")
	}
	if req.BlockReason == "" {
		t.Fatal("BlockReason is empty")
	}
}

func TestSandboxMountRejectsRelativeSandboxPath(t *testing.T) {
	manager := NewSandboxManagerForHome(DefaultSandboxArchitecture(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "/Users/alex/Projects/conduit",
		SandboxPath: "workspace",
	})
	if !errors.Is(err, ErrMountSandboxPathAbsolute) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSandboxPathAbsolute", err)
	}
}

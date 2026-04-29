package core

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestSandboxMountDefaultsToCopyIn(t *testing.T) {
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

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
	if mount.Mode != contracts.MountModeCopyIn {
		t.Fatalf("Mode = %q, want %q", mount.Mode, contracts.MountModeCopyIn)
	}
}

func TestSandboxMountAcceptsAllPRDModes(t *testing.T) {
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")
	modes := []contracts.MountMode{
		contracts.MountModeReadOnly,
		contracts.MountModeReadWrite,
		contracts.MountModeCopyIn,
		contracts.MountModeCopyOut,
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
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "~/.ssh",
		SandboxPath: "/keys",
		Mode:        contracts.MountModeReadOnly,
	})
	if !errors.Is(err, ErrMountSensitivePath) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSensitivePath", err)
	}
}

func TestSandboxMountAllowsSensitivePathsWithExplicitOverride(t *testing.T) {
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	mount, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:                   "~/Library/Keychains/login.keychain-db",
		SandboxPath:                "/keychains",
		Mode:                       contracts.MountModeReadOnly,
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
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "/Users/alex/project/access-token-cache",
		SandboxPath: "/workspace",
	})
	if !errors.Is(err, ErrMountSensitivePath) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSensitivePath", err)
	}
}

func TestDynamicMountRequestsAlwaysRequireApproval(t *testing.T) {
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	req, err := manager.RequestDynamicMount(contracts.SandboxMount{
		HostPath:    "~/Downloads",
		SandboxPath: "/downloads",
		Mode:        contracts.MountModeReadWrite,
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
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	req, err := manager.RequestDynamicMount(contracts.SandboxMount{
		HostPath:    "/etc",
		SandboxPath: "/host-etc",
		Mode:        contracts.MountModeReadOnly,
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
	manager := NewSandboxMountManagerForHome(DefaultSandboxConfig(), "/Users/alex")

	_, err := manager.NormalizeMount(contracts.SandboxMount{
		HostPath:    "/Users/alex/Projects/conduit",
		SandboxPath: "workspace",
	})
	if !errors.Is(err, ErrMountSandboxPathAbsolute) {
		t.Fatalf("NormalizeMount error = %v, want ErrMountSandboxPathAbsolute", err)
	}
}

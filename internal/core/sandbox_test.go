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
	for _, runtime := range []string{"python", "node", "go", "rust"} {
		if !containsFold(architecture.PreinstalledRuntimes, runtime) {
			t.Fatalf("PreinstalledRuntimes = %v, want %q", architecture.PreinstalledRuntimes, runtime)
		}
	}
	if architecture.RuntimeVersions["python"] != "3.12" || architecture.RuntimeVersions["node"] != "20" || architecture.RuntimeVersions["go"] != "1.22" {
		t.Fatalf("RuntimeVersions = %v, want Python 3.12+, Node 20+, Go 1.22+", architecture.RuntimeVersions)
	}
	for _, manager := range []string{"pip", "npm", "yarn", "pnpm", "cargo"} {
		if !containsFold(architecture.PackageManagers, manager) {
			t.Fatalf("PackageManagers = %v, want %q", architecture.PackageManagers, manager)
		}
	}
	for _, tool := range []string{"git", "curl", "jq", "rg", "fd", "vim", "nano", "sqlite3"} {
		if !containsFold(architecture.PreinstalledTools, tool) {
			t.Fatalf("PreinstalledTools = %v, want %q", architecture.PreinstalledTools, tool)
		}
	}
	for _, registry := range []string{"pypi.org", "registry.npmjs.org", "proxy.golang.org", "crates.io"} {
		if !containsFold(architecture.AllowlistedRegistries, registry) {
			t.Fatalf("AllowlistedRegistries = %v, want %q", architecture.AllowlistedRegistries, registry)
		}
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
	first.RuntimeVersions["python"] = "2.7"
	first.PackageManagers[0] = "gem"
	first.PreinstalledTools[0] = "svn"
	first.AllowlistedRegistries[0] = "example.com"

	next := engine.SandboxArchitecture()
	if next.Shells[0] != "bash" {
		t.Fatalf("Shells were mutated through Architecture: %v", next.Shells)
	}
	if next.PreinstalledRuntimes[0] != "go" {
		t.Fatalf("PreinstalledRuntimes were mutated through Architecture: %v", next.PreinstalledRuntimes)
	}
	if next.RuntimeVersions["python"] != "3.12" {
		t.Fatalf("RuntimeVersions were mutated through Architecture: %v", next.RuntimeVersions)
	}
	if next.PackageManagers[0] != "pip" {
		t.Fatalf("PackageManagers were mutated through Architecture: %v", next.PackageManagers)
	}
	if next.PreinstalledTools[0] != "git" {
		t.Fatalf("PreinstalledTools were mutated through Architecture: %v", next.PreinstalledTools)
	}
	if next.AllowlistedRegistries[0] != "pypi.org" {
		t.Fatalf("AllowlistedRegistries were mutated through Architecture: %v", next.AllowlistedRegistries)
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
	architecture.PreinstalledRuntimes = []string{"go"}
	architecture.RuntimeVersions = map[string]string{}
	architecture.PackageManagers = []string{"pip"}
	architecture.PreinstalledTools = []string{"git"}
	architecture.AllowlistedRegistries = []string{"pypi.org"}

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
		"required runtime",
		"minimum version",
		"package manager",
		"required tool",
		"package registry",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("Validate error %q does not contain %q", message, want)
		}
	}
}

func TestSandboxValidationAcceptsShareableCustomBaseImages(t *testing.T) {
	architecture := DefaultSandboxArchitecture()
	architecture.CustomBaseImages = []contracts.SandboxBaseImage{{
		Name:        "team-python",
		Image:       "ghcr.io/acme/conduit-python:2026-05",
		Digest:      "sha256:abc123",
		Description: "Team Python image",
		Shared:      true,
	}}

	if err := NewSandboxManager(architecture).Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestSandboxValidationRejectsIncompleteCustomBaseImages(t *testing.T) {
	architecture := DefaultSandboxArchitecture()
	architecture.CustomBaseImages = []contracts.SandboxBaseImage{{Name: "missing-image"}}

	err := NewSandboxManager(architecture).Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want custom image error")
	}
	if !strings.Contains(err.Error(), "custom base image reference") {
		t.Fatalf("Validate error %q does not mention custom image reference", err)
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

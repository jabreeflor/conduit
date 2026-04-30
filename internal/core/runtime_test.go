package core

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// staticProbe returns a fixed SandboxRuntimeCapabilities for testing.
type staticProbe struct {
	caps contracts.SandboxRuntimeCapabilities
}

func (p staticProbe) Probe() contracts.SandboxRuntimeCapabilities { return p.caps }

func availableProbe(backend contracts.SandboxBackend) RuntimeProbe {
	return staticProbe{caps: contracts.SandboxRuntimeCapabilities{
		Backend:          backend,
		Available:        true,
		SupportsRosetta2: backend == contracts.SandboxBackendAppleVirtualization,
		SupportsVirtioFS: backend == contracts.SandboxBackendAppleVirtualization,
		ColdStartBudget:  coldSandboxStartupBudget,
		MemoryOverheadMB: sandboxMaxMemoryOverheadMB,
	}}
}

func unavailableProbe(backend contracts.SandboxBackend, reason string) RuntimeProbe {
	return staticProbe{caps: contracts.SandboxRuntimeCapabilities{
		Backend:           backend,
		Available:         false,
		UnavailableReason: reason,
	}}
}

func TestRuntimeSelectorPicksAppleVirtualizationFirst(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			contracts.SandboxBackendAppleVirtualization: availableProbe(contracts.SandboxBackendAppleVirtualization),
			contracts.SandboxBackendOCIContainer:        availableProbe(contracts.SandboxBackendOCIContainer),
		},
	)

	caps, err := sel.Select()
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if caps.Backend != contracts.SandboxBackendAppleVirtualization {
		t.Fatalf("Backend = %q, want apple_virtualization", caps.Backend)
	}
}

func TestRuntimeSelectorFallsBackToOCIWhenAppleVirtualizationUnavailable(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			contracts.SandboxBackendAppleVirtualization: unavailableProbe(contracts.SandboxBackendAppleVirtualization, "requires macOS 13+"),
			contracts.SandboxBackendOCIContainer:        availableProbe(contracts.SandboxBackendOCIContainer),
		},
	)

	caps, err := sel.Select()
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if caps.Backend != contracts.SandboxBackendOCIContainer {
		t.Fatalf("Backend = %q, want oci_container", caps.Backend)
	}
}

func TestRuntimeSelectorReturnsErrorWhenNoBackendsAvailable(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			contracts.SandboxBackendAppleVirtualization: unavailableProbe(contracts.SandboxBackendAppleVirtualization, "requires macOS 13+"),
			contracts.SandboxBackendOCIContainer:        unavailableProbe(contracts.SandboxBackendOCIContainer, "docker not installed"),
		},
	)

	_, err := sel.Select()
	if !errors.Is(err, ErrNoRuntimeAvailable) {
		t.Fatalf("Select error = %v, want ErrNoRuntimeAvailable", err)
	}
}

func TestRuntimeSelectorSkipsBackendsWithNoProbe(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			// Apple Virtualization probe deliberately omitted.
			contracts.SandboxBackendOCIContainer: availableProbe(contracts.SandboxBackendOCIContainer),
		},
	)

	caps, err := sel.Select()
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if caps.Backend != contracts.SandboxBackendOCIContainer {
		t.Fatalf("Backend = %q, want oci_container", caps.Backend)
	}
}

func TestAppleVirtualizationCapsIncludeRosetta2AndVirtioFS(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			contracts.SandboxBackendAppleVirtualization: availableProbe(contracts.SandboxBackendAppleVirtualization),
		},
	)

	caps, err := sel.Select()
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if !caps.SupportsRosetta2 {
		t.Fatal("SupportsRosetta2 = false, want true for Apple Virtualization")
	}
	if !caps.SupportsVirtioFS {
		t.Fatal("SupportsVirtioFS = false, want true for Apple Virtualization")
	}
}

func TestDefaultRuntimeCapabilitiesMeetPRDBudgets(t *testing.T) {
	sel := NewRuntimeSelector(
		DefaultRuntimePreferences(),
		map[contracts.SandboxBackend]RuntimeProbe{
			contracts.SandboxBackendAppleVirtualization: availableProbe(contracts.SandboxBackendAppleVirtualization),
		},
	)

	caps, err := sel.Select()
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if caps.ColdStartBudget > 5*time.Second {
		t.Fatalf("ColdStartBudget = %s, want <= 5s", caps.ColdStartBudget)
	}
	if caps.MemoryOverheadMB > 256 {
		t.Fatalf("MemoryOverheadMB = %d, want <= 256", caps.MemoryOverheadMB)
	}
}

func TestDefaultRuntimePreferencesAppleVirtualizationFirst(t *testing.T) {
	prefs := DefaultRuntimePreferences()
	if len(prefs) == 0 {
		t.Fatal("DefaultRuntimePreferences returned empty slice")
	}
	if prefs[0] != contracts.SandboxBackendAppleVirtualization {
		t.Fatalf("prefs[0] = %q, want apple_virtualization", prefs[0])
	}
}

func TestSandboxValidationRejectsMissingColdStartBudget(t *testing.T) {
	arch := DefaultSandboxArchitecture()
	arch.ColdStartBudget = 0

	err := NewSandboxManager(arch).Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want cold start budget error")
	}
	if !strings.Contains(err.Error(), "cold start") {
		t.Fatalf("Validate error %q does not mention cold start", err.Error())
	}
}

func TestSandboxValidationRejectsExcessiveColdStartBudget(t *testing.T) {
	arch := DefaultSandboxArchitecture()
	arch.ColdStartBudget = 10 * time.Second

	err := NewSandboxManager(arch).Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want cold start budget error")
	}
	if !strings.Contains(err.Error(), "cold start") {
		t.Fatalf("Validate error %q does not mention cold start", err.Error())
	}
}

func TestSandboxValidationRejectsExcessiveMemoryOverhead(t *testing.T) {
	arch := DefaultSandboxArchitecture()
	arch.MaxMemoryOverheadMB = 512

	err := NewSandboxManager(arch).Validate()
	if err == nil {
		t.Fatal("Validate returned nil, want memory overhead error")
	}
	if !strings.Contains(err.Error(), "memory overhead") {
		t.Fatalf("Validate error %q does not mention memory overhead", err.Error())
	}
}

func TestDefaultSandboxArchitectureMeetsPRD159Requirements(t *testing.T) {
	arch := DefaultSandboxArchitecture()

	if arch.ColdStartBudget != coldSandboxStartupBudget {
		t.Fatalf("ColdStartBudget = %s, want %s", arch.ColdStartBudget, coldSandboxStartupBudget)
	}
	if arch.WarmStartBudget != warmSandboxStartupBudget {
		t.Fatalf("WarmStartBudget = %s, want %s", arch.WarmStartBudget, warmSandboxStartupBudget)
	}
	if arch.MaxMemoryOverheadMB != sandboxMaxMemoryOverheadMB {
		t.Fatalf("MaxMemoryOverheadMB = %d, want %d", arch.MaxMemoryOverheadMB, sandboxMaxMemoryOverheadMB)
	}
	if arch.Backend != contracts.SandboxBackendAppleVirtualization {
		t.Fatalf("Backend = %q, want apple_virtualization", arch.Backend)
	}
}

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func testMachineProfilerConfig(t *testing.T) MachineProfilerConfig {
	t.Helper()
	dir := t.TempDir()
	return MachineProfilerConfig{
		HomeDir:     dir,
		ProfilePath: filepath.Join(dir, machineProfileFileName),
	}
}

func TestMachineProfiler_ScanWritesCache(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	profile, err := m.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if profile.ProfiledAt.IsZero() {
		t.Error("ProfiledAt should be set")
	}

	data, err := os.ReadFile(cfg.ProfilePath)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}

	var cached contracts.MachineProfile
	if err := json.Unmarshal(data, &cached); err != nil {
		t.Fatalf("cache not valid JSON: %v", err)
	}
	if !cached.ProfiledAt.Equal(profile.ProfiledAt) {
		t.Errorf("cached ProfiledAt = %v, want %v", cached.ProfiledAt, profile.ProfiledAt)
	}
}

func TestMachineProfiler_LoadReturnsCacheWithoutRescanning(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	stub := contracts.MachineProfile{
		ProfiledAt:   fixed,
		MacOSVersion: "14.0.0",
		CPU:          contracts.CPUInfo{Brand: "Test CPU", PhysicalCores: 4, LogicalCores: 8},
		Memory:       contracts.MemInfo{TotalBytes: 16 * (1 << 30), TotalGB: 16},
	}

	data, _ := json.MarshalIndent(stub, "", "  ")
	if err := os.WriteFile(cfg.ProfilePath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := m.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !loaded.ProfiledAt.Equal(fixed) {
		t.Errorf("Load() returned ProfiledAt %v, want %v (should use cache)", loaded.ProfiledAt, fixed)
	}
	if loaded.MacOSVersion != "14.0.0" {
		t.Errorf("Load() MacOSVersion = %q, want %q", loaded.MacOSVersion, "14.0.0")
	}
}

func TestMachineProfiler_LoadFallsBackToScanOnMissingCache(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	profile, err := m.Load()
	if err != nil {
		t.Fatalf("Load() error when no cache exists: %v", err)
	}
	if profile.ProfiledAt.IsZero() {
		t.Error("ProfiledAt should be set after fallback scan")
	}

	if _, err := os.Stat(cfg.ProfilePath); os.IsNotExist(err) {
		t.Error("cache file should have been written after fallback scan")
	}
}

func TestMachineProfiler_LoadFallsBackToScanOnCorruptCache(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	if err := os.WriteFile(cfg.ProfilePath, []byte("not json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile, err := m.Load()
	if err != nil {
		t.Fatalf("Load() error on corrupt cache: %v", err)
	}
	if profile.ProfiledAt.IsZero() {
		t.Error("ProfiledAt should be set after fallback scan")
	}
}

func TestParseVRAMString(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"8 GB", 8},
		{"16 GB", 16},
		{"16384 MB", 16},
		{"4096 MB", 4},
		{"", 0},
		{"bad input", 0},
		{"12 TB", 12},
	}
	for _, tc := range tests {
		got := parseVRAMString(tc.input)
		if got != tc.want {
			t.Errorf("parseVRAMString(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestMachineProfiler_ScanCompletesWithinBudget(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	start := time.Now()
	_, err := m.Scan()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if elapsed > profilingTimeout {
		t.Errorf("Scan() took %v, must complete within %v", elapsed, profilingTimeout)
	}
}

func TestMachineProfiler_ScanPopulatesFields(t *testing.T) {
	cfg := testMachineProfilerConfig(t)
	m := NewMachineProfiler(cfg)

	profile, err := m.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if profile.MacOSVersion == "" {
		t.Error("MacOSVersion should be populated on macOS")
	}
	if profile.CPU.Brand == "" {
		t.Error("CPU.Brand should be populated")
	}
	if profile.CPU.PhysicalCores == 0 {
		t.Error("CPU.PhysicalCores should be > 0")
	}
	if profile.Memory.TotalBytes == 0 {
		t.Error("Memory.TotalBytes should be > 0")
	}
	if profile.Disk.TotalBytes == 0 {
		t.Error("Disk.TotalBytes should be > 0")
	}
}

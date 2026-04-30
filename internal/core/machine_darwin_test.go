//go:build darwin

package core

import "testing"

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

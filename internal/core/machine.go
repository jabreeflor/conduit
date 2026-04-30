package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const (
	machineProfileFileName = "machine-profile.json"
	profilingTimeout       = 3 * time.Second
)

// MachineProfilerConfig locates the cache file. The default layout is
// ~/.conduit/machine-profile.json.
type MachineProfilerConfig struct {
	HomeDir     string
	ProfilePath string
}

// DefaultMachineProfilerConfig returns the standard cache location.
func DefaultMachineProfilerConfig() MachineProfilerConfig {
	home, _ := os.UserHomeDir()
	conduitHome := filepath.Join(home, ".conduit")
	return MachineProfilerConfig{
		HomeDir:     conduitHome,
		ProfilePath: filepath.Join(conduitHome, machineProfileFileName),
	}
}

// MachineProfiler probes local hardware and caches the result to disk.
type MachineProfiler struct {
	config MachineProfilerConfig
	now    func() time.Time
}

// NewMachineProfiler creates a profiler using the provided config.
func NewMachineProfiler(config MachineProfilerConfig) *MachineProfiler {
	return &MachineProfiler{
		config: config,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Load returns the cached profile from disk, or runs a fresh scan if no cache
// exists or the cache file is unreadable.
func (m *MachineProfiler) Load() (contracts.MachineProfile, error) {
	if profile, ok := m.readCache(); ok {
		return profile, nil
	}
	return m.Scan()
}

// Scan runs a fresh hardware probe in parallel across all subsystems, writes
// the result to disk, and returns it. The entire probe is bounded by a
// 3-second deadline.
func (m *MachineProfiler) Scan() (contracts.MachineProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), profilingTimeout)
	defer cancel()

	profile := contracts.MachineProfile{ProfiledAt: m.now()}

	var mu sync.Mutex
	var wg sync.WaitGroup

	type probe struct {
		run func(context.Context)
	}
	probes := []probe{
		{func(ctx context.Context) {
			v := swVers(ctx)
			mu.Lock()
			profile.MacOSVersion = v
			mu.Unlock()
		}},
		{func(ctx context.Context) {
			cpu := probeCPU(ctx)
			mu.Lock()
			profile.CPU = cpu
			mu.Unlock()
		}},
		{func(ctx context.Context) {
			mem := probeMemory(ctx)
			mu.Lock()
			profile.Memory = mem
			mu.Unlock()
		}},
		{func(ctx context.Context) {
			gpus := probeGPUs(ctx)
			mu.Lock()
			profile.GPU = gpus
			mu.Unlock()
		}},
		{func(ctx context.Context) {
			disk := probeDisk(ctx)
			mu.Lock()
			profile.Disk = disk
			mu.Unlock()
		}},
	}

	for _, p := range probes {
		wg.Add(1)
		go func(fn func(context.Context)) {
			defer wg.Done()
			fn(ctx)
		}(p.run)
	}
	wg.Wait()

	if err := m.writeCache(profile); err != nil {
		return profile, err
	}
	return profile, nil
}

func (m *MachineProfiler) readCache() (contracts.MachineProfile, bool) {
	data, err := os.ReadFile(m.config.ProfilePath)
	if err != nil {
		return contracts.MachineProfile{}, false
	}
	var profile contracts.MachineProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return contracts.MachineProfile{}, false
	}
	return profile, true
}

func (m *MachineProfiler) writeCache(profile contracts.MachineProfile) error {
	if err := os.MkdirAll(m.config.HomeDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.config.ProfilePath, append(data, '\n'), 0o644)
}

// sysctl runs `sysctl -n <key>` and returns trimmed stdout.
func sysctl(ctx context.Context, key string) string {
	out, err := exec.CommandContext(ctx, "sysctl", "-n", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func swVers(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func probeCPU(ctx context.Context) contracts.CPUInfo {
	brand := sysctl(ctx, "machdep.cpu.brand_string")
	physical, _ := strconv.Atoi(sysctl(ctx, "hw.physicalcpu"))
	logical, _ := strconv.Atoi(sysctl(ctx, "hw.logicalcpu"))
	return contracts.CPUInfo{
		Brand:         brand,
		PhysicalCores: physical,
		LogicalCores:  logical,
	}
}

func probeMemory(ctx context.Context) contracts.MemInfo {
	raw := sysctl(ctx, "hw.memsize")
	bytes, _ := strconv.ParseInt(raw, 10, 64)
	return contracts.MemInfo{
		TotalBytes: bytes,
		TotalGB:    float64(bytes) / (1 << 30),
	}
}

// probeGPUs calls system_profiler with a JSON output flag to avoid screen
// capture dependencies. Returns nil when the call fails or times out.
func probeGPUs(ctx context.Context) []contracts.GPUInfo {
	type spDisplay struct {
		Model         string `json:"sppci_model"`
		VRAMShared    string `json:"spdisplays_vram_shared"`
		VRAMDedicated string `json:"spdisplays_vram"`
	}
	type spData struct {
		SPDisplaysDataType []spDisplay `json:"SPDisplaysDataType"`
	}

	out, err := exec.CommandContext(ctx, "system_profiler", "SPDisplaysDataType", "-json").Output()
	if err != nil {
		return nil
	}

	var data spData
	if err := json.Unmarshal(out, &data); err != nil {
		return nil
	}

	gpus := make([]contracts.GPUInfo, 0, len(data.SPDisplaysDataType))
	for _, d := range data.SPDisplaysDataType {
		gpu := contracts.GPUInfo{Name: d.Model}
		switch {
		case d.VRAMShared != "":
			gpu.VRAMType = "shared"
			gpu.VRAMGB = parseVRAMString(d.VRAMShared)
		case d.VRAMDedicated != "":
			gpu.VRAMType = "dedicated"
			gpu.VRAMGB = parseVRAMString(d.VRAMDedicated)
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

// parseVRAMString converts strings like "16384 MB" or "8 GB" to GB.
func parseVRAMString(s string) float64 {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(fields[1]) {
	case "MB":
		return val / 1024
	case "GB":
		return val
	default:
		return val
	}
}

// probeDisk reads capacity and availability for the root filesystem using df.
func probeDisk(ctx context.Context) contracts.DiskInfo {
	out, err := exec.CommandContext(ctx, "df", "-k", "/").Output()
	if err != nil {
		return contracts.DiskInfo{}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return contracts.DiskInfo{}
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return contracts.DiskInfo{}
	}

	totalKB, _ := strconv.ParseInt(fields[1], 10, 64)
	availKB, _ := strconv.ParseInt(fields[3], 10, 64)

	totalBytes := totalKB * 1024
	availBytes := availKB * 1024
	return contracts.DiskInfo{
		TotalBytes:     totalBytes,
		AvailableBytes: availBytes,
		TotalGB:        float64(totalBytes) / (1 << 30),
		AvailableGB:    float64(availBytes) / (1 << 30),
	}
}

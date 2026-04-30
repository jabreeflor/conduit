package core

import (
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestClassifyMachine(t *testing.T) {
	tests := []struct {
		name    string
		profile contracts.MachineProfile
		want    contracts.MachineClass
	}{
		{
			name: "high end apple silicon",
			profile: contracts.MachineProfile{
				CPU:    contracts.CPUInfo{Brand: "Apple M3 Max", PhysicalCores: 12},
				Memory: contracts.MemInfo{TotalGB: 64},
				GPU:    []contracts.GPUInfo{{Name: "Apple M3 Max", VRAMGB: 48, VRAMType: "shared"}},
			},
			want: contracts.MachineClassHighEnd,
		},
		{
			name: "mid range apple silicon",
			profile: contracts.MachineProfile{
				CPU:    contracts.CPUInfo{Brand: "Apple M2 Pro", PhysicalCores: 10},
				Memory: contracts.MemInfo{TotalGB: 16},
				GPU:    []contracts.GPUInfo{{Name: "Apple M2 Pro", VRAMGB: 16, VRAMType: "shared"}},
			},
			want: contracts.MachineClassMidRange,
		},
		{
			name: "entry level",
			profile: contracts.MachineProfile{
				CPU:    contracts.CPUInfo{Brand: "Apple M1", PhysicalCores: 8},
				Memory: contracts.MemInfo{TotalGB: 8},
			},
			want: contracts.MachineClassEntryLevel,
		},
		{
			name: "constrained memory",
			profile: contracts.MachineProfile{
				CPU:    contracts.CPUInfo{Brand: "Intel Core i5", PhysicalCores: 4},
				Memory: contracts.MemInfo{TotalGB: 4},
			},
			want: contracts.MachineClassConstrained,
		},
		{
			name: "constrained old intel",
			profile: contracts.MachineProfile{
				CPU:    contracts.CPUInfo{Brand: "Intel Core 2 Duo", PhysicalCores: 2},
				Memory: contracts.MemInfo{TotalGB: 16},
			},
			want: contracts.MachineClassConstrained,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyMachine(tc.profile)
			if got != tc.want {
				t.Fatalf("ClassifyMachine() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRecommendLocalModels_highEndRanksLargeGeneralModel(t *testing.T) {
	profile := contracts.MachineProfile{
		CPU:    contracts.CPUInfo{Brand: "Apple M3 Max", PhysicalCores: 12},
		Memory: contracts.MemInfo{TotalGB: 64},
		GPU:    []contracts.GPUInfo{{Name: "Apple M3 Max", VRAMGB: 48, VRAMType: "shared"}},
		Disk:   contracts.DiskInfo{AvailableGB: 200},
	}

	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{})

	if set.MachineClass != contracts.MachineClassHighEnd {
		t.Fatalf("MachineClass = %q, want high_end", set.MachineClass)
	}
	if len(set.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	top := set.Recommendations[0]
	if top.ID != "llama3:70b-q5" {
		t.Fatalf("top recommendation = %q, want llama3:70b-q5", top.ID)
	}
	if !top.Recommended {
		t.Fatal("top recommendation should be one-click recommended")
	}
	if top.EstimatedTokensPerSec != 15 {
		t.Fatalf("EstimatedTokensPerSec = %v, want 15", top.EstimatedTokensPerSec)
	}
	if !top.FitsAvailableDisk {
		t.Fatal("top recommendation should fit available disk")
	}
}

func TestRecommendLocalModels_midRangeIncludesManualBrowseCandidates(t *testing.T) {
	profile := contracts.MachineProfile{
		CPU:    contracts.CPUInfo{Brand: "Apple M3", PhysicalCores: 8},
		Memory: contracts.MemInfo{TotalGB: 24},
		GPU:    []contracts.GPUInfo{{Name: "Apple M3", VRAMGB: 24, VRAMType: "shared"}},
		Disk:   contracts.DiskInfo{AvailableGB: 40},
	}

	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{})

	if set.MachineClass != contracts.MachineClassMidRange {
		t.Fatalf("MachineClass = %q, want mid_range", set.MachineClass)
	}
	if got := set.Recommendations[0].ID; got != "mistral:7b-q6" {
		t.Fatalf("top recommendation = %q, want mistral:7b-q6", got)
	}
	if len(set.Recommendations) < 4 {
		t.Fatalf("expected manual browse choices, got %d", len(set.Recommendations))
	}
	for _, rec := range set.Recommendations {
		if rec.MachineClass == contracts.MachineClassHighEnd {
			t.Fatalf("mid-range machine should not receive high-end candidate %+v", rec)
		}
	}
}

func TestRecommendLocalModels_codeOptionPreselectsCodeCompanion(t *testing.T) {
	profile := contracts.MachineProfile{
		CPU:    contracts.CPUInfo{Brand: "Apple M2 Pro", PhysicalCores: 10},
		Memory: contracts.MemInfo{TotalGB: 32},
		Disk:   contracts.DiskInfo{AvailableGB: 100},
	}

	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{IncludeCodeModel: true})

	var codeRecommended contracts.LocalModelRecommendation
	for _, rec := range set.Recommendations {
		if rec.Use == contracts.LocalModelUseCode && rec.Recommended {
			codeRecommended = rec
			break
		}
	}
	if codeRecommended.ID != "qwen2.5-coder:7b-q6" {
		t.Fatalf("recommended code model = %q, want qwen2.5-coder:7b-q6", codeRecommended.ID)
	}
}

func TestRecommendLocalModels_constrainedReturnsFallback(t *testing.T) {
	profile := contracts.MachineProfile{
		CPU:    contracts.CPUInfo{Brand: "Intel Core i5", PhysicalCores: 2},
		Memory: contracts.MemInfo{TotalGB: 4},
	}

	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{})

	if set.MachineClass != contracts.MachineClassConstrained {
		t.Fatalf("MachineClass = %q, want constrained", set.MachineClass)
	}
	if len(set.Recommendations) != 0 {
		t.Fatalf("constrained machine recommendations = %d, want 0", len(set.Recommendations))
	}
	if set.FallbackReason == "" {
		t.Fatal("FallbackReason should guide users to external endpoints")
	}
	if !strings.Contains(set.FallbackReason, "external endpoint") {
		t.Fatalf("FallbackReason = %q, want external endpoint guidance", set.FallbackReason)
	}
}

func TestRecommendLocalModels_marksDiskShortage(t *testing.T) {
	profile := contracts.MachineProfile{
		CPU:    contracts.CPUInfo{Brand: "Apple M1", PhysicalCores: 8},
		Memory: contracts.MemInfo{TotalGB: 8},
		Disk:   contracts.DiskInfo{AvailableGB: 3},
	}

	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{})

	if len(set.Recommendations) == 0 {
		t.Fatal("expected entry-level recommendation")
	}
	if set.Recommendations[0].FitsAvailableDisk {
		t.Fatal("recommendation should be flagged when disk is too low")
	}
}

package core

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

type localModelCandidate struct {
	id             string
	name           string
	use            contracts.LocalModelUse
	minClass       contracts.MachineClass
	quantization   string
	downloadGB     float64
	footprintGB    float64
	baseTokensSec  float64
	classTokensSec map[contracts.MachineClass]float64
	notes          []string
}

var machineClassRank = map[contracts.MachineClass]int{
	contracts.MachineClassConstrained: 0,
	contracts.MachineClassEntryLevel:  1,
	contracts.MachineClassMidRange:    2,
	contracts.MachineClassHighEnd:     3,
}

var bundledLocalModelCandidates = []localModelCandidate{
	{
		id:           "llama3:70b-q5",
		name:         "Llama 3 70B",
		use:          contracts.LocalModelUseGeneral,
		minClass:     contracts.MachineClassHighEnd,
		quantization: "Q5",
		downloadGB:   40,
		footprintGB:  45,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassHighEnd: 15,
		},
		notes: []string{"Best quality general-purpose local model when memory allows."},
	},
	{
		id:           "qwen2.5-coder:32b-q4",
		name:         "Qwen2.5 Coder 32B",
		use:          contracts.LocalModelUseCode,
		minClass:     contracts.MachineClassHighEnd,
		quantization: "Q4",
		downloadGB:   20,
		footprintGB:  24,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassHighEnd: 24,
		},
		notes: []string{"Code-focused companion for high-memory Apple Silicon machines."},
	},
	{
		id:           "llama3:8b-q8",
		name:         "Llama 3 8B",
		use:          contracts.LocalModelUseGeneral,
		minClass:     contracts.MachineClassMidRange,
		quantization: "Q8",
		downloadGB:   8,
		footprintGB:  10,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassMidRange: 30,
			contracts.MachineClassHighEnd:  55,
		},
		notes: []string{"Balanced default for interactive local use."},
	},
	{
		id:           "mistral:7b-q6",
		name:         "Mistral 7B",
		use:          contracts.LocalModelUseGeneral,
		minClass:     contracts.MachineClassMidRange,
		quantization: "Q6",
		downloadGB:   5,
		footprintGB:  7,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassMidRange: 36,
			contracts.MachineClassHighEnd:  60,
		},
		notes: []string{"Fast fallback when disk or download size matters."},
	},
	{
		id:           "qwen2.5-coder:7b-q6",
		name:         "Qwen2.5 Coder 7B",
		use:          contracts.LocalModelUseCode,
		minClass:     contracts.MachineClassMidRange,
		quantization: "Q6",
		downloadGB:   6,
		footprintGB:  8,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassMidRange: 32,
			contracts.MachineClassHighEnd:  58,
		},
		notes: []string{"Code-focused recommendation for mainstream laptops."},
	},
	{
		id:           "phi3:mini-q4",
		name:         "Phi-3 Mini",
		use:          contracts.LocalModelUseGeneral,
		minClass:     contracts.MachineClassEntryLevel,
		quantization: "Q4",
		downloadGB:   2.4,
		footprintGB:  3.2,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassEntryLevel: 18,
			contracts.MachineClassMidRange:   42,
			contracts.MachineClassHighEnd:    70,
		},
		notes: []string{"Small, quick install with lower quality expectations."},
	},
	{
		id:           "llama3:8b-q4",
		name:         "Llama 3 8B",
		use:          contracts.LocalModelUseGeneral,
		minClass:     contracts.MachineClassEntryLevel,
		quantization: "Q4",
		downloadGB:   4.7,
		footprintGB:  6,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassEntryLevel: 10,
			contracts.MachineClassMidRange:   28,
			contracts.MachineClassHighEnd:    48,
		},
		notes: []string{"Better quality than tiny models, but slower on 8GB systems."},
	},
	{
		id:           "starcoder2:3b-q4",
		name:         "StarCoder2 3B",
		use:          contracts.LocalModelUseCode,
		minClass:     contracts.MachineClassEntryLevel,
		quantization: "Q4",
		downloadGB:   2,
		footprintGB:  2.8,
		classTokensSec: map[contracts.MachineClass]float64{
			contracts.MachineClassEntryLevel: 16,
			contracts.MachineClassMidRange:   38,
			contracts.MachineClassHighEnd:    64,
		},
		notes: []string{"Small code-focused model for low-memory machines."},
	},
}

// RecommendLocalModels ranks local model candidates using only the cached
// machine profile. It never calls external services; download sizes are bundled
// heuristics for the install UI.
func RecommendLocalModels(profile contracts.MachineProfile, opts contracts.LocalModelRecommendationOptions) contracts.LocalModelRecommendationSet {
	class := ClassifyMachine(profile)
	set := contracts.LocalModelRecommendationSet{
		MachineClass: class,
		GeneratedAt:  time.Now().UTC(),
	}

	if class == contracts.MachineClassConstrained {
		set.FallbackReason = "Local models are not recommended on machines with less than 8GB RAM or very old hardware; use an external endpoint fallback instead."
		return set
	}

	candidates := make([]contracts.LocalModelRecommendation, 0, len(bundledLocalModelCandidates))
	for _, candidate := range bundledLocalModelCandidates {
		if candidate.use == contracts.LocalModelUseCode && !opts.IncludeCodeModel {
			continue
		}
		if machineClassRank[candidate.minClass] > machineClassRank[class] {
			continue
		}
		rec := candidate.toRecommendation(class, profile)
		candidates = append(candidates, rec)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Use != candidates[j].Use {
			return candidates[i].Use == contracts.LocalModelUseGeneral
		}
		if candidates[i].MachineClass != candidates[j].MachineClass {
			return machineClassRank[candidates[i].MachineClass] > machineClassRank[candidates[j].MachineClass]
		}
		return candidates[i].EstimatedTokensPerSec > candidates[j].EstimatedTokensPerSec
	})

	markTopByUse(candidates)
	for i := range candidates {
		candidates[i].Rank = i + 1
	}
	set.Recommendations = candidates
	return set
}

// ClassifyMachine maps a hardware profile into the PRD §7.3 recommendation
// tiers using local-only heuristics.
func ClassifyMachine(profile contracts.MachineProfile) contracts.MachineClass {
	memGB := profile.Memory.TotalGB
	if memGB == 0 && profile.Memory.TotalBytes > 0 {
		memGB = float64(profile.Memory.TotalBytes) / (1 << 30)
	}
	if memGB < 8 || isVeryOldIntel(profile.CPU.Brand) {
		return contracts.MachineClassConstrained
	}
	if isAppleSilicon(profile) && memGB >= 64 {
		return contracts.MachineClassHighEnd
	}
	if memGB >= 48 && bestVRAMGB(profile) >= 16 {
		return contracts.MachineClassHighEnd
	}
	if memGB >= 16 {
		return contracts.MachineClassMidRange
	}
	return contracts.MachineClassEntryLevel
}

func (c localModelCandidate) toRecommendation(class contracts.MachineClass, profile contracts.MachineProfile) contracts.LocalModelRecommendation {
	tokensSec := c.classTokensSec[class]
	if tokensSec == 0 {
		tokensSec = c.baseTokensSec
	}
	if tokensSec == 0 {
		tokensSec = 8
	}

	fitsDisk := profile.Disk.AvailableGB == 0 || profile.Disk.AvailableGB >= c.footprintGB*1.15
	notes := append([]string(nil), c.notes...)
	if !fitsDisk {
		notes = append(notes, "Requires freeing disk space before installation.")
	}

	return contracts.LocalModelRecommendation{
		ID:                    c.id,
		Name:                  c.name,
		Use:                   c.use,
		MachineClass:          c.minClass,
		Quantization:          c.quantization,
		DownloadSizeGB:        round1(c.downloadGB),
		DiskFootprintGB:       round1(c.footprintGB),
		EstimatedTokensPerSec: estimateTokensPerSec(tokensSec, profile),
		FitsAvailableDisk:     fitsDisk,
		Notes:                 notes,
	}
}

func markTopByUse(recs []contracts.LocalModelRecommendation) {
	seen := map[contracts.LocalModelUse]bool{}
	for i := range recs {
		if !seen[recs[i].Use] {
			recs[i].Recommended = true
			seen[recs[i].Use] = true
		}
	}
}

func estimateTokensPerSec(base float64, profile contracts.MachineProfile) float64 {
	if !isAppleSilicon(profile) && bestVRAMGB(profile) == 0 {
		base *= 0.65
	}
	if profile.CPU.PhysicalCores > 0 && profile.CPU.PhysicalCores < 6 {
		base *= 0.8
	}
	return round1(base)
}

func isAppleSilicon(profile contracts.MachineProfile) bool {
	brand := strings.ToLower(profile.CPU.Brand)
	if strings.Contains(brand, "apple") {
		return true
	}
	for _, gpu := range profile.GPU {
		if strings.Contains(strings.ToLower(gpu.Name), "apple") {
			return true
		}
	}
	return false
}

func isVeryOldIntel(brand string) bool {
	brand = strings.ToLower(brand)
	return strings.Contains(brand, "core 2") || strings.Contains(brand, "2012")
}

func bestVRAMGB(profile contracts.MachineProfile) float64 {
	best := 0.0
	for _, gpu := range profile.GPU {
		if gpu.VRAMGB > best {
			best = gpu.VRAMGB
		}
	}
	return best
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

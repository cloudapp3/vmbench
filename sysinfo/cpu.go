package sysinfo

import (
	"runtime"
	"sort"
	"strings"

	"github.com/klauspost/cpuid/v2"
)

func defaultCacheSizes() map[string]int64 {
	return map[string]int64{}
}

func cpuidFeatureList() []string {
	features := make([]string, 0, 12)
	checks := []struct {
		name    string
		feature cpuid.FeatureID
	}{
		{name: "SSE4.2", feature: cpuid.SSE42},
		{name: "AVX", feature: cpuid.AVX},
		{name: "AVX2", feature: cpuid.AVX2},
		{name: "AVX-512F", feature: cpuid.AVX512F},
		{name: "AES-NI", feature: cpuid.AESNI},
		{name: "SHA", feature: cpuid.SHA},
		{name: "BMI1", feature: cpuid.BMI1},
		{name: "BMI2", feature: cpuid.BMI2},
		{name: "FMA3", feature: cpuid.FMA3},
		{name: "ATOMICS", feature: cpuid.ATOMICS},
	}
	for _, check := range checks {
		if cpuid.CPU.Supports(check.feature) {
			features = append(features, check.name)
		}
	}
	if runtime.GOARCH == "arm64" && !containsFold(features, "NEON") {
		features = append(features, "NEON")
	}
	sort.Strings(features)
	return features
}

func detectMicroArch(model, arch string, features []string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	arch = strings.ToLower(strings.TrimSpace(arch))
	if arch == "arm64" {
		switch {
		case strings.Contains(model, "apple m1"):
			return "Apple M1"
		case strings.Contains(model, "apple m2"):
			return "Apple M2"
		case strings.Contains(model, "apple m3"):
			return "Apple M3"
		case strings.Contains(model, "apple m4"):
			return "Apple M4"
		case strings.Contains(model, "graviton3"):
			return "Neoverse V1"
		default:
			return "unknown"
		}
	}

	hasAVX512 := containsFold(features, "AVX-512F")
	hasAVX2 := containsFold(features, "AVX2")
	switch {
	case strings.Contains(model, "7950x") || strings.Contains(model, "zen 4"):
		return "Zen 4"
	case strings.Contains(model, "ryzen") && hasAVX2:
		return "Zen"
	case strings.Contains(model, "14900") || strings.Contains(model, "13900") || strings.Contains(model, "raptor"):
		return "Raptor Lake"
	case strings.Contains(model, "12900") || strings.Contains(model, "alder"):
		return "Alder Lake"
	case hasAVX512:
		return "AVX-512 class"
	case hasAVX2:
		return "AVX2 class"
	default:
		return "unknown"
	}
}

func containsFold(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

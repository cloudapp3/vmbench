package sysinfo

import (
	"fmt"
	"sort"
	"strings"
)

// OversellSignal is a host capability that lets a provider pack more tenants
// than physical memory supports. Detection is evidence, not a verdict: On
// says whether the capability is active here, Risk says whether On means
// oversell exposure for the buyer (Off is therefore reassurance, not risk).
type OversellSignal struct {
	Key   string `json:"key"`   // stable id: "balloon" | "ksm"
	State string `json:"state"` // raw evidence word: present/absent/enabled/disabled
	On    bool   `json:"on"`    // capability detected on this host
	Risk  bool   `json:"risk"`  // true when On marks an oversell signal
}

// OversellSignals maps raw platform evidence (virtio balloon, KSM) into
// buyer-facing oversell indicators. Unsupported/unknown evidence is omitted
// so bare-metal hosts show nothing instead of fake reassurance.
func (d PlatformDiagnostics) OversellSignals() []OversellSignal {
	signals := make([]OversellSignal, 0, 2)
	switch d.VirtioBalloon {
	case "present":
		signals = append(signals, OversellSignal{Key: "balloon", State: "present", On: true, Risk: true})
	case "absent":
		signals = append(signals, OversellSignal{Key: "balloon", State: "absent", On: false})
	}
	switch d.KSM {
	case "enabled":
		signals = append(signals, OversellSignal{Key: "ksm", State: "enabled", On: true, Risk: true})
	case "disabled":
		signals = append(signals, OversellSignal{Key: "ksm", State: "disabled", On: false})
	}
	return signals
}

// OversellSignalsText renders the signals as one plain-text line, e.g.
// "balloon=present (!) / ksm=disabled"; "(!)" marks buyer-facing risk.
// Empty when there is no evidence, so text surfaces can skip the line.
func (d PlatformDiagnostics) OversellSignalsText() string {
	signals := d.OversellSignals()
	if len(signals) == 0 {
		return ""
	}
	parts := make([]string, 0, len(signals))
	for _, signal := range signals {
		text := signal.Key + "=" + signal.State
		if signal.Risk {
			text += " (!)"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " / ")
}

// PrimaryNIC renders the primary physical NIC as display text, e.g.
// "virtio_net (1af4:1000)". Empty when no device-backed interface exists —
// renderers skip the line entirely.
func (n NetworkInfo) PrimaryNIC() string {
	driver := strings.TrimSpace(n.PrimaryDriver)
	pci := strings.TrimSpace(n.PrimaryPCI)
	switch {
	case driver != "" && pci != "":
		return driver + " (" + pci + ")"
	default:
		return driver + pci
	}
}

// cacheOrder fixes the display order of the well-known cache levels; unknown
// levels sort after them.
var cacheOrder = []string{"L1d", "L1i", "L2", "L3"}

// FormatCacheLine renders cache sizes in fixed L1d/L1i/L2/L3 order with
// binary units, e.g. "L1d 32 KiB, L1i 32 KiB, L2 4 MiB, L3 16 MiB". Levels
// missing from the map are skipped; an empty map renders as "".
func FormatCacheLine(sizes map[string]int64) string {
	if len(sizes) == 0 {
		return ""
	}
	keys := make([]string, 0, len(sizes))
	for key := range sizes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, pj := cachePos(keys[i]), cachePos(keys[j])
		if pi != pj {
			return pi < pj
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+" "+formatCacheBytes(sizes[key]))
	}
	return strings.Join(parts, ", ")
}

func cachePos(key string) int {
	for i, known := range cacheOrder {
		if key == known {
			return i
		}
	}
	return len(cacheOrder)
}

func formatCacheBytes(value int64) string {
	switch {
	case value >= 1<<20:
		if value%(1<<20) == 0 {
			return fmt.Sprintf("%d MiB", value/(1<<20))
		}
		return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
	case value >= 1<<10:
		return fmt.Sprintf("%d KiB", value/(1<<10))
	default:
		return fmt.Sprintf("%d B", value)
	}
}

package vmbench

import (
	"os"
	"regexp"
	"strings"
	"time"

	gbbench "github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/catalog"
)

const defaultTimeout = 5 * time.Minute

const (
	ScopeHardware = catalog.ScopeHardware
)

// Options controls vmbench execution.
type Options struct {
	DiskPath      string
	TraceTarget   string
	Timeout       time.Duration
	Iterations    int
	Filter        string
	OnEvent       EventHandler
	Engine        string   // "external"; legacy "native"/"full" values are treated as external
	HardwareTools []string // external hardware tool IDs
}

func prepareOptions(opts Options) (Options, string, []string) {
	norm := opts
	warnings := make([]string, 0, 2)
	if strings.TrimSpace(norm.DiskPath) == "" {
		norm.DiskPath = os.TempDir()
	}
	if norm.Timeout <= 0 {
		norm.Timeout = defaultTimeout
	}
	if norm.Iterations <= 0 {
		norm.Iterations = 3
	}
	if norm.Iterations > 9 {
		norm.Iterations = 9
	}
	switch norm.Engine {
	case "", "external":
		norm.Engine = "external"
	case "native", "full":
		warnings = append(warnings, "legacy engine "+norm.Engine+" ignored: hardware benchmarks use external tools only")
		norm.Engine = "external"
	default:
		norm.Engine = "external"
	}
	filterExpr := strings.TrimSpace(norm.Filter)
	if filterExpr != "" {
		if _, err := regexp.Compile(filterExpr); err != nil {
			warnings = append(warnings, "invalid filter regex: no workloads selected: "+err.Error())
			filterExpr = "a^"
			emitEvent(norm, Event{Kind: EventBenchLog, Message: warnings[len(warnings)-1]})
		}
	}
	norm.HardwareTools = catalog.StandardizeHardwareTools(norm.HardwareTools)
	if len(norm.HardwareTools) == 0 && len(opts.HardwareTools) == 0 {
		norm.HardwareTools = catalog.DefaultHardwareTools()
	}
	return norm, filterExpr, warnings
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func buildWorkloads(diskPath, filterExpr string, hardwareTools []string) []gbbench.Workload {
	defs := catalog.ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
	var filter *regexp.Regexp
	if filterExpr != "" {
		filter = regexp.MustCompile(filterExpr)
	}
	out := make([]gbbench.Workload, 0, len(defs))
	for _, def := range defs {
		if filter != nil && !filter.MatchString(def.Name) && !filter.MatchString(def.Category) {
			continue
		}
		out = append(out, def.Factory(diskPath))
	}
	return out
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func emitEvent(opts Options, event Event) {
	if opts.OnEvent == nil {
		return
	}
	opts.OnEvent(event)
}

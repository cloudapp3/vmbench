package vmbench

import (
	"os"
	"regexp"
	"strings"
	"time"

	gbbench "github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

const defaultTimeout = 5 * time.Minute

const (
	ScopeAll      = catalog.ScopeAll
	ScopeHardware = catalog.ScopeHardware
	ScopeNetwork  = catalog.ScopeNetwork
)

// Options controls vmbench execution.
type Options struct {
	DiskPath         string
	TraceTarget      string
	Timeout          time.Duration
	Iterations       int
	Filter           string
	OnEvent          EventHandler
	Mode             string   // "single"; legacy "multi"/"all" values run the external catalog once
	Engine           string   // "external"; legacy "native"/"full" values are treated as external
	Scope            string   // "hardware", "network", or "all"
	IperfHosts       []string // iperf3 target servers
	HardwareTools    []string // external hardware tool IDs
	CatalogSource    string   // embedded, auto, or an explicit manifest path
	CatalogRevision  string   // optional immutable revision pin
	CatalogCachePath string   // optional cache override used with source=auto
	ResolvedCatalog  *nodecatalog.Manifest
	CatalogWarning   string
}

func prepareOptions(opts Options) (Options, string, []string) {
	norm := opts
	warnings := make([]string, 0, 2)
	norm.IperfHosts = normalizeStringList(norm.IperfHosts)
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
	switch strings.ToLower(strings.TrimSpace(norm.Mode)) {
	case "", "single":
		norm.Mode = "single"
	case "multi", "all":
		warnings = append(warnings, "legacy mode "+norm.Mode+" runs the external workload catalog once; each tool defines its own concurrency")
		norm.Mode = "single"
	default:
		warnings = append(warnings, "invalid mode ignored: external workloads use tool-defined concurrency")
		norm.Mode = "single"
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
	switch strings.ToLower(strings.TrimSpace(norm.Scope)) {
	case ScopeAll:
		norm.Scope = ScopeAll
	case ScopeNetwork:
		norm.Scope = ScopeNetwork
	case ScopeHardware, "":
		norm.Scope = ScopeHardware
	default:
		warnings = append(warnings, "invalid scope ignored: defaulting to hardware")
		norm.Scope = ScopeHardware
	}
	filterExpr := strings.TrimSpace(norm.Filter)
	if filterExpr != "" {
		if _, err := regexp.Compile(filterExpr); err != nil {
			warnings = append(warnings, "invalid filter regex: no workloads selected: "+err.Error())
			filterExpr = "a^"
			emitEvent(norm, Event{Kind: EventBenchLog, Message: warnings[len(warnings)-1]})
		}
	}
	if norm.Scope == ScopeNetwork {
		norm.HardwareTools = nil
		return norm, filterExpr, warnings
	}
	if norm.Scope == ScopeHardware {
		norm.IperfHosts = nil
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

func buildWorkloads(diskPath, filterExpr, engine, scope string, iperfHosts, hardwareTools []string, manifest *nodecatalog.Manifest) []gbbench.Workload {
	var defs []catalog.Definition
	switch scope {
	case ScopeNetwork:
		if manifest != nil {
			defs = catalog.NetworkDefinitionsWithManifest(iperfHosts, manifest.Clone(), "v4")
		}
	case ScopeAll:
		defs = catalog.ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
		if manifest != nil {
			defs = append(defs, catalog.NetworkDefinitionsWithManifest(iperfHosts, manifest.Clone(), "v4")...)
		}
	default:
		defs = catalog.ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
	}
	_ = engine
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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/nodecatalog"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/suitecompare"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return runTUI(nil)
	}
	switch args[0] {
	case "run":
		return runBench(args[1:])
	case "suite":
		return runSuite(args[1:])
	case "tui":
		return runTUI(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "list":
		return runList(args[1:])
	case "nodes":
		return runNodes(args[1:])
	case "sysinfo":
		return runSysinfo(args[1:])
	case "compare":
		return runCompare(args[1:])
	case "history":
		return runHistory(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("vmbench %s\n", vmbench.Version)
		return 0
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.Join([]string{
		"vmbench — cross-platform CPU and network benchmark suite",
		"",
		"Usage:",
		"  vmbench run      [flags]              run benchmark workloads",
		"  vmbench suite    [flags]              run VPS composite suite",
		"  vmbench tui                          interactive TUI mode",
		"  vmbench mcp serve [flags]            expose MCP tools over stdio",
		"  vmbench list                          list available workloads",
		"  vmbench nodes     <command> [flags]   manage versioned network nodes",
		"  vmbench sysinfo   [--json]            show system information",
		"  vmbench compare   <a.json> <b.json>   compare two reports",
		"  vmbench history   <command>           manage local report history",
		"  vmbench version                       show version",
		"",
		"Run 'vmbench run --help', 'vmbench suite --help', or 'vmbench mcp serve --help' for detailed flags.",
	}, "\n"))
}

func runBench(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		iterations      int
		filter          string
		diskPath        string
		timeout         time.Duration
		jsonOut         string
		htmlOut         string
		quiet           bool
		mode            string
		scope           string
		iperfHost       string
		hardwareTool    string
		saveHistory     bool
		historyTag      string
		catalogSource   string
		catalogRevision string
		catalogCache    string
	)

	fs.IntVar(&iterations, "iterations", 3, "iterations per workload (1-9)")
	fs.StringVar(&filter, "filter", "", "run workloads matching regex")
	fs.StringVar(&diskPath, "disk-path", "", "disk benchmark temp directory")
	fs.DurationVar(&timeout, "timeout", 0, "per-workload timeout (default 5m)")
	fs.StringVar(&jsonOut, "json", "", "write JSON report to file")
	fs.StringVar(&htmlOut, "html", "", "write HTML report to file")
	fs.BoolVar(&quiet, "quiet", false, "suppress progress output")
	fs.StringVar(&mode, "mode", "single", "compatibility mode: single (legacy multi/all run the catalog once)")
	fs.StringVar(&scope, "scope", vmbench.ScopeHardware, "workload scope: hardware, network, or all")
	fs.StringVar(&iperfHost, "iperf-host", "", "iperf3 server for network test (comma-separated for multiple)")
	fs.StringVar(&hardwareTool, "hardware-tool", "", "hardware tools (comma-separated: sysbench,openssl,fio,dd,stream,mbw,geekbench,winsat; use all for every adapter)")
	fs.BoolVar(&saveHistory, "save-history", false, "save the report to local history")
	fs.StringVar(&historyTag, "history-tag", "", "optional tag used with --save-history")
	fs.StringVar(&catalogSource, "node-catalog", nodecatalog.SourceEmbedded, "node catalog source: embedded, auto, or a JSON path")
	fs.StringVar(&catalogRevision, "node-revision", "", "require an exact node catalog revision")
	fs.StringVar(&catalogCache, "node-cache", "", "cache path override used with --node-catalog auto")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench run [flags]",
			"",
			"Run benchmark workloads and produce a measured report.",
			"",
			"Flags:",
		}, "\n"))
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if iterations < 1 || iterations > 9 {
		fmt.Fprintln(os.Stderr, "error: --iterations must be between 1 and 9")
		return 2
	}
	if timeout < 0 {
		fmt.Fprintln(os.Stderr, "error: --timeout must not be negative")
		return 2
	}
	var filterRE *regexp.Regexp
	filter = strings.TrimSpace(filter)
	if filter != "" {
		var err error
		if filterRE, err = regexp.Compile(filter); err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid --filter regex: %v\n", err)
			return 2
		}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "single", "multi", "all":
	default:
		fmt.Fprintln(os.Stderr, "error: --mode must be one of: single, multi, all")
		return 2
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case vmbench.ScopeHardware, vmbench.ScopeNetwork, vmbench.ScopeAll:
	default:
		fmt.Fprintln(os.Stderr, "error: --scope must be one of: hardware, network, all")
		return 2
	}
	if scope == vmbench.ScopeNetwork || scope == vmbench.ScopeAll {
		fmt.Fprintln(os.Stderr, "notice: network scope is enabled and may transfer about 1.75 GB before optional speedtest/iperf workloads")
	}
	if strings.TrimSpace(historyTag) != "" && !saveHistory {
		fmt.Fprintln(os.Stderr, "error: --history-tag requires --save-history")
		return 2
	}
	if bad := invalidCSVValue(hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown hardware tool %q\n", bad)
		return 2
	}
	hardwareTools := parseHosts(hardwareTool)
	if strings.TrimSpace(hardwareTool) != "" && len(catalog.StandardizeHardwareTools(hardwareTools)) == 0 {
		fmt.Fprintf(os.Stderr, "error: no valid hardware tools in %q (available: %s)\n", hardwareTool, strings.Join(catalog.HardwareToolIDs(), ", "))
		return 2
	}

	runOptions, err := vmbench.NormalizeOptions(vmbench.Options{
		DiskPath:         diskPath,
		Timeout:          timeout,
		Iterations:       iterations,
		Filter:           filter,
		Mode:             mode,
		Engine:           "external",
		Scope:            scope,
		IperfHosts:       parseHosts(iperfHost),
		HardwareTools:    hardwareTools,
		OnEvent:          progressPrinter(!quiet),
		CatalogSource:    catalogSource,
		CatalogRevision:  catalogRevision,
		CatalogCachePath: catalogCache,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if runOptions.Scope == vmbench.ScopeHardware || runOptions.Scope == vmbench.ScopeAll {
		printHardwareToolPreflight(os.Stderr, runOptions.HardwareTools, filterRE)
	}
	report := vmbench.RunCore(context.Background(), runOptions)
	if saveHistory {
		record, err := saveHistoryReport(report, historyTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error saving history: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "history: saved %s\n", record.ID)
	}

	if jsonOut != "" {
		if err := writeFile(jsonOut, func(w io.Writer) error {
			return gbreport.WriteJSON(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			return 1
		}
	}
	if htmlOut != "" {
		if err := writeFile(htmlOut, func(w io.Writer) error {
			return gbreport.WriteHTML(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing HTML: %v\n", err)
			return 1
		}
	}

	if err := gbreport.WriteConsole(os.Stdout, report); err != nil {
		return 1
	}
	if gbreport.HasFailures(report) {
		return 1
	}
	return 0
}

func runSuite(args []string) int {
	fs := flag.NewFlagSet("suite", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		iterations      int
		filter          string
		diskPath        string
		timeout         time.Duration
		jsonOut         string
		htmlOut         string
		preset          string
		routePreset     string
		speedProvider   string
		hardwareTool    string
		iperfHost       string
		only            string
		skip            string
		ipVersion       string
		noHardware      bool
		noRoute         bool
		noPing          bool
		noSpeed         bool
		noIP            bool
		noMail          bool
		noMedia         bool
		saveHistory     bool
		historyTag      string
		catalogSource   string
		catalogRevision string
		catalogCache    string
		noNetworkInfo   bool
		noReachability  bool
		quiet           bool
	)

	fs.IntVar(&iterations, "iterations", 3, "iterations for hardware workloads")
	fs.StringVar(&filter, "filter", "", "run hardware workloads matching regex")
	fs.StringVar(&diskPath, "disk-path", "", "disk benchmark temp directory")
	fs.DurationVar(&timeout, "timeout", 0, "per-section timeout (default 5m; hardware applies it per workload)")
	fs.StringVar(&jsonOut, "json", "", "write suite JSON report to file")
	fs.StringVar(&htmlOut, "html", "", "write suite HTML report to file")
	fs.StringVar(&preset, "preset", "", "scenario preset: quick, website, proxy, mail")
	fs.StringVar(&routePreset, "route-presets", "", "comma-separated route presets (gz,bj,sh,cd,cernet,cstnet)")
	fs.StringVar(&speedProvider, "speed-provider", "", "speed providers (comma-separated: cloudflare,speedtest_net,speedtest_cn,iperf3)")
	fs.StringVar(&hardwareTool, "hardware-tool", "", "hardware tools (comma-separated: sysbench,openssl,fio,dd,stream,mbw,geekbench,winsat; use all for every adapter)")
	fs.StringVar(&iperfHost, "iperf-host", "", "iperf3 server(s) for speed section (comma-separated)")
	fs.StringVar(&only, "only", "", "run only selected sections (comma-separated: hardware,network_info,route,ping,speed,ip,reachability,mail,media)")
	fs.StringVar(&skip, "skip", "", "skip selected sections (comma-separated: hardware,network_info,route,ping,speed,ip,reachability,mail,media)")
	fs.StringVar(&ipVersion, "ip-version", "v4", "network IP version: v4, v6, or dual")
	fs.BoolVar(&noHardware, "no-hardware", false, "skip hardware section")
	fs.BoolVar(&noNetworkInfo, "no-network-info", false, "skip network identity section")
	fs.BoolVar(&noRoute, "no-route", false, "skip route section")
	fs.BoolVar(&noPing, "no-ping", false, "skip ping section")
	fs.BoolVar(&noSpeed, "no-speed", false, "skip speed section")
	fs.BoolVar(&noIP, "no-ip", false, "skip IP quality section")
	fs.BoolVar(&noMail, "no-mail", false, "skip mail port section")
	fs.BoolVar(&noMedia, "no-media", false, "skip media section")
	fs.BoolVar(&noReachability, "no-reachability", false, "skip website and Telegram reachability section")
	fs.BoolVar(&saveHistory, "save-history", false, "save the report to local history")
	fs.StringVar(&historyTag, "history-tag", "", "optional tag used with --save-history")
	fs.BoolVar(&quiet, "quiet", false, "suppress section progress output")
	fs.StringVar(&catalogSource, "node-catalog", nodecatalog.SourceEmbedded, "node catalog source: embedded, auto, or a JSON path")
	fs.StringVar(&catalogRevision, "node-revision", "", "require an exact node catalog revision")
	fs.StringVar(&catalogCache, "node-cache", "", "cache path override used with --node-catalog auto")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench suite [flags]",
			"",
			"Run hardware/network_info/route/ping/speed/ip_quality/reachability/mail/media as a composite VPS test suite.",
			"",
			"Presets:",
			formatPresetHelp(),
			"",
			"Speed providers:",
			formatSpeedProviderHelp(),
			"",
			"Hardware tools:",
			formatHardwareToolHelp(),
			"",
			"Flags:",
		}, "\n"))
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if iterations < 1 || iterations > 9 {
		fmt.Fprintln(os.Stderr, "error: --iterations must be between 1 and 9")
		return 2
	}
	if timeout < 0 {
		fmt.Fprintln(os.Stderr, "error: --timeout must not be negative")
		return 2
	}
	var filterRE *regexp.Regexp
	filter = strings.TrimSpace(filter)
	if filter != "" {
		var err error
		if filterRE, err = regexp.Compile(filter); err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid --filter regex: %v\n", err)
			return 2
		}
	}
	if strings.TrimSpace(historyTag) != "" && !saveHistory {
		fmt.Fprintln(os.Stderr, "error: --history-tag requires --save-history")
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(ipVersion)) {
	case "v4", "v6", "dual":
	default:
		fmt.Fprintln(os.Stderr, "error: --ip-version must be one of: v4, v6, dual")
		return 2
	}
	if bad := invalidSectionName(only); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown section %q in --only\n", bad)
		return 2
	}
	if bad := invalidSectionName(skip); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown section %q in --skip\n", bad)
		return 2
	}
	if bad := invalidCSVValue(routePreset, suite.StandardizeRoutePresets); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown route preset %q\n", bad)
		return 2
	}
	if bad := invalidCSVValue(speedProvider, suite.StandardizeSpeedProviders); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown speed provider %q\n", bad)
		return 2
	}
	if bad := invalidCSVValue(hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "error: unknown hardware tool %q\n", bad)
		return 2
	}

	routePresets := parseHosts(routePreset)
	speedProviders := parseHosts(speedProvider)
	hardwareTools := parseHosts(hardwareTool)
	if strings.TrimSpace(speedProvider) != "" && len(suite.StandardizeSpeedProviders(speedProviders)) == 0 {
		fmt.Fprintf(os.Stderr, "error: no valid speed providers in %q (available: %s)\n", speedProvider, strings.Join(suite.SpeedProviderIDs(), ", "))
		return 2
	}
	if strings.TrimSpace(hardwareTool) != "" && len(catalog.StandardizeHardwareTools(hardwareTools)) == 0 {
		fmt.Fprintf(os.Stderr, "error: no valid hardware tools in %q (available: %s)\n", hardwareTool, strings.Join(catalog.HardwareToolIDs(), ", "))
		return 2
	}
	sections := suite.DefaultSections()
	if strings.TrimSpace(preset) != "" {
		spec, ok := suite.LookupPreset(preset)
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown suite preset %q (available: %s)\n", preset, strings.Join(suite.PresetIDs(), ", "))
			return 2
		}
		sections = spec.Sections
		if strings.TrimSpace(ipVersion) == "" && spec.IPVersion != "" {
			ipVersion = spec.IPVersion
		}
		if len(routePresets) == 0 && len(spec.RoutePresets) > 0 {
			routePresets = spec.RoutePresets
		}
	}
	if strings.TrimSpace(only) != "" {
		var err error
		sections, err = suite.ApplySectionNames(suite.SectionSelector{}, parseHosts(only), true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}
	if strings.TrimSpace(skip) != "" {
		var err error
		sections, err = suite.ApplySectionNames(sections, parseHosts(skip), false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}
	if noHardware {
		sections.Hardware = false
	}
	if noNetworkInfo {
		sections.NetworkInfo = false
	}
	if noRoute {
		sections.Route = false
	}
	if noPing {
		sections.Ping = false
	}
	if noSpeed {
		sections.Speed = false
	}
	if noIP {
		sections.IPQuality = false
	}
	if noMail {
		sections.Mail = false
	}
	if noMedia {
		sections.Media = false
	}
	if noReachability {
		sections.Reachability = false
	}
	if !sections.AnyEnabled() {
		fmt.Fprintln(os.Stderr, "error: at least one suite section must remain enabled")
		return 2
	}

	suiteOptions, err := suite.NormalizeOptions(suite.Options{
		Iterations:       iterations,
		Filter:           filter,
		DiskPath:         diskPath,
		Timeout:          timeout,
		Preset:           preset,
		RoutePresets:     routePresets,
		SpeedProviders:   speedProviders,
		HardwareTools:    hardwareTools,
		Sections:         sections,
		IperfHosts:       parseHosts(iperfHost),
		IPVersion:        ipVersion,
		CatalogSource:    catalogSource,
		CatalogRevision:  catalogRevision,
		CatalogCachePath: catalogCache,
		OnEvent:          suiteProgressPrinter(!quiet),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if suiteOptions.Sections.Hardware {
		printHardwareToolPreflight(os.Stderr, suiteOptions.HardwareTools, filterRE)
	}
	report := suite.Run(context.Background(), suiteOptions)
	if saveHistory {
		record, err := saveHistoryReport(report, historyTag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error saving history: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "history: saved %s\n", record.ID)
	}

	if jsonOut != "" {
		if err := writeFile(jsonOut, func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			return 1
		}
	}
	if htmlOut != "" {
		if err := writeFile(htmlOut, func(w io.Writer) error {
			return suite.WriteHTML(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "error writing HTML: %v\n", err)
			return 1
		}
	}
	if err := suite.WriteConsole(os.Stdout, report); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if report.HasFailures() {
		return 1
	}
	return 0
}

func formatPresetHelp() string {
	var b strings.Builder
	for i, spec := range suite.Presets() {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "  %-8s %s (%s)", spec.ID, spec.Description, spec.Sections.String())
	}
	return b.String()
}

func formatSpeedProviderHelp() string {
	var b strings.Builder
	for i, spec := range suite.SpeedProviders() {
		if i > 0 {
			b.WriteByte('\n')
		}
		if spec.Requires != "" {
			fmt.Fprintf(&b, "  %-13s %s Requires: %s.", spec.ID, spec.Description, spec.Requires)
			continue
		}
		fmt.Fprintf(&b, "  %-13s %s", spec.ID, spec.Description)
	}
	return b.String()
}

func formatHardwareToolHelp() string {
	var b strings.Builder
	for i, spec := range catalog.HardwareTools() {
		if i > 0 {
			b.WriteByte('\n')
		}
		defaultLabel := ""
		if spec.Default {
			defaultLabel = " Default."
		}
		fmt.Fprintf(&b, "  %-10s %s%s", spec.ID, spec.Description, defaultLabel)
	}
	return b.String()
}

func progressPrinter(enabled bool) vmbench.EventHandler {
	if !enabled {
		return nil
	}
	var prev string
	return func(ev vmbench.Event) {
		switch ev.Kind {
		case vmbench.EventSuiteStart:
			fmt.Fprintf(os.Stderr, "  %-28s ", ev.Workload)
		case vmbench.EventSuiteDone:
			fmt.Fprintf(os.Stderr, "done  %s\n", ev.Metric)
		case vmbench.EventSuiteFail:
			fmt.Fprintf(os.Stderr, "FAIL  %s\n", ev.Err)
		case vmbench.EventBenchDone:
			if prev != "" {
				fmt.Fprintln(os.Stderr)
			}
			fmt.Fprintln(os.Stderr, "  Benchmark complete")
		}
		if ev.Message != "" {
			prev = ev.Message
		}
	}
}

func suiteProgressPrinter(enabled bool) suite.EventHandler {
	return suiteProgressPrinterTo(os.Stderr, enabled)
}

func suiteProgressPrinterTo(w io.Writer, enabled bool) suite.EventHandler {
	if !enabled || w == nil {
		return nil
	}
	return func(event suite.Event) {
		section := strings.TrimSpace(string(event.Section))
		switch event.Kind {
		case suite.EventSectionStart:
			fmt.Fprintf(w, "  [suite] %-16s running\n", section)
		case suite.EventSectionDone, suite.EventSectionFail:
			fmt.Fprintf(w, "  [suite] %-16s %-7s %s\n", section, firstNonEmpty(event.Status, "unknown"), strings.TrimSpace(event.Message))
		case suite.EventSuiteDone:
			fmt.Fprintf(w, "  [suite] complete         %-7s %s\n", firstNonEmpty(event.Status, "unknown"), strings.TrimSpace(event.Message))
		}
	}
}

func printHardwareToolPreflight(w io.Writer, tools []string, filter *regexp.Regexp) {
	if w == nil {
		return
	}
	missing := catalog.MissingHardwareToolsForFilter(tools, filter)
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(w, "notice: missing hardware tools: %s; affected workloads will be recorded as structured errors\n", strings.Join(missing, ", "))
	if runtime.GOOS == "linux" {
		packages := linuxHardwarePackages(missing)
		if len(packages) > 0 {
			fmt.Fprintf(w, "hint: Debian/Ubuntu: sudo apt-get install -y %s\n", strings.Join(packages, " "))
		}
	}
}

func linuxHardwarePackages(tools []string) []string {
	packages := make([]string, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		var pkg string
		switch tool {
		case catalog.HardwareToolSysbench:
			pkg = "sysbench"
		case catalog.HardwareToolOpenSSL:
			pkg = "openssl"
		case catalog.HardwareToolFio:
			pkg = "fio"
		case catalog.HardwareToolDD:
			pkg = "coreutils"
		case catalog.HardwareToolMBW:
			pkg = "mbw"
		}
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		packages = append(packages, pkg)
	}
	return packages
}

func runList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench list\n\nList available workloads.")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	defs := catalog.DefaultDefinitions(true)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "Workload\tCategory\tDescription")
	for _, def := range defs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", strings.TrimSpace(def.Name), strings.TrimSpace(def.Category), strings.TrimSpace(def.Description))
	}
	tw.Flush()
	return 0
}

func runSysinfo(args []string) int {
	fs := flag.NewFlagSet("sysinfo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "output as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench sysinfo [--json]",
			"",
			"Show system information for the current host.",
		}, "\n"))
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	info, warnings := sysinfo.Collect(context.Background())

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(struct {
			System   sysinfo.SystemInfo `json:"system"`
			Warnings []string           `json:"warnings,omitempty"`
		}{System: info, Warnings: warnings})
		return 0
	}

	writeSysinfoConsole(os.Stdout, info, warnings)
	return 0
}

func writeSysinfoConsole(w io.Writer, info sysinfo.SystemInfo, warnings []string) {
	line := strings.Repeat("=", 62)
	fmt.Fprintf(w, "%s\n  VMBench System Info\n%s\n", line, line)

	fmt.Fprintf(w, "  Host     : %s\n", firstNonEmpty(info.OS.Hostname, "-"))
	fmt.Fprintf(w, "  OS       : %s (%s)\n", firstNonEmpty(info.OS.Name, "-"), firstNonEmpty(info.OS.Kernel, "-"))
	fmt.Fprintf(w, "  CPU      : %s (%s, %dC/%dT)\n", firstNonEmpty(info.CPU.Model, "-"), firstNonEmpty(info.CPU.Arch, "-"), info.CPU.PhysicalCores, info.CPU.LogicalCores)
	fmt.Fprintf(w, "  Memory   : %.1f GB %s\n", float64(info.Memory.TotalBytes)/(1024*1024*1024), firstNonEmpty(info.Memory.Type, ""))
	if info.Virtualization.System != "" || info.Virtualization.Role != "" {
		fmt.Fprintf(w, "  Virtual  : %s (%s)\n", firstNonEmpty(info.Virtualization.System, "unknown"), firstNonEmpty(info.Virtualization.Role, "unknown"))
	}
	if len(info.CPU.Features) > 0 {
		features := slices.Clone(info.CPU.Features)
		sort.Strings(features)
		fmt.Fprintf(w, "  Features : %s\n", strings.Join(features, ", "))
	}
	if info.GPU != nil {
		fmt.Fprintf(w, "  GPU      : %s\n", firstNonEmpty(info.GPU.Model, "-"))
	}
	fmt.Fprintf(w, "  Go       : %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	if len(info.Disks) > 0 {
		fmt.Fprintf(w, "\n%s\n  Disks\n%s\n", line, line)
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "Device\tMountpoint\tFS\tTotal")
		disks := slices.Clone(info.Disks)
		sort.Slice(disks, func(i, j int) bool { return disks[i].Mountpoint < disks[j].Mountpoint })
		for _, d := range disks {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", firstNonEmpty(d.Device, "-"), firstNonEmpty(d.Mountpoint, "-"), firstNonEmpty(d.FSType, "-"), formatBytes(d.TotalBytes))
		}
		tw.Flush()
	}

	if len(info.Network.ActiveNames) > 0 {
		names := slices.Clone(info.Network.ActiveNames)
		sort.Strings(names)
		fmt.Fprintf(w, "\n  Network  : %d interfaces (%s)\n", info.Network.InterfaceCount, strings.Join(names, ", "))
	}

	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warning := range warnings {
			fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(warning))
		}
	}
}

func runCompare(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench compare <report1.json> <report2.json>\n\nCompare two benchmark reports side by side.")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "error: at least 2 JSON report files required")
		return 2
	}

	rawReports := make([][]byte, fs.NArg())
	for i, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			return 1
		}
		rawReports[i] = data
	}

	if err := writeReportComparison(os.Stdout, rawReports); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	return 0
}

func writeReportComparison(w io.Writer, rawReports [][]byte) error {
	if len(rawReports) < 2 {
		return fmt.Errorf("at least 2 reports required for comparison")
	}
	kind := history.Kind("")
	for i, raw := range rawReports {
		meta, err := history.Inspect(raw)
		if err != nil {
			return fmt.Errorf("report %d: %w", i+1, err)
		}
		if kind == "" {
			kind = meta.Kind
			continue
		}
		if meta.Kind != kind {
			return fmt.Errorf("cannot compare mixed report kinds: report 1 is %s, report %d is %s", kind, i+1, meta.Kind)
		}
	}
	if kind == history.KindSuite {
		return suitecompare.WriteCompare(w, rawReports)
	}
	docs := make([]gbreport.Document, len(rawReports))
	for i, raw := range rawReports {
		if err := json.Unmarshal(raw, &docs[i]); err != nil {
			return fmt.Errorf("parse run report %d: %w", i+1, err)
		}
	}
	return gbreport.WriteCompare(w, docs)
}

func saveHistoryReport(value any, tag string) (history.Record, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return history.Record{}, fmt.Errorf("encode report: %w", err)
	}
	store, err := history.Open("")
	if err != nil {
		return history.Record{}, err
	}
	return store.Add(data, tag)
}

func writeFile(path string, fn func(io.Writer) error) (returnErr error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := fn(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return os.Chmod(path, 0o600)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func formatBytes(value uint64) string {
	if value == 0 {
		return "-"
	}
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func parseHosts(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func invalidCSVValue(raw string, normalize func([]string) []string) string {
	for _, value := range parseHosts(raw) {
		if len(normalize([]string{value})) == 0 {
			return value
		}
	}
	return ""
}

func invalidSectionName(raw string) string {
	for _, value := range parseHosts(raw) {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "hardware", "hw",
			"network_info", "network-info", "netinfo", "identity", "network_identity", "network-identity",
			"route", "trace", "traceroute", "backtrace",
			"ping", "latency",
			"speed", "speedtest", "network",
			"ip", "ip_quality", "ip-quality", "quality",
			"reachability", "reach", "web", "website", "tg", "telegram",
			"mail", "email", "ports", "port",
			"media", "unlock", "streaming":
		default:
			return value
		}
	}
	return ""
}

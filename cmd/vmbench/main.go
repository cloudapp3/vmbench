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
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/suitecompare"
	"github.com/cloudapp3/vmbench/sysinfo"
	"github.com/cloudapp3/vmbench/tui"
)

// langFlag registers the shared --lang flag and applies it during parsing,
// so all subsequent output (usage included) follows the selected language.
func registerLangFlag(fs *flag.FlagSet) {
	fs.Var(new(langValue), "lang", i18n.T("cli.flag.lang"))
}

type langValue struct{}

func (l *langValue) String() string { return "" }

func (l *langValue) Set(v string) error {
	i18n.ApplyLang(v)
	return nil
}

// printErr wraps a (usually English, data-plane) error with the localized
// "error: " prefix.
func printErr(err error) {
	fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	i18n.Init("", tui.LoadConfig().Lang)
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
		fmt.Fprintf(os.Stderr, "%s\n\n", i18n.Tf("cli.error.unknownCommand", map[string]any{"Command": args[0]}))
		printUsage(os.Stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, strings.Join([]string{
		i18n.T("cli.usage.tagline"),
		"",
		i18n.T("cli.usage.header"),
		"  vmbench run      [flags]              " + i18n.T("cli.usage.cmdRun"),
		"  vmbench suite    [flags]              " + i18n.T("cli.usage.cmdSuite"),
		"  vmbench tui                          " + i18n.T("cli.usage.cmdTui"),
		"  vmbench mcp serve [flags]            " + i18n.T("cli.usage.cmdMcp"),
		"  vmbench list                          " + i18n.T("cli.usage.cmdList"),
		"  vmbench nodes     <command> [flags]   " + i18n.T("cli.usage.cmdNodes"),
		"  vmbench sysinfo   [--json]            " + i18n.T("cli.usage.cmdSysinfo"),
		"  vmbench compare   <a.json> <b.json>   " + i18n.T("cli.usage.cmdCompare"),
		"  vmbench history   <command>           " + i18n.T("cli.usage.cmdHistory"),
		"  vmbench version                       " + i18n.T("cli.usage.cmdVersion"),
		"",
		i18n.T("cli.usage.detailHint"),
	}, "\n"))
}

func runBench(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)

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

	fs.IntVar(&iterations, "iterations", 3, i18n.T("cli.flag.iterationsRun"))
	fs.StringVar(&filter, "filter", "", i18n.T("cli.flag.filterRun"))
	fs.StringVar(&diskPath, "disk-path", "", i18n.T("cli.flag.diskPath"))
	fs.DurationVar(&timeout, "timeout", 0, i18n.T("cli.flag.timeoutRun"))
	fs.StringVar(&jsonOut, "json", "", i18n.T("cli.flag.jsonRun"))
	fs.StringVar(&htmlOut, "html", "", i18n.T("cli.flag.htmlRun"))
	fs.BoolVar(&quiet, "quiet", false, i18n.T("cli.flag.quietRun"))
	fs.StringVar(&mode, "mode", "single", i18n.T("cli.flag.mode"))
	fs.StringVar(&scope, "scope", vmbench.ScopeHardware, i18n.T("cli.flag.scope"))
	fs.StringVar(&iperfHost, "iperf-host", "", i18n.T("cli.flag.iperfHostRun"))
	fs.StringVar(&hardwareTool, "hardware-tool", "", i18n.T("cli.flag.hardwareTool"))
	fs.BoolVar(&saveHistory, "save-history", false, i18n.T("cli.flag.saveHistory"))
	fs.StringVar(&historyTag, "history-tag", "", i18n.T("cli.flag.historyTag"))
	fs.StringVar(&catalogSource, "node-catalog", nodecatalog.SourceEmbedded, i18n.T("cli.flag.nodeCatalog"))
	fs.StringVar(&catalogRevision, "node-revision", "", i18n.T("cli.flag.nodeRevision"))
	fs.StringVar(&catalogCache, "node-cache", "", i18n.T("cli.flag.nodeCache"))

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench run [flags]",
			"",
			i18n.T("cli.usage.runDetail"),
			"",
			i18n.T("cli.usage.flags"),
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
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.iterationsRange"))
		return 2
	}
	if timeout < 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.timeoutNegative"))
		return 2
	}
	var filterRE *regexp.Regexp
	filter = strings.TrimSpace(filter)
	if filter != "" {
		var err error
		if filterRE, err = regexp.Compile(filter); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.badFilterRegex", map[string]any{"Err": err.Error()}))
			return 2
		}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "single", "multi", "all":
	default:
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.badMode"))
		return 2
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case vmbench.ScopeHardware, vmbench.ScopeNetwork, vmbench.ScopeAll:
	default:
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.badScope"))
		return 2
	}
	if scope == vmbench.ScopeNetwork || scope == vmbench.ScopeAll {
		fmt.Fprintln(os.Stderr, i18n.T("cli.notice.networkScopeTransfer"))
	}
	if strings.TrimSpace(historyTag) != "" && !saveHistory {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyTagRequiresSave"))
		return 2
	}
	if bad := invalidCSVValue(hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownHardwareTool", map[string]any{"Value": bad}))
		return 2
	}
	hardwareTools := parseHosts(hardwareTool)
	if strings.TrimSpace(hardwareTool) != "" && len(catalog.StandardizeHardwareTools(hardwareTools)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidHardwareTools", map[string]any{"Value": hardwareTool, "Available": strings.Join(catalog.HardwareToolIDs(), ", ")}))
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
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.savingHistory", map[string]any{"Err": err.Error()}))
			return 1
		}
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.notice.historySaved", map[string]any{"ID": record.ID}))
	}

	if jsonOut != "" {
		if err := writeFile(jsonOut, func(w io.Writer) error {
			return gbreport.WriteJSON(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingJSON", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if htmlOut != "" {
		if err := writeFile(htmlOut, func(w io.Writer) error {
			return gbreport.WriteHTML(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingHTML", map[string]any{"Err": err.Error()}))
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
	registerLangFlag(fs)

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
		mediaSet        string
		ipSource        string
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

	fs.IntVar(&iterations, "iterations", 3, i18n.T("cli.flag.iterationsSuite"))
	fs.StringVar(&filter, "filter", "", i18n.T("cli.flag.filterSuite"))
	fs.StringVar(&diskPath, "disk-path", "", i18n.T("cli.flag.diskPath"))
	fs.DurationVar(&timeout, "timeout", 0, i18n.T("cli.flag.timeoutSuite"))
	fs.StringVar(&jsonOut, "json", "", i18n.T("cli.flag.jsonSuite"))
	fs.StringVar(&htmlOut, "html", "", i18n.T("cli.flag.htmlSuite"))
	fs.StringVar(&preset, "preset", "", i18n.T("cli.flag.preset"))
	fs.StringVar(&routePreset, "route-presets", "", i18n.T("cli.flag.routePresets"))
	fs.StringVar(&speedProvider, "speed-provider", "", i18n.T("cli.flag.speedProvider"))
	fs.StringVar(&hardwareTool, "hardware-tool", "", i18n.T("cli.flag.hardwareTool"))
	fs.StringVar(&iperfHost, "iperf-host", "", i18n.T("cli.flag.iperfHostSuite"))
	fs.StringVar(&only, "only", "", i18n.T("cli.flag.only"))
	fs.StringVar(&skip, "skip", "", i18n.T("cli.flag.skip"))
	fs.StringVar(&ipVersion, "ip-version", "v4", i18n.T("cli.flag.ipVersion"))
	fs.StringVar(&mediaSet, "media-set", "all", i18n.T("cli.flag.mediaSet"))
	fs.StringVar(&ipSource, "ip-quality-source", "builtin", i18n.T("cli.flag.ipQualitySource"))
	fs.BoolVar(&noHardware, "no-hardware", false, i18n.T("cli.flag.noHardware"))
	fs.BoolVar(&noNetworkInfo, "no-network-info", false, i18n.T("cli.flag.noNetworkInfo"))
	fs.BoolVar(&noRoute, "no-route", false, i18n.T("cli.flag.noRoute"))
	fs.BoolVar(&noPing, "no-ping", false, i18n.T("cli.flag.noPing"))
	fs.BoolVar(&noSpeed, "no-speed", false, i18n.T("cli.flag.noSpeed"))
	fs.BoolVar(&noIP, "no-ip", false, i18n.T("cli.flag.noIP"))
	fs.BoolVar(&noMail, "no-mail", false, i18n.T("cli.flag.noMail"))
	fs.BoolVar(&noMedia, "no-media", false, i18n.T("cli.flag.noMedia"))
	fs.BoolVar(&noReachability, "no-reachability", false, i18n.T("cli.flag.noReachability"))
	fs.BoolVar(&saveHistory, "save-history", false, i18n.T("cli.flag.saveHistory"))
	fs.StringVar(&historyTag, "history-tag", "", i18n.T("cli.flag.historyTag"))
	fs.BoolVar(&quiet, "quiet", false, i18n.T("cli.flag.quietSuite"))
	fs.StringVar(&catalogSource, "node-catalog", nodecatalog.SourceEmbedded, i18n.T("cli.flag.nodeCatalog"))
	fs.StringVar(&catalogRevision, "node-revision", "", i18n.T("cli.flag.nodeRevision"))
	fs.StringVar(&catalogCache, "node-cache", "", i18n.T("cli.flag.nodeCache"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench suite [flags]",
			"",
			i18n.T("cli.usage.suiteDetail"),
			"",
			i18n.T("cli.usage.presets"),
			formatPresetHelp(),
			"",
			i18n.T("cli.usage.speedProviders"),
			formatSpeedProviderHelp(),
			"",
			i18n.T("cli.usage.hardwareTools"),
			formatHardwareToolHelp(),
			"",
			i18n.T("cli.usage.flags"),
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
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.iterationsRange"))
		return 2
	}
	if timeout < 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.timeoutNegative"))
		return 2
	}
	var filterRE *regexp.Regexp
	filter = strings.TrimSpace(filter)
	if filter != "" {
		var err error
		if filterRE, err = regexp.Compile(filter); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.badFilterRegex", map[string]any{"Err": err.Error()}))
			return 2
		}
	}
	if strings.TrimSpace(historyTag) != "" && !saveHistory {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyTagRequiresSave"))
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(ipVersion)) {
	case "v4", "v6", "dual":
	default:
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.badIPVersion"))
		return 2
	}
	if bad := invalidSectionName(only); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionOnly", map[string]any{"Value": bad}))
		return 2
	}
	if bad := invalidSectionName(skip); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionSkip", map[string]any{"Value": bad}))
		return 2
	}
	if bad := invalidCSVValue(routePreset, suite.StandardizeRoutePresets); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownRoutePreset", map[string]any{"Value": bad}))
		return 2
	}
	if bad := invalidCSVValue(speedProvider, suite.StandardizeSpeedProviders); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSpeedProvider", map[string]any{"Value": bad}))
		return 2
	}
	if bad := invalidCSVValue(hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownHardwareTool", map[string]any{"Value": bad}))
		return 2
	}
	if strings.TrimSpace(mediaSet) != "" {
		if _, err := suite.StandardizeMediaSet(mediaSet); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownMediaSet", map[string]any{"Value": mediaSet, "Available": strings.Join(suite.MediaSets(), ", ")}))
			return 2
		}
	}
	if bad := invalidCSVValue(ipSource, suite.StandardizeIPSources); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownIPSource", map[string]any{"Value": bad, "Available": strings.Join(suite.IPSourceIDs(), ", ")}))
		return 2
	}

	routePresets := parseHosts(routePreset)
	speedProviders := parseHosts(speedProvider)
	hardwareTools := parseHosts(hardwareTool)
	if strings.TrimSpace(speedProvider) != "" && len(suite.StandardizeSpeedProviders(speedProviders)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidSpeedProviders", map[string]any{"Value": speedProvider, "Available": strings.Join(suite.SpeedProviderIDs(), ", ")}))
		return 2
	}
	if strings.TrimSpace(hardwareTool) != "" && len(catalog.StandardizeHardwareTools(hardwareTools)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidHardwareTools", map[string]any{"Value": hardwareTool, "Available": strings.Join(catalog.HardwareToolIDs(), ", ")}))
		return 2
	}
	sections := suite.DefaultSections()
	if strings.TrimSpace(preset) != "" {
		spec, ok := suite.LookupPreset(preset)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownPreset", map[string]any{"Value": preset, "Available": strings.Join(suite.PresetIDs(), ", ")}))
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
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.noSectionsEnabled"))
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
		MediaSet:         mediaSet,
		IPSources:        parseHosts(ipSource),
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
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.savingHistory", map[string]any{"Err": err.Error()}))
			return 1
		}
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.notice.historySaved", map[string]any{"ID": record.ID}))
	}

	if jsonOut != "" {
		if err := writeFile(jsonOut, func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingJSON", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if htmlOut != "" {
		if err := writeFile(htmlOut, func(w io.Writer) error {
			return suite.WriteHTML(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingHTML", map[string]any{"Err": err.Error()}))
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
		fmt.Fprintf(&b, "  %-8s %s (%s)", spec.ID, spec.LocalizedDescription(), spec.Sections.String())
	}
	return b.String()
}

func formatSpeedProviderHelp() string {
	var b strings.Builder
	for i, spec := range suite.SpeedProviders() {
		if i > 0 {
			b.WriteByte('\n')
		}
		if requires := spec.LocalizedRequires(); requires != "" {
			fmt.Fprintf(&b, "  %-13s %s %s: %s.", spec.ID, spec.LocalizedDescription(), i18n.T("cli.usage.requires"), requires)
			continue
		}
		fmt.Fprintf(&b, "  %-13s %s", spec.ID, spec.LocalizedDescription())
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
			defaultLabel = " " + i18n.T("cli.usage.defaultTool")
		}
		fmt.Fprintf(&b, "  %-10s %s%s", spec.ID, spec.LocalizedDescription(), defaultLabel)
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
			fmt.Fprintf(os.Stderr, "  %s ", i18n.PadCells(ev.Workload, 28))
		case vmbench.EventSuiteDone:
			fmt.Fprintf(os.Stderr, "%s %s\n", i18n.PadCells(i18n.T("cli.progress.done"), 5), ev.Metric)
		case vmbench.EventSuiteFail:
			fmt.Fprintf(os.Stderr, "%s %s\n", i18n.PadCells(i18n.T("cli.progress.fail"), 5), ev.Err)
		case vmbench.EventBenchDone:
			if prev != "" {
				fmt.Fprintln(os.Stderr)
			}
			fmt.Fprintln(os.Stderr, "  "+i18n.T("cli.progress.benchmarkComplete"))
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
		status := i18n.StatusLabel(firstNonEmpty(event.Status, "unknown"))
		switch event.Kind {
		case suite.EventSectionStart:
			fmt.Fprintf(w, "  [suite] %s %s\n", i18n.PadCells(section, 16), i18n.T("cli.progress.running"))
		case suite.EventSectionDone, suite.EventSectionFail:
			fmt.Fprintf(w, "  [suite] %s %s %s\n", i18n.PadCells(section, 16), i18n.PadCells(status, 7), strings.TrimSpace(event.Message))
		case suite.EventSuiteDone:
			fmt.Fprintf(w, "  [suite] %s %s %s\n", i18n.PadCells(i18n.T("cli.progress.complete"), 16), i18n.PadCells(status, 7), strings.TrimSpace(event.Message))
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
	fmt.Fprintf(w, "%s\n", i18n.Tf("cli.notice.missingHardwareTools", map[string]any{"Tools": strings.Join(missing, ", ")}))
	if runtime.GOOS == "linux" {
		packages := linuxHardwarePackages(missing)
		if len(packages) > 0 {
			fmt.Fprintf(w, "%s\n", i18n.Tf("cli.notice.installHint", map[string]any{"Packages": strings.Join(packages, " ")}))
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
	registerLangFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench list\n\n"+i18n.T("cli.usage.listDetail"))
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	defs := catalog.DefaultDefinitions(true)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join([]string{i18n.T("cli.list.workload"), i18n.T("cli.list.category"), i18n.T("cli.list.description")}, "\t"))
	for _, def := range defs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", strings.TrimSpace(def.Name), strings.TrimSpace(def.Category), strings.TrimSpace(def.Description))
	}
	tw.Flush()
	return 0
}

func runSysinfo(args []string) int {
	fs := flag.NewFlagSet("sysinfo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)

	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonSysinfo"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench sysinfo [--json]",
			"",
			i18n.T("cli.usage.sysinfoDetail"),
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
	sysLabel := func(value string) string { return i18n.PadCells(i18n.T("cli.sysinfo."+value), 11) }
	line := strings.Repeat("=", 62)
	fmt.Fprintf(w, "%s\n  %s\n%s\n", line, i18n.T("cli.sysinfo.title"), line)

	fmt.Fprintf(w, "  %s : %s\n", sysLabel("host"), firstNonEmpty(info.OS.Hostname, "-"))
	fmt.Fprintf(w, "  %s : %s (%s)\n", sysLabel("os"), firstNonEmpty(info.OS.Name, "-"), firstNonEmpty(info.OS.Kernel, "-"))
	fmt.Fprintf(w, "  %s : %s (%s, %dC/%dT)\n", sysLabel("cpu"), firstNonEmpty(info.CPU.Model, "-"), firstNonEmpty(info.CPU.Arch, "-"), info.CPU.PhysicalCores, info.CPU.LogicalCores)
	fmt.Fprintf(w, "  %s : %.1f GB %s\n", sysLabel("memory"), float64(info.Memory.TotalBytes)/(1024*1024*1024), firstNonEmpty(info.Memory.Type, ""))
	if info.Virtualization.System != "" || info.Virtualization.Role != "" {
		fmt.Fprintf(w, "  %s : %s (%s)\n", sysLabel("virtual"), firstNonEmpty(info.Virtualization.System, i18n.Unknown()), firstNonEmpty(info.Virtualization.Role, i18n.Unknown()))
	}
	if len(info.CPU.Features) > 0 {
		features := slices.Clone(info.CPU.Features)
		sort.Strings(features)
		fmt.Fprintf(w, "  %s : %s\n", sysLabel("features"), strings.Join(features, ", "))
	}
	if info.GPU != nil {
		fmt.Fprintf(w, "  %s : %s\n", sysLabel("gpu"), firstNonEmpty(info.GPU.Model, "-"))
	}
	fmt.Fprintf(w, "  %s : %s (%s/%s)\n", sysLabel("go"), runtime.Version(), runtime.GOOS, runtime.GOARCH)

	if platform := info.Platform; platformHasEvidence(platform) {
		fmt.Fprintf(w, "\n%s\n  %s\n%s\n", line, i18n.T("cli.sysinfo.platformTitle"), line)
		if platform.UptimeSeconds > 0 {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("uptime"), formatDuration(platform.UptimeSeconds))
		}
		if platform.Load1 > 0 || platform.Load5 > 0 || platform.Load15 > 0 {
			fmt.Fprintf(w, "  %s : %.2f %.2f %.2f\n", sysLabel("load"), platform.Load1, platform.Load5, platform.Load15)
		}
		if platform.Timezone != "" {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("timezone"), platform.Timezone)
		}
		if platform.SwapTotalBytes > 0 {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("swap"), i18n.Tf("cli.sysinfo.swapUsed", map[string]any{"Used": formatBytes(platform.SwapUsedBytes), "Total": formatBytes(platform.SwapTotalBytes)}))
		}
		if platform.VirtioBalloon != "" {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("balloon"), platform.VirtioBalloon)
		}
		if platform.KSM != "" {
			if platform.KSMPagesShared > 0 {
				fmt.Fprintf(w, "  %s : %s\n", sysLabel("ksm"), i18n.Tf("cli.sysinfo.ksmPages", map[string]any{"Value": platform.KSM, "Pages": platform.KSMPagesShared}))
			} else {
				fmt.Fprintf(w, "  %s : %s\n", sysLabel("ksm"), platform.KSM)
			}
		}
		if platform.TCPCongestion != "" || platform.TCPQDisc != "" {
			fmt.Fprintf(w, "  %s : %s / %s\n", sysLabel("tcp"), firstNonEmpty(platform.TCPCongestion, "-"), firstNonEmpty(platform.TCPQDisc, "-"))
		}
		if platform.TCPRmemMax > 0 {
			fmt.Fprintf(w, "  %s : %s / %s / %s\n", sysLabel("tcpRmem"), formatBytesInt64(platform.TCPRmemMin), formatBytesInt64(platform.TCPRmemDefault), formatBytesInt64(platform.TCPRmemMax))
		}
		if platform.TCPWmemMax > 0 {
			fmt.Fprintf(w, "  %s : %s / %s / %s\n", sysLabel("tcpWmem"), formatBytesInt64(platform.TCPWmemMin), formatBytesInt64(platform.TCPWmemDefault), formatBytesInt64(platform.TCPWmemMax))
		}
		if platform.NestedVirtualization != "" {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("nestedVirt"), platform.NestedVirtualization)
		}
		if platform.HugePagesTotal > 0 {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("hugePages"), i18n.Tf("cli.sysinfo.hugePagesCount", map[string]any{"Total": platform.HugePagesTotal, "Free": platform.HugePagesFree, "Each": formatBytesInt64(platform.HugePageSizeBytes)}))
		}
		if platform.BootDisk != "" {
			fmt.Fprintf(w, "  %s : %s\n", sysLabel("bootDisk"), platform.BootDisk)
		}
	}

	if len(info.Disks) > 0 {
		fmt.Fprintf(w, "\n%s\n  %s\n%s\n", line, i18n.T("cli.sysinfo.disksTitle"), line)
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join([]string{i18n.T("cli.sysinfo.device"), i18n.T("cli.sysinfo.mountpoint"), i18n.T("cli.sysinfo.fs"), i18n.T("cli.sysinfo.total")}, "\t"))
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
		fmt.Fprintf(w, "\n  %s : %s\n", sysLabel("network"), i18n.Tf("cli.sysinfo.ifaceCount", map[string]any{"Count": info.Network.InterfaceCount, "Names": strings.Join(names, ", ")}))
	}

	if len(warnings) > 0 {
		fmt.Fprintf(w, "\n%s:\n", i18n.T("cli.sysinfo.warnings"))
		for _, warning := range warnings {
			fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(warning))
		}
	}
}

func runCompare(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench compare <report1.json> <report2.json>\n\n"+i18n.T("cli.usage.compareDetail"))
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.compareNeedsTwo"))
		return 2
	}

	rawReports := make([][]byte, fs.NArg())
	for i, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.readingReport", map[string]any{"Path": path, "Err": err.Error()}))
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

// formatBytesInt64 adapts signed byte counters for formatBytes.
func formatBytesInt64(value int64) string {
	if value <= 0 {
		return "-"
	}
	return formatBytes(uint64(value))
}

// formatDuration renders an uptime in days/hours/minutes.
func formatDuration(seconds uint64) string {
	if seconds == 0 {
		return "-"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// platformHasEvidence reports whether any platform diagnostic is populated.
func platformHasEvidence(p sysinfo.PlatformDiagnostics) bool {
	return p.UptimeSeconds > 0 || p.Load1 > 0 || p.Timezone != "" || p.SwapTotalBytes > 0 ||
		p.VirtioBalloon != "" || p.KSM != "" || p.TCPCongestion != "" || p.TCPRmemMax > 0 ||
		p.NestedVirtualization != "" || p.HugePagesTotal > 0 || p.BootDisk != ""
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

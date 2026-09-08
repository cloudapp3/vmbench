package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
)

// benchmarkFlags carries every root-level benchmark flag. The flag set is the
// union of the former `run` and `suite` subcommands (merged in v0.8.0): with
// no preset/only/skip selection the command runs the hardware benchmark only
// and produces a run report; any wider selection runs the composite suite.
type benchmarkFlags struct {
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
	saveHistory     bool
	historyTag      string
	catalogSource   string
	catalogRevision string
	catalogCache    string
	quiet           bool
}

func newBenchmarkFlags() *benchmarkFlags {
	return &benchmarkFlags{
		ipVersion:     "v4",
		mediaSet:      "all",
		ipSource:      "builtin",
		catalogSource: nodecatalog.SourceEmbedded,
	}
}

// newBenchmarkFlagSet registers the shared benchmark flags once, for both the
// root invocation and the root help text.
func newBenchmarkFlagSet(bf *benchmarkFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("vmbench", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	fs.IntVar(&bf.iterations, "iterations", 3, i18n.T("cli.flag.iterations"))
	fs.StringVar(&bf.filter, "filter", "", i18n.T("cli.flag.filter"))
	fs.StringVar(&bf.diskPath, "disk-path", "", i18n.T("cli.flag.diskPath"))
	fs.DurationVar(&bf.timeout, "timeout", 0, i18n.T("cli.flag.timeout"))
	fs.StringVar(&bf.jsonOut, "json", "", i18n.T("cli.flag.json"))
	fs.StringVar(&bf.htmlOut, "html", "", i18n.T("cli.flag.html"))
	fs.StringVar(&bf.preset, "preset", "", i18n.T("cli.flag.preset"))
	fs.StringVar(&bf.routePreset, "route-presets", "", i18n.T("cli.flag.routePresets"))
	fs.StringVar(&bf.speedProvider, "speed-provider", "", i18n.T("cli.flag.speedProvider"))
	fs.StringVar(&bf.hardwareTool, "hardware-tool", "", i18n.T("cli.flag.hardwareTool"))
	fs.StringVar(&bf.iperfHost, "iperf-host", "", i18n.T("cli.flag.iperfHost"))
	fs.StringVar(&bf.only, "only", "", i18n.T("cli.flag.only"))
	fs.StringVar(&bf.skip, "skip", "", i18n.T("cli.flag.skip"))
	fs.StringVar(&bf.ipVersion, "ip-version", bf.ipVersion, i18n.T("cli.flag.ipVersion"))
	fs.StringVar(&bf.mediaSet, "media-set", bf.mediaSet, i18n.T("cli.flag.mediaSet"))
	fs.StringVar(&bf.ipSource, "ip-quality-source", bf.ipSource, i18n.T("cli.flag.ipQualitySource"))
	fs.BoolVar(&bf.saveHistory, "save-history", false, i18n.T("cli.flag.saveHistory"))
	fs.StringVar(&bf.historyTag, "history-tag", "", i18n.T("cli.flag.historyTag"))
	fs.BoolVar(&bf.quiet, "quiet", false, i18n.T("cli.flag.quiet"))
	fs.StringVar(&bf.catalogSource, "node-catalog", bf.catalogSource, i18n.T("cli.flag.nodeCatalog"))
	fs.StringVar(&bf.catalogRevision, "node-revision", "", i18n.T("cli.flag.nodeRevision"))
	fs.StringVar(&bf.catalogCache, "node-cache", "", i18n.T("cli.flag.nodeCache"))
	fs.Usage = func() {
		printBenchUsage(fs)
	}
	return fs
}

// runBenchmark executes the root benchmark surface. Hardware-only selections
// keep the former `vmbench run` behavior (run report, workload exit codes);
// anything else keeps the former `vmbench suite` behavior.
func runBenchmark(args []string) int {
	bf := newBenchmarkFlags()
	fs := newBenchmarkFlagSet(bf)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	// A lone --lang only switches the interface language: open the TUI
	// instead of silently starting a benchmark.
	langOnly := true
	fs.Visit(func(f *flag.Flag) {
		if f.Name != "lang" {
			langOnly = false
		}
	})
	if langOnly {
		return runTUI(nil)
	}

	filterRE, code := bf.validate(fs)
	if code != 0 {
		return code
	}
	sections, routePresets, ipVersion, code := bf.resolveSections()
	if code != 0 {
		return code
	}
	if sections.HardwareOnly() {
		return bf.runHardwareOnly(filterRE)
	}
	return bf.runSuiteSections(sections, routePresets, ipVersion, filterRE)
}

// validate applies the shared preflight checks in stable order and returns
// the compiled --filter regex for the hardware-tool preflight.
func (bf *benchmarkFlags) validate(fs *flag.FlagSet) (*regexp.Regexp, int) {
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.noPositionalArgs"))
		return nil, 2
	}
	if bf.iterations < 1 || bf.iterations > 9 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.iterationsRange"))
		return nil, 2
	}
	if bf.timeout < 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.timeoutNegative"))
		return nil, 2
	}
	var filterRE *regexp.Regexp
	bf.filter = strings.TrimSpace(bf.filter)
	if bf.filter != "" {
		var err error
		if filterRE, err = regexp.Compile(bf.filter); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.badFilterRegex", map[string]any{"Err": err.Error()}))
			return nil, 2
		}
	}
	if strings.TrimSpace(bf.historyTag) != "" && !bf.saveHistory {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyTagRequiresSave"))
		return nil, 2
	}
	switch strings.ToLower(strings.TrimSpace(bf.ipVersion)) {
	case "v4", "v6", "dual":
	default:
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.badIPVersion"))
		return nil, 2
	}
	if bad := invalidSectionName(bf.only); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionOnly", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidSectionName(bf.skip); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionSkip", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.routePreset, suite.StandardizeRoutePresets); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownRoutePreset", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.speedProvider, suite.StandardizeSpeedProviders); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSpeedProvider", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownHardwareTool", map[string]any{"Value": bad}))
		return nil, 2
	}
	if strings.TrimSpace(bf.mediaSet) != "" {
		if _, err := suite.StandardizeMediaSet(bf.mediaSet); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownMediaSet", map[string]any{"Value": bf.mediaSet, "Available": strings.Join(suite.MediaSets(), ", ")}))
			return nil, 2
		}
	}
	if bad := invalidCSVValue(bf.ipSource, suite.StandardizeIPSources); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownIPSource", map[string]any{"Value": bad, "Available": strings.Join(suite.IPSourceIDs(), ", ")}))
		return nil, 2
	}
	speedProviders := parseHosts(bf.speedProvider)
	if strings.TrimSpace(bf.speedProvider) != "" && len(suite.StandardizeSpeedProviders(speedProviders)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidSpeedProviders", map[string]any{"Value": bf.speedProvider, "Available": strings.Join(suite.SpeedProviderIDs(), ", ")}))
		return nil, 2
	}
	hardwareTools := parseHosts(bf.hardwareTool)
	if strings.TrimSpace(bf.hardwareTool) != "" && len(catalog.StandardizeHardwareTools(hardwareTools)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidHardwareTools", map[string]any{"Value": bf.hardwareTool, "Available": strings.Join(catalog.HardwareToolIDs(), ", ")}))
		return nil, 2
	}
	return filterRE, 0
}

// resolveSections turns preset/--only/--skip into the effective section
// selection. The base selection is hardware-only, matching the default
// behavior of the merged root command.
func (bf *benchmarkFlags) resolveSections() (suite.SectionSelector, []string, string, int) {
	sections := suite.SectionSelector{Hardware: true}
	routePresets := parseHosts(bf.routePreset)
	ipVersion := bf.ipVersion
	if strings.TrimSpace(bf.preset) != "" {
		spec, ok := suite.LookupPreset(bf.preset)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownPreset", map[string]any{"Value": bf.preset, "Available": strings.Join(suite.PresetIDs(), ", ")}))
			return suite.SectionSelector{}, nil, "", 2
		}
		sections = spec.Sections
		if strings.TrimSpace(ipVersion) == "" && spec.IPVersion != "" {
			ipVersion = spec.IPVersion
		}
		if len(routePresets) == 0 && len(spec.RoutePresets) > 0 {
			routePresets = spec.RoutePresets
		}
	}
	if strings.TrimSpace(bf.only) != "" {
		var err error
		sections, err = suite.ApplySectionNames(suite.SectionSelector{}, parseHosts(bf.only), true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return suite.SectionSelector{}, nil, "", 2
		}
	}
	if strings.TrimSpace(bf.skip) != "" {
		var err error
		sections, err = suite.ApplySectionNames(sections, parseHosts(bf.skip), false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return suite.SectionSelector{}, nil, "", 2
		}
	}
	if !sections.AnyEnabled() {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.noSectionsEnabled"))
		return suite.SectionSelector{}, nil, "", 2
	}
	return sections, routePresets, ipVersion, 0
}

// runHardwareOnly is the former `vmbench run` execution path.
func (bf *benchmarkFlags) runHardwareOnly(filterRE *regexp.Regexp) int {
	runOptions, err := vmbench.NormalizeOptions(vmbench.Options{
		DiskPath:      bf.diskPath,
		Timeout:       bf.timeout,
		Iterations:    bf.iterations,
		Filter:        bf.filter,
		Engine:        "external",
		HardwareTools: parseHosts(bf.hardwareTool),
		OnEvent:       progressPrinter(!bf.quiet),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	printHardwareToolPreflight(os.Stderr, runOptions.HardwareTools, filterRE)
	report := vmbench.RunCore(context.Background(), runOptions)
	if !bf.saveHistoryIfRequested(report) {
		return 1
	}
	return bf.writeRunReport(report)
}

// runSuiteSections is the former `vmbench suite` execution path.
func (bf *benchmarkFlags) runSuiteSections(sections suite.SectionSelector, routePresets []string, ipVersion string, filterRE *regexp.Regexp) int {
	suiteOptions, err := suite.NormalizeOptions(suite.Options{
		Iterations:       bf.iterations,
		Filter:           bf.filter,
		DiskPath:         bf.diskPath,
		Timeout:          bf.timeout,
		Preset:           bf.preset,
		RoutePresets:     routePresets,
		SpeedProviders:   parseHosts(bf.speedProvider),
		HardwareTools:    parseHosts(bf.hardwareTool),
		Sections:         sections,
		IperfHosts:       parseHosts(bf.iperfHost),
		IPVersion:        ipVersion,
		MediaSet:         bf.mediaSet,
		IPSources:        parseHosts(bf.ipSource),
		CatalogSource:    bf.catalogSource,
		CatalogRevision:  bf.catalogRevision,
		CatalogCachePath: bf.catalogCache,
		OnEvent:          suiteProgressPrinter(!bf.quiet),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if suiteOptions.Sections.Hardware {
		printHardwareToolPreflight(os.Stderr, suiteOptions.HardwareTools, filterRE)
	}
	report := suite.Run(context.Background(), suiteOptions)
	if !bf.saveHistoryIfRequested(report) {
		return 1
	}
	return bf.writeSuiteReport(report)
}

func (bf *benchmarkFlags) saveHistoryIfRequested(report any) bool {
	if !bf.saveHistory {
		return true
	}
	record, err := saveHistoryReport(report, bf.historyTag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.savingHistory", map[string]any{"Err": err.Error()}))
		return false
	}
	fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.notice.historySaved", map[string]any{"ID": record.ID}))
	return true
}

func (bf *benchmarkFlags) writeRunReport(report vmbench.Report) int {
	if bf.jsonOut != "" {
		if err := writeFile(bf.jsonOut, func(w io.Writer) error {
			return gbreport.WriteJSON(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingJSON", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if bf.htmlOut != "" {
		if err := writeFile(bf.htmlOut, func(w io.Writer) error {
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

func (bf *benchmarkFlags) writeSuiteReport(report suite.SuiteReport) int {
	if bf.jsonOut != "" {
		if err := writeFile(bf.jsonOut, func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingJSON", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if bf.htmlOut != "" {
		if err := writeFile(bf.htmlOut, func(w io.Writer) error {
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

// invalidSectionName returns the first unrecognized section value, resolving
// aliases through suite.NormalizeSectionName so CLI and library agree.
func invalidSectionName(raw string) string {
	valid := suite.SectionIDs()
	for _, value := range parseHosts(raw) {
		if !slices.Contains(valid, suite.NormalizeSectionName(value)) {
			return value
		}
	}
	return ""
}

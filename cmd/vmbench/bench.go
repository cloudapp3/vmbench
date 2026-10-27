package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/cloudapp3/vmbench/redact"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/tui"
)

// benchmarkFlags carries every root-level benchmark flag. The flag set is the
// union of the former `run` and `checkup` subcommands (merged in v0.8.0): with
// no preset/only/skip selection the command runs the hardware benchmark only
// and produces a run report; any wider selection runs the composite checkup.
type benchmarkFlags struct {
	iterations      int
	filter          string
	diskPath        string
	timeout         time.Duration
	jsonOut         string
	jsonLegacyFlag  string
	htmlOut         string
	markdownOut     string
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
	redact          string
	saveHistory     bool
	historyTag      string
	catalogSource   string
	catalogRevision string
	catalogCache    string
	quiet           bool
	autoSwap        bool
}

func newBenchmarkFlags() *benchmarkFlags {
	return &benchmarkFlags{
		ipVersion:     "v4",
		mediaSet:      checkup.DefaultMediaSet(),
		ipSource:      "builtin",
		redact:        string(redact.Default),
		catalogSource: nodecatalog.SourceEmbedded,
	}
}

// newBenchmarkFlagSet registers the shared benchmark flags once, for both the
// root invocation and the root help text.
func newBenchmarkFlagSet(bf *benchmarkFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("vmbench", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerLangFlag(fs)
	fs.IntVar(&bf.iterations, "iterations", 3, i18n.T("cli.flag.iterations"))
	fs.StringVar(&bf.filter, "filter", "", i18n.T("cli.flag.filter"))
	fs.StringVar(&bf.diskPath, "disk-path", "", i18n.T("cli.flag.diskPath"))
	fs.Var(durationValue{&bf.timeout}, "timeout", i18n.T("cli.flag.timeout"))
	fs.StringVar(&bf.jsonOut, "json-out", "", i18n.T("cli.flag.jsonOut"))
	fs.StringVar(&bf.jsonLegacyFlag, "json", "", i18n.T("cli.flag.jsonLegacy"))
	fs.StringVar(&bf.htmlOut, "html", "", i18n.T("cli.flag.html"))
	fs.StringVar(&bf.markdownOut, "markdown", "", i18n.T("cli.flag.markdown"))
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
	fs.StringVar(&bf.redact, "redact", bf.redact, i18n.T("cli.flag.redact"))
	fs.BoolVar(&bf.saveHistory, "save-history", false, i18n.T("cli.flag.saveHistory"))
	fs.StringVar(&bf.historyTag, "history-tag", "", i18n.T("cli.flag.historyTag"))
	fs.BoolVar(&bf.quiet, "quiet", false, i18n.T("cli.flag.quiet"))
	fs.BoolVar(&bf.autoSwap, "auto-swap", false, i18n.T("cli.flag.autoSwap"))
	fs.StringVar(&bf.catalogSource, "node-catalog", bf.catalogSource, i18n.T("cli.flag.nodeCatalog"))
	fs.StringVar(&bf.catalogRevision, "node-revision", "", i18n.T("cli.flag.nodeRevision"))
	fs.StringVar(&bf.catalogCache, "node-cache", "", i18n.T("cli.flag.nodeCache"))
	fs.Usage = func() {
		printBenchUsage(fs.Output(), fs)
	}
	return fs
}

// durationValue parses the --timeout flag with a friendlier error than the
// stock duration flag's bare "parse error", spelling out the expected format.
type durationValue struct{ d *time.Duration }

func (v durationValue) String() string {
	if v.d == nil {
		return "0s"
	}
	return v.d.String()
}

func (v durationValue) Set(s string) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return errors.New(i18n.Tf("cli.error.badDuration", map[string]any{"Value": s}))
	}
	if d < 0 {
		return errors.New(i18n.T("cli.error.timeoutNegative"))
	}
	*v.d = d
	return nil
}

// resolveJSONOutput applies the --json-out flag plus the deprecated --json
// PATH spelling (kept working with a notice; subcommands use boolean --json).
func (bf *benchmarkFlags) resolveJSONOutput() int {
	if bf.jsonLegacyFlag == "" {
		return 0
	}
	if bf.jsonOut != "" {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.jsonBothFlags"))
		return 2
	}
	fmt.Fprintln(os.Stderr, i18n.T("cli.notice.jsonDeprecated"))
	bf.jsonOut = bf.jsonLegacyFlag
	return 0
}

// runBenchmark executes the root benchmark surface. Hardware-only selections
// keep the former `vmbench run` behavior (run report, workload exit codes);
// anything else keeps the former `vmbench checkup` behavior.
func runBenchmark(args []string) int {
	bf := newBenchmarkFlags()
	fs := newBenchmarkFlagSet(bf)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fs.SetOutput(os.Stdout)
			fs.Usage()
			return 0
		}
		fs.SetOutput(os.Stderr)
		fmt.Fprintln(os.Stderr, err)
		fs.Usage()
		return 2
	}
	// A lone --lang only switches the interface language: with a terminal it
	// opens the TUI (persisting the choice on exit, as always); without one
	// it persists the preference directly instead of failing to render.
	langOnly := true
	fs.Visit(func(f *flag.Flag) {
		if f.Name != "lang" {
			langOnly = false
		}
	})
	if langOnly {
		if !stdinIsInteractiveTTY() {
			cfg := tui.LoadConfig()
			if langFlagSeen == "auto" {
				cfg.Lang = "" // omitempty: back to auto
			} else {
				cfg.Lang = langFlagSeen
			}
			if err := tui.SaveConfig(cfg); err != nil {
				printErr(err)
				return 1
			}
			fmt.Fprintln(os.Stderr, i18n.Tf("cli.notice.langSet", map[string]any{"Lang": i18n.Lang()}))
			return 0
		}
		return runTUI(nil)
	}

	if code := bf.resolveJSONOutput(); code != 0 {
		return code
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
	return bf.runCheckupSections(sections, routePresets, ipVersion, filterRE)
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
	redactMode, err := redact.Parse(bf.redact)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.badRedact", map[string]any{"Value": bf.redact}))
		return nil, 2
	}
	bf.redact = string(redactMode)
	if redactMode == redact.ModeNone {
		fmt.Fprintln(os.Stderr, i18n.T("cli.notice.redactDisabled"))
	}
	if bad := invalidSectionName(bf.only); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionOnly", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidSectionName(bf.skip); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSectionSkip", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.routePreset, checkup.StandardizeRoutePresets); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownRoutePreset", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.speedProvider, checkup.StandardizeSpeedProviders); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownSpeedProvider", map[string]any{"Value": bad}))
		return nil, 2
	}
	if bad := invalidCSVValue(bf.hardwareTool, catalog.StandardizeHardwareTools); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownHardwareTool", map[string]any{"Value": bad}))
		return nil, 2
	}
	if strings.TrimSpace(bf.mediaSet) != "" {
		if _, err := checkup.StandardizeMediaSet(bf.mediaSet); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownMediaSet", map[string]any{"Value": bf.mediaSet, "Available": strings.Join(checkup.MediaSets(), ", ")}))
			return nil, 2
		}
	}
	if bad := invalidCSVValue(bf.ipSource, checkup.StandardizeIPSources); bad != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownIPSource", map[string]any{"Value": bad, "Available": strings.Join(checkup.IPSourceIDs(), ", ")}))
		return nil, 2
	}
	speedProviders := parseHosts(bf.speedProvider)
	if strings.TrimSpace(bf.speedProvider) != "" && len(checkup.StandardizeSpeedProviders(speedProviders)) == 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.noValidSpeedProviders", map[string]any{"Value": bf.speedProvider, "Available": strings.Join(checkup.SpeedProviderIDs(), ", ")}))
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
func (bf *benchmarkFlags) resolveSections() (checkup.SectionSelector, []string, string, int) {
	sections := checkup.SectionSelector{Hardware: true}
	routePresets := parseHosts(bf.routePreset)
	ipVersion := bf.ipVersion
	if strings.TrimSpace(bf.preset) != "" && strings.TrimSpace(bf.only) != "" {
		// --only silently replaced the whole preset selection before; make
		// the conflict explicit instead.
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.presetOnlyConflict"))
		return checkup.SectionSelector{}, nil, "", 2
	}
	if strings.TrimSpace(bf.preset) != "" {
		spec, ok := checkup.LookupPreset(bf.preset)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownPreset", map[string]any{"Value": bf.preset, "Available": strings.Join(checkup.PresetIDs(), ", ")}))
			return checkup.SectionSelector{}, nil, "", 2
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
		sections, err = checkup.ApplySectionNames(checkup.SectionSelector{}, parseHosts(bf.only), true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return checkup.SectionSelector{}, nil, "", 2
		}
	}
	if strings.TrimSpace(bf.skip) != "" {
		var err error
		sections, err = checkup.ApplySectionNames(sections, parseHosts(bf.skip), false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return checkup.SectionSelector{}, nil, "", 2
		}
	}
	if !sections.AnyEnabled() {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.noSectionsEnabled"))
		return checkup.SectionSelector{}, nil, "", 2
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
		Redact:        redact.Mode(bf.redact),
		OnEvent:       progressPrinter(!bf.quiet),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	printHardwareToolPreflight(os.Stderr, runOptions.HardwareTools, filterRE)
	defer maybeSetupTempSwap(bf, runOptions.HardwareTools, filterRE)()
	// SIGINT/SIGTERM cancel the run context instead of killing the process:
	// the partial report is still written, history still saved, and deferred
	// temp-file cleanup (fio/dd workloads, temp swap) still runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report := vmbench.RunCore(ctx, runOptions)
	interrupted := ctx.Err() != nil
	if !bf.saveHistoryIfRequested(report) {
		return 1
	}
	code := bf.writeRunReport(report)
	if interrupted {
		fmt.Fprintln(os.Stderr, i18n.T("cli.notice.interrupted"))
		return 130
	}
	return code
}

// runCheckupSections is the former `vmbench checkup` execution path.
func (bf *benchmarkFlags) runCheckupSections(sections checkup.SectionSelector, routePresets []string, ipVersion string, filterRE *regexp.Regexp) int {
	checkupOptions, err := checkup.NormalizeOptions(checkup.Options{
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
		Redact:           redact.Mode(bf.redact),
		CatalogSource:    bf.catalogSource,
		CatalogRevision:  bf.catalogRevision,
		CatalogCachePath: bf.catalogCache,
		OnEvent:          checkupProgressPrinter(!bf.quiet),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if checkupOptions.Sections.Hardware {
		printHardwareToolPreflight(os.Stderr, checkupOptions.HardwareTools, filterRE)
		defer maybeSetupTempSwap(bf, checkupOptions.HardwareTools, filterRE)()
	}
	// Same interrupt contract as the hardware path: cancel, keep the partial
	// report, run deferred cleanup, exit 130.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report := checkup.Run(ctx, checkupOptions)
	interrupted := ctx.Err() != nil
	if !bf.saveHistoryIfRequested(report) {
		return 1
	}
	code := bf.writeCheckupReport(report)
	if interrupted {
		fmt.Fprintln(os.Stderr, i18n.T("cli.notice.interrupted"))
		return 130
	}
	return code
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
	if bf.markdownOut != "" {
		if err := writeFile(bf.markdownOut, func(w io.Writer) error {
			return gbreport.WriteMarkdown(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingMarkdown", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if err := gbreport.WriteConsole(os.Stdout, report); err != nil {
		return 1
	}
	bf.printScoreHint()
	// Exit 3: the report is complete and usable, but some workloads failed;
	// exit 1 stays reserved for "no usable report" (write errors).
	if gbreport.HasFailures(report) {
		return 3
	}
	return 0
}

// printScoreHint points at the deterministic assessment command after a run
// wrote a JSON report, so the derived layer is discoverable without reading
// the docs.
func (bf *benchmarkFlags) printScoreHint() {
	if bf.jsonOut != "" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.notice.scoreHint", map[string]any{"Path": bf.jsonOut}))
	}
}

func (bf *benchmarkFlags) writeCheckupReport(report checkup.CheckupReport) int {
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
			return checkup.WriteHTML(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingHTML", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if bf.markdownOut != "" {
		if err := writeFile(bf.markdownOut, func(w io.Writer) error {
			return checkup.WriteMarkdown(w, report)
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.writingMarkdown", map[string]any{"Err": err.Error()}))
			return 1
		}
	}
	if err := checkup.WriteConsole(os.Stdout, report); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	bf.printScoreHint()
	// Exit 3 mirrors the run path: usable report, some sections failed.
	if report.HasFailures() {
		return 3
	}
	return 0
}

// invalidSectionName returns the first unrecognized section value, resolving
// aliases through checkup.NormalizeSectionName so CLI and library agree.
func invalidSectionName(raw string) string {
	valid := checkup.SectionIDs()
	for _, value := range parseHosts(raw) {
		if !slices.Contains(valid, checkup.NormalizeSectionName(value)) {
			return value
		}
	}
	return ""
}

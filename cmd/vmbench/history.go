package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
)

func runHistory(args []string) int {
	if len(args) == 0 {
		printHistoryUsage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "add":
		return runHistoryAdd(args[1:])
	case "list", "ls":
		return runHistoryList(args[1:])
	case "show":
		return runHistoryShow(args[1:])
	case "delete", "rm":
		return runHistoryDelete(args[1:])
	case "compare":
		return runHistoryCompare(args[1:])
	case "-h", "--help", "help":
		printHistoryUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.unknownHistoryCommand", map[string]any{"Command": args[0]}))
		printHistoryUsage(os.Stderr)
		return 2
	}
}

func printHistoryUsage(w *os.File) {
	fmt.Fprintln(w, strings.Join([]string{
		i18n.T("cli.usage.header"),
		"  vmbench history add FILE [--tag TAG]",
		"  vmbench history list",
		"  vmbench history show ID",
		"  vmbench history delete ID",
		"  vmbench history compare --last N",
	}, "\n"))
}

func runHistoryAdd(args []string) int {
	path, tag, help, err := parseHistoryAddArgs(args)
	if help {
		fmt.Fprintln(os.Stdout, "Usage: vmbench history add FILE [--tag TAG]")
		return 0
	}
	if err != nil {
		printErr(err)
		return 2
	}
	store, err := history.Open("")
	if err != nil {
		printErr(err)
		return 1
	}
	record, err := store.AddFile(path, tag)
	if err != nil {
		printErr(err)
		return 1
	}
	fmt.Fprintln(os.Stdout, record.ID)
	return 0
}

func parseHistoryAddArgs(args []string) (path, tag string, help bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return "", "", true, nil
		case arg == "--tag":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("--tag requires a value")
			}
			i++
			tag = args[i]
		case strings.HasPrefix(arg, "--tag="):
			tag = strings.TrimPrefix(arg, "--tag=")
		case strings.HasPrefix(arg, "-"):
			return "", "", false, fmt.Errorf("unknown flag %q", arg)
		case path == "":
			path = arg
		default:
			return "", "", false, fmt.Errorf("history add accepts exactly one report file")
		}
	}
	if path == "" {
		return "", "", false, fmt.Errorf("history add requires a report file")
	}
	return path, tag, false, nil
}

func runHistoryList(args []string) int {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			fmt.Fprintln(os.Stdout, "Usage: vmbench history list")
			return 0
		}
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyListNoArgs"))
		return 2
	}
	store, err := history.Open("")
	if err != nil {
		printErr(err)
		return 1
	}
	records, err := store.List()
	if err != nil {
		printErr(err)
		return 1
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join([]string{i18n.T("cli.nodes.colID"), i18n.T("cli.history.colKind"), i18n.T("cli.history.colReportTime"), i18n.T("cli.history.colTag")}, "\t"))
	for _, record := range records {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", record.ID, record.Kind, record.ReportTime.Local().Format(time.RFC3339), record.Tag)
	}
	_ = tw.Flush()
	return 0
}

func runHistoryShow(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stdout, "Usage: vmbench history show ID")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyShowOneID"))
		return 2
	}
	store, err := history.Open("")
	if err != nil {
		printErr(err)
		return 1
	}
	record, err := store.Get(args[0])
	if err != nil {
		printErr(err)
		return 1
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, record.Report, "", "  "); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.invalidStoredReport", map[string]any{"Err": err.Error()}))
		return 1
	}
	formatted.WriteByte('\n')
	_, _ = formatted.WriteTo(os.Stdout)
	return 0
}

func runHistoryDelete(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stdout, "Usage: vmbench history delete ID")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyDeleteOneID"))
		return 2
	}
	store, err := history.Open("")
	if err != nil {
		printErr(err)
		return 1
	}
	if err := store.Delete(args[0]); err != nil {
		printErr(err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.history.deleted", map[string]any{"ID": args[0]}))
	return 0
}

func runHistoryCompare(args []string) int {
	fs := flag.NewFlagSet("history compare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	last := 2
	fs.IntVar(&last, "last", 2, i18n.T("cli.flag.historyLast"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench history compare --last N")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyCompareNoPaths"))
		return 2
	}
	if last < 2 || last > 100 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.historyLastRange"))
		return 2
	}
	store, err := history.Open("")
	if err != nil {
		printErr(err)
		return 1
	}
	records, err := store.Latest(last)
	if err != nil {
		printErr(err)
		return 1
	}
	kind := records[0].Kind
	for _, record := range records[1:] {
		if record.Kind != kind {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.historyMixedKinds", map[string]any{"Count": strconv.Itoa(last), "KindA": kind, "KindB": record.Kind}))
			return 2
		}
	}
	raw := make([][]byte, len(records))
	for i, record := range records {
		raw[i] = record.Report
	}
	if err := writeReportComparison(os.Stdout, raw); err != nil {
		printErr(err)
		return 1
	}
	return 0
}

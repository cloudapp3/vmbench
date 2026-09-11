package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/uninstall"
)

// runUninstall implements `vmbench uninstall`: it prints the removal plan,
// asks for confirmation on a terminal, and removes owned directories before
// the running binary. Piped stdin (the install.sh delegation) skips the
// prompt, matching the installer's own non-interactive fallback.
func runUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var dryRun, yes, asJSON bool
	fs.BoolVar(&dryRun, "dry-run", false, i18n.T("cli.uninstall.flag.dryRun"))
	fs.BoolVar(&yes, "yes", false, i18n.T("cli.uninstall.flag.yes"))
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench uninstall [flags]",
			"",
			i18n.T("cli.usage.uninstallDetail"),
		}, "\n"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.uninstall.error.requires"))
		return 2
	}

	items, kept, warnings := uninstall.Plan()

	if asJSON {
		return writeUninstallJSON(dryRun, items, kept, warnings)
	}

	printUninstallPlan(os.Stdout, items, kept, warnings)
	if dryRun || len(items) == 0 {
		return 0
	}
	if !yes && stdinIsTerminal() {
		if !confirmUninstall(os.Stdout, os.Stdin) {
			fmt.Fprintln(os.Stdout, i18n.T("cli.uninstall.aborted"))
			return 0
		}
	}

	result := uninstall.Execute(os.Stdout, items)
	if len(result.Errors) > 0 {
		fmt.Fprintln(os.Stdout, i18n.T("cli.uninstall.output.errors"))
		for _, problem := range result.Errors {
			fmt.Fprintf(os.Stdout, "  - %s\n", problem)
		}
		return 1
	}
	fmt.Fprintln(os.Stdout, i18n.T("cli.uninstall.output.complete"))
	return 0
}

// printUninstallPlan renders the plan, kept locations, and warnings. Paths
// and warnings stay as discovered; only the fixed vocabulary is localized.
func printUninstallPlan(w io.Writer, items []uninstall.Item, kept []uninstall.Kept, warnings []string) {
	if len(warnings) > 0 {
		fmt.Fprintf(w, "%s\n", i18n.T("cli.uninstall.plan.warnings"))
		for _, warning := range warnings {
			fmt.Fprintf(w, "  ! %s\n", warning)
		}
		fmt.Fprintln(w)
	}
	if len(items) == 0 {
		fmt.Fprintln(w, i18n.T("cli.uninstall.plan.nothing"))
		return
	}
	fmt.Fprintln(w, i18n.T("cli.uninstall.plan.following"))
	for _, item := range items {
		fmt.Fprintf(w, "  - %s  (%s)\n", item.Path, uninstallItemNote(item))
	}
	if len(kept) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, i18n.T("cli.uninstall.plan.kept"))
		for _, entry := range kept {
			fmt.Fprintf(w, "  - %s  (%s)\n", entry.Path, uninstallKeptNote(entry))
		}
	}
	fmt.Fprintln(w)
}

func uninstallItemNote(item uninstall.Item) string {
	switch item.Kind {
	case uninstall.KindData:
		if item.Records > 0 {
			return i18n.Tf("cli.uninstall.item.data", map[string]any{"Count": item.Records})
		}
		return i18n.T("cli.uninstall.item.dataEmpty")
	case uninstall.KindConfigDir:
		return i18n.T("cli.uninstall.item.configDir")
	case uninstall.KindConfigFile:
		return i18n.T("cli.uninstall.item.configFile")
	case uninstall.KindCache:
		if len(item.Tools) > 0 {
			return i18n.Tf("cli.uninstall.item.cache", map[string]any{"Tools": strings.Join(item.Tools, ", ")})
		}
		return i18n.T("cli.uninstall.item.cacheEmpty")
	case uninstall.KindBinary:
		return i18n.T("cli.uninstall.item.binary")
	}
	return string(item.Kind)
}

func uninstallKeptNote(entry uninstall.Kept) string {
	if entry.Reason == uninstall.KeptReasonHistoryOverride {
		return i18n.T("cli.uninstall.kept.customHistory")
	}
	return entry.Reason
}

// confirmUninstall prompts on w and reads an explicit yes from r.
func confirmUninstall(w io.Writer, r io.Reader) bool {
	return confirmPrompt(w, r, "cli.uninstall.confirm")
}

// confirmPrompt prints the localized messageKey on w and reads an explicit
// yes from r; anything else, including EOF, counts as "no".
func confirmPrompt(w io.Writer, r io.Reader, messageKey string) bool {
	fmt.Fprint(w, i18n.T(messageKey))
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "是":
		return true
	}
	return false
}

// stdinIsTerminal reports whether stdin is an interactive terminal. Piped
// or redirected stdin skips the prompt — including /dev/null, which is a
// character device but is what install.sh delegates with when no terminal
// is available; prompting into it would abort every scripted uninstall.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(info, null) {
		return false
	}
	return true
}

// writeUninstallJSON reports the plan and, unless --dry-run, executes it.
// JSON mode is script-facing, so it never prompts.
func writeUninstallJSON(dryRun bool, items []uninstall.Item, kept []uninstall.Kept, warnings []string) int {
	type plannedItem struct {
		Kind    string   `json:"kind"`
		Path    string   `json:"path"`
		Records int      `json:"records,omitempty"`
		Tools   []string `json:"tools,omitempty"`
	}
	type keptPath struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	payload := struct {
		DryRun   bool          `json:"dry_run"`
		Planned  []plannedItem `json:"planned"`
		Kept     []keptPath    `json:"kept"`
		Warnings []string      `json:"warnings,omitempty"`
		Removed  []string      `json:"removed,omitempty"`
		Errors   []string      `json:"errors,omitempty"`
	}{
		DryRun:   dryRun,
		Planned:  make([]plannedItem, 0, len(items)),
		Kept:     make([]keptPath, 0, len(kept)),
		Warnings: warnings,
	}
	for _, item := range items {
		payload.Planned = append(payload.Planned, plannedItem{
			Kind:    string(item.Kind),
			Path:    item.Path,
			Records: item.Records,
			Tools:   item.Tools,
		})
	}
	for _, entry := range kept {
		payload.Kept = append(payload.Kept, keptPath{Path: entry.Path, Reason: entry.Reason})
	}
	exit := 0
	if !dryRun && len(items) > 0 {
		result := uninstall.Execute(io.Discard, items)
		payload.Removed = result.Removed
		payload.Errors = result.Errors
		if len(result.Errors) > 0 {
			exit = 1
		}
	}
	if code := writeNodeJSON(os.Stdout, payload); code != 0 {
		return code
	}
	return exit
}

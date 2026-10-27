package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/tui"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// stdinIsInteractiveTTY reports whether this process can render the TUI. docker
// exec without -t, CI, and piped stdin are the common non-interactive cases.
func stdinIsInteractiveTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// printNonInteractiveHint explains what to run instead of the TUI when there
// is no usable terminal (first-run experience in CI / docker exec / ssh cmd).
func printNonInteractiveHint() {
	fmt.Fprintln(os.Stderr, i18n.T("cli.error.tuiNoTTY"))
}

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		compareA string
		compareB string
	)
	// Hidden while FeatureCompare is off: the flags vanish from help and
	// passing them fails flag parsing, like any unknown flag.
	if vmbench.FeatureCompare {
		fs.StringVar(&compareA, "compare-a", "", "compare report A path")
		fs.StringVar(&compareB, "compare-b", "", "compare report B path")
	}
	registerLangFlag(fs)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg := tui.LoadConfig()
	theme.InitThemeFromEnv(cfg.Theme)

	if !stdinIsInteractiveTTY() {
		// No terminal to render on: say what to run instead instead of
		// failing with a raw "could not open a new TTY" error.
		printNonInteractiveHint()
		return 1
	}

	model := tui.NewModel(compareA, compareB)
	// An explicit --lang seeds the preference (and keeps persisting on exit,
	// as it always has); otherwise the config file's choice carries over and
	// "" stays auto: follow VMBENCH_LANG / system locale / default.
	pref := cfg.Lang
	if langFlagSeen != "" {
		pref = i18n.Lang() // ApplyLang already ran during flag parsing
	}
	model.SetLangPref(pref)

	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		if !stdinIsInteractiveTTY() {
			printNonInteractiveHint()
		}
		return 1
	}

	cfg.Theme = theme.Active.Name
	if m, ok := final.(tui.Model); ok {
		cfg.Lang = m.LangPref() // "" (auto) omits the field via omitempty
	}
	_ = tui.SaveConfig(cfg)
	return 0
}

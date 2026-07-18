package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/tui"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func runTUI(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		compareA string
		compareB string
	)
	fs.StringVar(&compareA, "compare-a", "", "compare report A path")
	fs.StringVar(&compareB, "compare-b", "", "compare report B path")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg := tui.LoadConfig()
	theme.InitThemeFromEnv(cfg.Theme)

	p := tea.NewProgram(tui.NewModel(compareA, compareB),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return 1
	}

	cfg.Theme = theme.Active.Name
	_ = tui.SaveConfig(cfg)
	return 0
}

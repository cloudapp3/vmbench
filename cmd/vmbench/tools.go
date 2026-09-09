package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/toolbin"
)

func runTools(args []string) int {
	if len(args) == 0 {
		printToolsUsage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "status":
		return runToolsStatus(args[1:])
	case "fetch":
		return runToolsFetch(args[1:])
	case "help", "-h", "--help":
		printToolsUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s\n\n", i18n.Tf("cli.error.toolsUnknownCommand", map[string]any{"Command": args[0]}))
		printToolsUsage(os.Stderr)
		return 2
	}
}

func printToolsUsage(w io.Writer) {
	fmt.Fprintln(w, strings.Join([]string{
		"Usage: vmbench tools <command> [flags]",
		"",
		i18n.T("cli.usage.toolsCommands"),
		"  status    " + i18n.T("cli.usage.toolsStatus"),
		"  fetch     " + i18n.T("cli.usage.toolsFetch"),
		"",
		i18n.T("cli.usage.toolsDetail"),
	}, "\n"))
}

// toolsStatusRows lists every registered hardware tool in display order with
// its resolution result, so status covers both provisionable and PATH-only
// tools.
func toolsStatusRows() []catalog.HardwareToolSpec {
	return catalog.HardwareTools()
}

func runToolsStatus(args []string) int {
	fs := flag.NewFlagSet("tools status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench tools status") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join([]string{
		i18n.T("cli.tools.colTool"),
		i18n.T("cli.tools.colStatus"),
		i18n.T("cli.tools.colPath"),
	}, "\t"))
	for _, spec := range toolsStatusRows() {
		path, err := catalog.ResolveTool(spec.ID)
		status := i18n.T("cli.tools.ok")
		if err != nil {
			status = i18n.T("cli.tools.missing")
			path = ""
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", spec.ID, status, strings.TrimSpace(path))
	}
	tw.Flush()

	if cache, err := toolbin.CacheDir(); err == nil {
		fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.tools.cacheDir", map[string]any{"Path": cache}))
	}
	return 0
}

func runToolsFetch(args []string) int {
	fs := flag.NewFlagSet("tools fetch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var (
		all     bool
		dest    string
		base    string
		timeout time.Duration
	)
	fs.BoolVar(&all, "all", false, i18n.T("cli.flag.toolsAll"))
	fs.StringVar(&dest, "dest", "", i18n.T("cli.flag.toolsDest"))
	fs.StringVar(&base, "url", "", i18n.T("cli.flag.toolsURL"))
	fs.DurationVar(&timeout, "timeout", 120*time.Second, i18n.T("cli.flag.toolsTimeout"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench tools fetch [tool ...] [flags]",
			"",
			i18n.T("cli.usage.toolsFetchDetail"),
		}, "\n"))
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if timeout <= 0 {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("cli.error.toolsTimeout"))
		return 2
	}

	names := fs.Args()
	if all {
		for _, tool := range toolbin.Registry {
			names = append(names, tool.Name)
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			i18n.T("cli.error.toolsNeedsName"),
			"",
			"Usage: vmbench tools fetch [tool ...] [flags]",
		}, "\n"))
		return 2
	}

	seen := make(map[string]struct{}, len(names))
	selected := make([]toolbin.Tool, 0, len(names))
	for _, name := range names {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		tool, ok := toolbin.Find(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.toolsUnknownTool", map[string]any{"Tool": name, "Known": strings.Join(toolbinNames(), ", ")}))
			return 2
		}
		selected = append(selected, tool)
	}

	if runtime.GOOS != "linux" {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("cli.error.toolsLinuxOnly"))
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exit := 0
	for _, tool := range selected {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.tools.fetching", map[string]any{"Tool": tool.Name, "Version": tool.Version}))
		result, err := toolbin.Fetch(ctx, tool, toolbin.Options{
			Base:    base,
			Dest:    dest,
			Client:  nil,
			Version: vmbench.Version,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.tools.fetchFail", map[string]any{"Tool": tool.Name, "Err": err.Error()}))
			exit = 1
			continue
		}
		fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.tools.fetchDone", map[string]any{
			"Tool": tool.Name, "Version": tool.Version, "Path": result.Path,
		}))
	}
	return exit
}

func toolbinNames() []string {
	names := make([]string, 0, len(toolbin.Registry))
	for _, tool := range toolbin.Registry {
		names = append(names, tool.Name)
	}
	return names
}

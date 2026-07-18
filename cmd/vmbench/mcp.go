package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	mcpserver "github.com/cloudapp3/vmbench/mcp"
)

func runMCP(args []string) int {
	if len(args) == 0 || args[0] == "serve" {
		if len(args) > 0 {
			args = args[1:]
		}
		return runMCPServe(args)
	}
	fmt.Fprintf(os.Stderr, "unknown mcp command: %s\n\n", args[0])
	printMCPUsage(os.Stderr)
	return 2
}

func printMCPUsage(w io.Writer) {
	fmt.Fprintln(w, strings.Join([]string{
		"Usage: vmbench mcp serve [flags]",
		"",
		"Expose vmbench tools through the Model Context Protocol.",
		"",
		"Flags:",
		"  --transport stdio   MCP transport (stdio only in this build)",
	}, "\n"))
}

func runMCPServe(args []string) int {
	fs := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var transport string
	fs.StringVar(&transport, "transport", "stdio", "MCP transport: stdio")
	fs.Usage = func() { printMCPUsage(os.Stderr) }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if strings.ToLower(strings.TrimSpace(transport)) != "stdio" {
		fmt.Fprintf(os.Stderr, "error: unsupported MCP transport %q (only stdio is available)\n", transport)
		return 2
	}
	if err := mcpserver.ServeStdio(context.Background(), os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		return 1
	}
	return 0
}

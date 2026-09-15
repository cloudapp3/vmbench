package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/redact"
	"github.com/cloudapp3/vmbench/share"
)

// runShare implements `vmbench share <report.json|->`: it projects a saved
// report into a paste-friendly payload, redacts it, and uploads it to an
// explicitly chosen paste provider. Uploading never happens without this
// subcommand — the benchmark, TUI, and MCP paths do not call into share.
func runShare(args []string) int {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	format := fs.String("format", "text", i18n.T("cli.flag.shareFormat"))
	redactMode := fs.String("redact", string(redact.Default), i18n.T("cli.flag.redact"))
	media := fs.String("media", "summary", i18n.T("cli.flag.shareMedia"))
	providers := fs.String("provider", share.DefaultProviders, i18n.T("cli.flag.shareProvider"))
	endpoint := fs.String("share-endpoint", "", i18n.T("cli.flag.shareEndpoint"))
	dryRun := fs.Bool("dry-run", false, i18n.T("cli.flag.shareDryRun"))
	savePayload := fs.String("save-payload", "", i18n.T("cli.flag.savePayload"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench share <report.json|-> [flags]\n\n"+i18n.T("cli.usage.shareDetail"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.shareNeedsReport"))
		return 2
	}

	shareFormat, err := share.ParseFormat(*format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
		return 2
	}
	mode, err := redact.Parse(*redactMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
		return 2
	}
	mediaFull := false
	switch strings.ToLower(strings.TrimSpace(*media)) {
	case "", "summary":
	case "full":
		mediaFull = true
	default:
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.shareUnknownMedia", map[string]any{"Value": *media}))
		return 2
	}
	providerChain, err := share.ParseProviders(*providers, *endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
		return 2
	}

	data, err := readScoreInput(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.readingReport", map[string]any{"Path": fs.Arg(0), "Err": err.Error()}))
		return 1
	}

	prepared, err := share.Prepare(data, share.Options{
		Format:    shareFormat,
		Redact:    mode,
		MediaFull: mediaFull,
	})
	if err != nil {
		var tooLarge *share.TooLargeError
		if errors.As(err, &tooLarge) {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.shareTooLarge", map[string]any{
				"Size":  fmt.Sprintf("%.1f KiB", float64(tooLarge.Size)/1024),
				"Limit": fmt.Sprintf("%d KiB", tooLarge.Limit/1024),
			}))
			return 1
		}
		var notReport *share.NotReportError
		if errors.As(err, &notReport) {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
			return 2
		}
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
		return 1
	}

	if !mode.Enabled() {
		fmt.Fprintln(os.Stderr, i18n.T("cli.notice.redactDisabled"))
	}

	if *dryRun {
		os.Stdout.Write(prepared.Payload)
		if len(prepared.Payload) > 0 && prepared.Payload[len(prepared.Payload)-1] != '\n' {
			fmt.Println()
		}
		fmt.Fprintf(os.Stdout, "\n---\n%s\n", prepared.Inventory.Summary())
		return 0
	}

	if *savePayload != "" {
		if err := writeFile(*savePayload, func(w io.Writer) error {
			_, err := w.Write(prepared.Payload)
			return err
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.error.wrap", map[string]any{"Err": err.Error()}))
			return 1
		}
	}

	url, attempts, err := share.Upload(context.Background(), prepared.Payload, share.UploadOptions{
		Providers: providerChain,
		Endpoint:  *endpoint,
		Client:    shareUploadClient, // nil in production: share.Upload default (30s)
		UserAgent: fmt.Sprintf("vmbench/%s (+https://github.com/cloudapp3/vmbench)", vmbench.Version),
	})
	if err != nil {
		for _, attempt := range attempts {
			fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.share.providerFailed", map[string]any{
				"Provider": attempt.Provider,
				"Err":      attempt.Err.Error(),
			}))
		}
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("cli.share.allFailed"))
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("cli.share.dryRunHint"))
		return 1
	}
	for _, attempt := range attempts {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.share.providerFailed", map[string]any{
			"Provider": attempt.Provider,
			"Err":      attempt.Err.Error(),
		}))
	}
	fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.share.uploaded", map[string]any{
		"Format":   string(shareFormat),
		"Size":     fmt.Sprintf("%.1f KiB", float64(len(prepared.Payload))/1024),
		"Provider": providerChainName(providerChain, attempts),
	}))
	fmt.Fprintln(os.Stderr, prepared.Inventory.Summary())
	fmt.Println(url)
	return 0
}

// providerChainName reports which provider actually succeeded: the last one
// attempted (fallback skips are recorded in attempts).
func providerChainName(chain []string, attempts []share.Attempt) string {
	if len(attempts) == 0 {
		return chain[0]
	}
	return chain[len(attempts)]
}

// shareUploadClient overrides the upload http client for tests (pointing it
// at a local TLS server); nil keeps share.Upload's default 30s client.
var shareUploadClient *http.Client

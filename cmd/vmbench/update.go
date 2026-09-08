package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/selfupdate"
)

// currentVersion is the seam tests use to exercise the up-to-date branch.
var currentVersion = vmbench.Version

func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var checkOnly, asJSON bool
	var version, apiBase, dest string
	var timeout time.Duration
	fs.BoolVar(&checkOnly, "check", false, i18n.T("cli.update.flag.check"))
	fs.StringVar(&version, "version", "", i18n.T("cli.update.flag.version"))
	fs.DurationVar(&timeout, "timeout", 300*time.Second, i18n.T("cli.update.flag.timeout"))
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.StringVar(&apiBase, "api-url", selfupdate.DefaultAPIBase, i18n.T("cli.update.flag.apiUrl"))
	fs.StringVar(&dest, "dest", "", i18n.T("cli.update.flag.dest"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench update [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, i18n.T("cli.usage.updateDetail"))
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || timeout <= 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.update.error.requires"))
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	options := selfupdate.Options{
		APIBase: apiBase,
		Tag:     version,
		Current: currentVersion,
		Dest:    dest,
		Client:  http.DefaultClient,
		Token:   githubToken(),
	}
	release, err := selfupdate.Check(ctx, options)
	if err != nil {
		printErr(err)
		return 1
	}
	available := options.Tag != "" || selfupdate.IsDevBuild(currentVersion) || selfupdate.CompareVersions(currentVersion, release.Ver) < 0
	assetName := selfupdate.AssetName(release.Ver, runtime.GOOS, runtime.GOARCH)
	payload := struct {
		Current         string `json:"current"`
		Latest          string `json:"latest"`
		Tag             string `json:"tag"`
		UpdateAvailable bool   `json:"update_available"`
		Updated         bool   `json:"updated"`
		Asset           string `json:"asset,omitempty"`
		Path            string `json:"path,omitempty"`
	}{
		Current:         currentVersion,
		Latest:          release.Ver,
		Tag:             release.Tag,
		UpdateAvailable: available,
	}

	if checkOnly {
		if asJSON {
			return writeNodeJSON(os.Stdout, payload)
		}
		if selfupdate.IsDevBuild(currentVersion) {
			fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.update.notice.devBuild", map[string]any{"Current": displayVersion(currentVersion), "Latest": release.Ver}))
			return 0
		}
		if available {
			fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.update.notice.available", map[string]any{"Current": currentVersion, "Latest": release.Ver, "Asset": assetName}))
		} else {
			fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.update.notice.upToDate", map[string]any{"Current": currentVersion}))
		}
		return 0
	}
	if !available {
		if asJSON {
			return writeNodeJSON(os.Stdout, payload)
		}
		fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.update.notice.upToDate", map[string]any{"Current": currentVersion}))
		return 0
	}

	if !asJSON {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.Tf("cli.update.notice.downloading", map[string]any{"Asset": assetName}))
	}
	result, err := selfupdate.Install(ctx, options, release)
	if err != nil {
		printErr(err)
		if errors.Is(err, selfupdate.ErrTargetNotWritable) {
			fmt.Fprintln(os.Stderr, i18n.T("cli.update.notice.pkgManagerHint"))
		}
		return 1
	}
	if asJSON {
		payload.Updated = true
		payload.Asset = result.AssetName
		payload.Path = result.Path
		return writeNodeJSON(os.Stdout, payload)
	}
	fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.update.notice.updated", map[string]any{
		"Latest": release.Ver,
		"Old":    displayVersion(result.OldVersion),
		"New":    result.NewVersion,
		"Path":   result.Path,
	}))
	return 0
}

// githubToken mirrors install.sh: GITHUB_TOKEN or GH_TOKEN as a bearer token
// for API rate limits.
func githubToken() string {
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("GH_TOKEN"))
}

func displayVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return "dev"
	}
	return version
}

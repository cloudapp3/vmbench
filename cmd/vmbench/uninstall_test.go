package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/history"
)

// detachStdin points os.Stdin at a pipe so stdinIsTerminal() is false even
// when the tests run attached to an interactive terminal.
func detachStdin(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})
}

// isolateUninstallEnv redirects every vmbench-owned directory into base and
// returns the redirected data root.
func isolateUninstallEnv(t *testing.T, base string) string {
	t.Helper()
	for _, pair := range [][2]string{
		{"VMBENCH_HISTORY_DIR", ""},
		{"VMBENCH_CONFIG", ""},
		{"XDG_DATA_HOME", filepath.Join(base, "data")},
		{"XDG_CONFIG_HOME", filepath.Join(base, "config")},
		{"XDG_CACHE_HOME", filepath.Join(base, "cache")},
		{"HOME", base},
	} {
		t.Setenv(pair[0], pair[1])
	}
	return filepath.Join(base, "data", "vmbench")
}

// TestUninstallHelpProbeExitsZero pins the install.sh delegation contract:
// `vmbench uninstall --help` must exit 0 so the installer delegates to this
// command instead of its shell fallback.
func TestUninstallHelpProbeExitsZero(t *testing.T) {
	detachStdin(t)
	if _, code := captureStdout(t, func() int { return runUninstall([]string{"--help"}) }); code != 0 {
		t.Fatalf("runUninstall(--help) = %d, want 0", code)
	}
}

func TestUninstallRejectsBadInvocation(t *testing.T) {
	detachStdin(t)
	if _, code := captureStdout(t, func() int { return runUninstall([]string{"--bogus"}) }); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
	if _, code := captureStdout(t, func() int { return runUninstall([]string{"extra"}) }); code != 2 {
		t.Fatalf("positional exit = %d, want 2", code)
	}
}

func TestUninstallDryRunPrintsPlanAndKeepsFiles(t *testing.T) {
	detachStdin(t)
	dataRoot := isolateUninstallEnv(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(dataRoot, "history"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int { return runUninstall([]string{"--dry-run"}) })
	if code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if !strings.Contains(out, dataRoot) {
		t.Fatalf("plan must list the data root %q:\n%s", dataRoot, out)
	}
	if _, err := os.Stat(dataRoot); err != nil {
		t.Fatalf("dry-run must not delete %s: %v", dataRoot, err)
	}
}

// TestUninstallNonInteractiveExecutes covers the full wiring end to end:
// under a non-terminal stdin the command executes without prompting. The
// binary item resolves to the test binary itself, which is safe to unlink
// while running on Unix but not on Windows.
func TestUninstallNonInteractiveExecutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the binary item is the running test executable")
	}
	detachStdin(t)
	dataRoot := isolateUninstallEnv(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(dataRoot, "history"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := history.Open(filepath.Join(dataRoot, "history"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add([]byte(`{"results":{"workloads":[]}}`), ""); err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int { return runUninstall(nil) })
	if code != 0 {
		t.Fatalf("uninstall exit = %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("data root must be gone: %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Fatalf("expected removal progress, got:\n%s", out)
	}
}

func TestUninstallJSONDryRunShape(t *testing.T) {
	detachStdin(t)
	dataRoot := isolateUninstallEnv(t, t.TempDir())
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, code := captureStdout(t, func() int { return runUninstall([]string{"--json", "--dry-run"}) })
	if code != 0 {
		t.Fatalf("json dry-run exit = %d", code)
	}
	var payload struct {
		DryRun  bool `json:"dry_run"`
		Planned []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"planned"`
		Removed []string `json:"removed"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, stdout)
	}
	if !payload.DryRun {
		t.Fatal("dry_run must be true")
	}
	found := false
	for _, item := range payload.Planned {
		if item.Path == dataRoot {
			found = true
		}
	}
	if !found {
		t.Fatalf("planned must contain %q: %+v", dataRoot, payload.Planned)
	}
	if len(payload.Removed) != 0 {
		t.Fatalf("dry-run must not remove anything: %v", payload.Removed)
	}
	if _, err := os.Stat(dataRoot); err != nil {
		t.Fatalf("dry-run must not delete %s: %v", dataRoot, err)
	}
}

func TestUninstallJSONExecutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the binary item is the running test executable")
	}
	detachStdin(t)
	dataRoot := isolateUninstallEnv(t, t.TempDir())
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, code := captureStdout(t, func() int { return runUninstall([]string{"--json"}) })
	if code != 0 {
		t.Fatalf("json exit = %d, output:\n%s", code, stdout)
	}
	var payload struct {
		DryRun  bool `json:"dry_run"`
		Planned []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"planned"`
		Removed []string `json:"removed"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, stdout)
	}
	if payload.DryRun {
		t.Fatal("dry_run must be false")
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", payload.Errors)
	}
	found := false
	for _, path := range payload.Removed {
		if path == dataRoot {
			found = true
		}
	}
	if !found {
		t.Fatalf("removed must contain %q: %v", dataRoot, payload.Removed)
	}
	if _, err := os.Stat(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("data root must be gone: %v", err)
	}
}

// TestStdinIsTerminalRejectsDevNull pins the install.sh delegation contract:
// `uninstall </dev/null` must count as non-interactive even though /dev/null
// is a character device.
func TestStdinIsTerminalRejectsDevNull(t *testing.T) {
	old := os.Stdin
	t.Cleanup(func() { os.Stdin = old })

	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer null.Close()
	os.Stdin = null
	if stdinIsTerminal() {
		t.Fatal("/dev/null stdin must not count as a terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	os.Stdin = r
	if stdinIsTerminal() {
		t.Fatal("pipe stdin must not count as a terminal")
	}
}

func TestConfirmUninstall(t *testing.T) {
	cases := map[string]bool{
		"y\n":       true,
		"Y\n":       true,
		"yes\n":     true,
		"是\n":       true,
		"n\n":       false,
		"\n":        false,
		"no\n":      false,
		"":          false, // EOF counts as no
		"garbage\n": false,
	}
	for input, want := range cases {
		var out strings.Builder
		if got := confirmUninstall(&out, strings.NewReader(input)); got != want {
			t.Errorf("confirmUninstall(%q) = %v, want %v", input, got, want)
		}
	}
}

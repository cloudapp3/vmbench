package vmbench_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testInstallVersion is the release version served by the fake release source
// used by the installer integration tests.
const testInstallVersion = "1.2.3-install-test"

// testArchiveExtraEntry is a non-binary archive entry used to prove that the
// installer installs only the vmbench executable.
const testArchiveExtraEntry = "notes.txt"

func TestInstallScriptCustomReleaseSource(t *testing.T) {
	requireInstallScriptTools(t)

	version := "dev-install-test"
	archiveName := installArchiveName(t, version)
	archive := makeInstallArchive(t)
	archiveSum := fmt.Sprintf("%x", sha256.Sum256(archive))
	checksums := []byte(fmt.Sprintf("%s  %s\n", archiveSum, archiveName))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("tt"); got != "best-vps" {
			t.Errorf("tt header = %q, want best-vps", got)
		}
		if got := r.Header.Get("f"); got != "downloadFile" {
			t.Errorf("f header = %q, want downloadFile", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("custom release request forwarded Authorization header %q", got)
		}

		switch r.URL.Path {
		case "/archive":
			_, _ = w.Write(archive)
		case "/checksums":
			_, _ = w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	cmd := exec.Command("bash", "install.sh", //nolint:gosec -- repository script under test
		"--version", version,
		"--dir", installDir,
		"--print-install-dir",
	)
	cmd.Env = installTestEnv(map[string]string{
		"GITHUB_TOKEN":              "must-not-leak",
		"VMBENCH_ARCHIVE_URL":       server.URL + "/archive",
		"VMBENCH_CHECKSUMS_URL":     server.URL + "/checksums",
		"VMBENCH_DOWNLOAD_HEADER_1": "tt: best-vps",
		"VMBENCH_DOWNLOAD_HEADER_2": "f: downloadFile",
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	if got := strings.TrimSpace(stdout.String()); got != installDir {
		t.Fatalf("install dir stdout = %q, want %q", got, installDir)
	}
	installed := filepath.Join(installDir, "vmbench")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("installed binary: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary mode = %v, want executable", info.Mode())
	}
	if !strings.Contains(stderr.String(), "Checksum verified.") {
		t.Fatalf("install output did not confirm checksum verification:\n%s", stderr.String())
	}
}

func TestInstallScriptRejectsIncompleteCustomReleaseSource(t *testing.T) {
	requireInstallScriptTools(t)

	cmd := exec.Command("bash", "install.sh", "--version", "dev-install-test") //nolint:gosec -- repository script under test
	cmd.Env = installTestEnv(map[string]string{
		"VMBENCH_ARCHIVE_URL": "https://downloads.example.test/vmbench.tar.gz",
	})
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install.sh unexpectedly accepted an incomplete custom release source:\n%s", out)
	}
	if !strings.Contains(string(out), "VMBENCH_ARCHIVE_URL and VMBENCH_CHECKSUMS_URL must be set together") {
		t.Fatalf("unexpected install.sh error:\n%s", out)
	}
}

func TestInstallScriptInstallsOnlyBinary(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installDir := filepath.Join(t.TempDir(), "install dir")

	out := runInstallScript(t, fixture, installDir, true)
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
	if _, err := os.Lstat(filepath.Join(installDir, testArchiveExtraEntry)); !os.IsNotExist(err) {
		t.Fatalf("installer unexpectedly created %s: %v", testArchiveExtraEntry, err)
	}
	if !strings.Contains(out, `"`+filepath.Join(installDir, "vmbench")+`" --preset quick`) {
		t.Fatalf("installer output does not use the quick-start flow:\n%s", out)
	}
}

func TestInstallScriptIgnoresPreexistingFilesBesideBinary(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installDir := t.TempDir()
	notesPath := filepath.Join(installDir, testArchiveExtraEntry)
	const existing = "operator-owned\n"
	if err := os.WriteFile(notesPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	runInstallScript(t, fixture, installDir, true)
	assertFileContent(t, notesPath, existing)
	assertFileMode(t, notesPath, 0o644)
}

func TestInstallScriptAcceptsBinaryOnlyArchive(t *testing.T) {
	fixture := newInstallFixture(t, "")
	installDir := t.TempDir()

	out := runInstallScriptArgs(t, fixture, true, "--dir", installDir)
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
	if !strings.Contains(out, "Resolving latest release") ||
		!strings.Contains(out, "Installed vmbench v"+testInstallVersion) {
		t.Fatalf("default-version binary-only install output is incomplete:\n%s", out)
	}
}

func TestInstallScriptAutoUserInstallUpdatesZshPathOnce(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	sudoLog := filepath.Join(t.TempDir(), "sudo.log")
	installFakeSudo(t, fixture.fakeBin)
	t.Setenv("FAKE_SUDO_LOG", sudoLog)
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	args := []string{"--version", "v" + testInstallVersion}
	out := runInstallScriptArgs(t, fixture, true, args...)
	installDir := filepath.Join(home, ".local", "bin")
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
	if _, err := os.Lstat(filepath.Join(installDir, testArchiveExtraEntry)); !os.IsNotExist(err) {
		t.Fatalf("automatic install unexpectedly created a binary-adjacent %s: %v", testArchiveExtraEntry, err)
	}
	if !strings.Contains(out, "Added "+installDir+" to "+filepath.Join(home, ".zshrc")) {
		t.Fatalf("installer did not report the persistent PATH update:\n%s", out)
	}

	secondOut := runInstallScriptArgs(t, fixture, true, args...)
	rc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	pathLine := `export PATH=` + installDir + `:$PATH`
	if count := strings.Count(string(rc), pathLine); count != 1 {
		t.Fatalf("PATH line count = %d, want 1:\n%s", count, rc)
	}
	if strings.Contains(secondOut, "Added "+installDir) ||
		!strings.Contains(secondOut, installDir+" is already configured in "+filepath.Join(home, ".zshrc")) {
		t.Fatalf("repeat install reported the wrong PATH state:\n%s", secondOut)
	}
	if _, err := os.Lstat(sudoLog); !os.IsNotExist(err) {
		t.Fatalf("automatic user install unexpectedly invoked sudo: %v", err)
	}
}

func TestInstallScriptDelegatedUninstallRemovesOnlyOwnedPathBlock(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := filepath.Join(t.TempDir(), "home with spaces")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	args := []string{"--version", "v" + testInstallVersion}
	runInstallScriptArgs(t, fixture, true, args...)
	installDir := filepath.Join(home, ".local", "bin")
	rcPath := filepath.Join(home, ".zshrc")
	quotedInstallDir := strings.ReplaceAll(installDir, " ", `\ `)
	pathLine := `export PATH=` + quotedInstallDir + `:$PATH`
	operatorBlock := "# vmbench user install\nexport PATH=/operator/bin:$PATH\n" + pathLine + "\n"
	otherInstallDir := filepath.Join(home, "bin")
	if err := os.Mkdir(otherInstallDir, 0o700); err != nil {
		t.Fatal(err)
	}
	otherBinary := filepath.Join(otherInstallDir, "vmbench")
	writeExecutable(t, otherBinary, "other-install\n")
	otherPathLine := `export PATH=` + strings.ReplaceAll(otherInstallDir, " ", `\ `) + `:$PATH`
	otherBlock := "# vmbench user install\n" + otherPathLine + "\n"
	rc, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(rc, operatorBlock+otherBlock); err != nil {
		rc.Close()
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}

	writeExecutable(t, filepath.Join(installDir, "vmbench"), `#!/bin/sh
set -eu
if [ "${1:-}" = "uninstall" ] && [ "${2:-}" = "--help" ]; then
  exit 0
fi
if [ "${1:-}" = "uninstall" ]; then
  /bin/rm -f -- "$0"
  exit 0
fi
exit 1
`)
	out := runInstallScriptArgs(t, fixture, true, "--uninstall", "--dir", installDir)
	if !strings.Contains(out, "Removed vmbench PATH entry from "+rcPath) {
		t.Fatalf("uninstall did not report PATH cleanup:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(installDir, "vmbench")); !os.IsNotExist(err) {
		t.Fatalf("delegated uninstall left the binary behind: %v", err)
	}
	assertFileContent(t, otherBinary, "other-install\n")
	raw, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "# vmbench user install"); got != 2 {
		t.Fatalf("marker count = %d, want the operator and other-install blocks:\n%s", got, raw)
	}
	if got := strings.Count(string(raw), pathLine); got != 1 {
		t.Fatalf("matching PATH line count = %d, want standalone operator line preserved:\n%s", got, raw)
	}
	if !strings.Contains(string(raw), operatorBlock) {
		t.Fatalf("uninstall changed the operator PATH content:\n%s", raw)
	}
	if !strings.Contains(string(raw), otherBlock) {
		t.Fatalf("uninstall removed the other active installation's PATH block:\n%s", raw)
	}
}

func TestInstallScriptUninstallCleansStalePathBlockWithoutInstalledFiles(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installDir := filepath.Join(home, ".local", "bin")
	pathBlock := "before\n\n# vmbench user install\nexport PATH=" + installDir + ":$PATH\nafter\n"
	rcPath := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rcPath, []byte(pathBlock), 0o640); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(home, "profile-victim")
	if err := os.WriteFile(victim, []byte(pathBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, ".profile")); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fixture.fakeBin, "uname"), "#!/bin/sh\nprintf 'TestOS\\n'\n")
	t.Setenv("HOME", home)
	t.Setenv("PATH", fixture.fakeBin+string(os.PathListSeparator)+"/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true, "--uninstall", "--dir", installDir)
	if !strings.Contains(out, "stale shell PATH entries were removed") ||
		!strings.Contains(out, filepath.Join(home, ".profile")+" is not a regular file") {
		t.Fatalf("stale cleanup output is incomplete:\n%s", out)
	}
	assertFileContent(t, rcPath, "before\n\nafter\n")
	assertFileContent(t, victim, pathBlock)
}

func TestInstallScriptFallbackUninstallPreservesFilesWhenSystemdStopFails(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installDir := filepath.Join(home, "bin")
	if err := os.Mkdir(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(installDir, "vmbench")
	dataDir := filepath.Join(home, ".local", "share", "vmbench")
	historyPath := filepath.Join(dataDir, "history", "report.json")
	if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, binaryPath, "#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(historyPath, []byte("{\"schema_version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	systemctlLog := filepath.Join(t.TempDir(), "systemctl.log")
	t.Setenv("FAKE_SYSTEMCTL_LOG", systemctlLog)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	configPath := filepath.Join(home, ".config", "vmbench", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{\"lang\":\"en\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fixture.fakeBin, "uname"), "#!/bin/sh\nprintf 'Linux\\n'\n")
	writeExecutable(t, filepath.Join(fixture.fakeBin, "systemctl"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$FAKE_SYSTEMCTL_LOG"
case "${1:-}" in
  is-active) printf 'active\n'; exit 0 ;;
  stop) printf 'permission denied\n' >&2; exit 1 ;;
  *) exit 0 ;;
esac
`)

	out := runInstallScriptArgs(t, fixture, false, "--uninstall", "--dir", installDir)
	if !strings.Contains(out, "could not stop systemd service vmbench") ||
		!strings.Contains(out, "vmbench binary and data files were left in place") {
		t.Fatalf("fallback uninstall did not fail closed:\n%s", out)
	}
	for _, path := range []string{binaryPath, historyPath, configPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s changed after failed systemd stop: %v", path, err)
		}
	}
	assertFileContent(t, systemctlLog, "is-active vmbench\nstop vmbench\n")
}

func TestInstallScriptFallbackUninstallAllowsAbsentInactiveSystemdService(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installDir := filepath.Join(home, "bin")
	if err := os.Mkdir(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(installDir, "vmbench")
	dataDir := filepath.Join(home, ".local", "share", "vmbench")
	historyPath := filepath.Join(dataDir, "history", "report.json")
	if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, binaryPath, "#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(historyPath, []byte("{\"schema_version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	systemctlLog := filepath.Join(t.TempDir(), "systemctl.log")
	t.Setenv("FAKE_SYSTEMCTL_LOG", systemctlLog)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	configDir := filepath.Join(home, ".config", "vmbench")
	configPath := filepath.Join(configDir, "config.json")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{\"lang\":\"en\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(fixture.fakeBin, "uname"), "#!/bin/sh\nprintf 'Linux\\n'\n")
	writeExecutable(t, filepath.Join(fixture.fakeBin, "systemctl"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$FAKE_SYSTEMCTL_LOG"
if [ "${1:-}" = "is-active" ]; then
  printf 'inactive\n'
  exit 3
fi
exit 1
`)

	out := runInstallScriptArgs(t, fixture, true, "--uninstall", "--dir", installDir)
	if !strings.Contains(out, "vmbench uninstalled") ||
		!strings.Contains(out, "removed vmbench data directory "+dataDir) ||
		!strings.Contains(out, "removed vmbench config directory "+configDir) {
		t.Fatalf("fallback uninstall did not complete:\n%s", out)
	}
	for _, path := range []string{binaryPath, historyPath, dataDir, configDir, configPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("fallback uninstall left %s behind: %v", path, err)
		}
	}
	assertFileContent(t, systemctlLog, "is-active vmbench\n")
}

func TestInstallScriptRejectsInvalidOptionValuesBeforeDownload(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		installDir string
		want       string
	}{
		{name: "version consumes option", args: []string{"--version", "--system"}, want: "invalid value for --version"},
		{name: "dir consumes option", args: []string{"--dir", "--system"}, want: "invalid value for --dir"},
		{name: "empty version equals", args: []string{"--version="}, want: "empty value for --version"},
		{name: "empty dir equals", args: []string{"--dir="}, want: "empty value for --dir"},
		{name: "option-like version equals", args: []string{"--version=--system"}, want: "invalid value for --version"},
		{name: "option-like dir equals", args: []string{"--dir=--system"}, want: "invalid value for --dir"},
		{
			name:       "option-like dir environment",
			args:       []string{"--version", "v" + testInstallVersion},
			installDir: "--target-directory=/tmp",
			want:       "invalid value for VMBENCH_INSTALL_DIR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newInstallFixture(t, testArchiveExtraEntry)
			curlMarker := filepath.Join(t.TempDir(), "curl-called")
			t.Setenv("FAKE_CURL_MARKER", curlMarker)
			if tc.installDir != "" {
				t.Setenv("VMBENCH_INSTALL_DIR", tc.installDir)
			}
			writeExecutable(t, filepath.Join(fixture.fakeBin, "curl"), `#!/bin/sh
set -eu
: >"$FAKE_CURL_MARKER"
exit 99
`)

			out := runInstallScriptArgs(t, fixture, false, tc.args...)
			if !strings.Contains(out, tc.want) {
				t.Fatalf("unexpected argument validation failure:\n%s", out)
			}
			if _, err := os.Lstat(curlMarker); !os.IsNotExist(err) {
				t.Fatalf("installer reached curl before rejecting arguments: %v", err)
			}
		})
	}
}

func TestInstallScriptDirArgumentOverridesInvalidEnvironment(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installDir := t.TempDir()
	t.Setenv("VMBENCH_INSTALL_DIR", "--target-directory=/tmp")

	runInstallScript(t, fixture, installDir, true)
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
}

func TestInstallScriptReadmeExportMakesBinaryResolvable(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	const script = `
VMBENCH_BIN_DIR="$(
  bash install.sh --version "$1" --print-install-dir
)" \
  && [ -n "$VMBENCH_BIN_DIR" ] \
  && [ "${VMBENCH_BIN_DIR#/}" != "$VMBENCH_BIN_DIR" ] \
  && [ -x "$VMBENCH_BIN_DIR/vmbench" ] \
  && export PATH="$VMBENCH_BIN_DIR:$PATH" \
  && command -v vmbench
`
	cmd := exec.Command("bash", "-c", script, "vmbench-readme-test", "v"+testInstallVersion) //nolint:gosec -- repository script under test
	cmd.Env = append(os.Environ(),
		"PATH="+fixture.fakeBin+string(os.PathListSeparator)+"/usr/bin:/bin",
		"FAKE_ARCHIVE="+fixture.archive,
		"FAKE_CHECKSUMS="+fixture.checksums,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("README PATH integration failed: %v\n%s", err, out)
	}
	want := filepath.Join(home, ".local", "bin", "vmbench")
	if !strings.Contains(string(out), want+"\n") {
		t.Fatalf("current shell did not resolve the installed binary %q:\n%s", want, out)
	}
}

func TestInstallScriptAutoUserInstallPrefersUserBinInPath(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	userBin := filepath.Join(home, "bin")
	if err := os.Mkdir(userBin, 0o700); err != nil {
		t.Fatal(err)
	}
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", userBin+string(os.PathListSeparator)+"/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true, "--version", "v"+testInstallVersion)
	assertFileContent(t, filepath.Join(userBin, "vmbench"), "test-binary\n")
	if !strings.Contains(out, "Verify the installation with:\n  vmbench version") {
		t.Fatalf("installer did not report an immediately runnable command:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("installer unexpectedly modified shell startup file: %v", err)
	}
}

func TestInstallScriptExplicitDirDoesNotModifyShellPath(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	installDir := filepath.Join(home, "custom", "bin")
	runInstallScript(t, fixture, installDir, true)
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("explicit --dir unexpectedly modified shell startup file: %v", err)
	}
}

func TestInstallScriptNoModifyPathLeavesShellFilesUntouched(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true,
		"--version", "v"+testInstallVersion,
		"--no-modify-path",
	)
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("--no-modify-path unexpectedly modified shell startup file: %v", err)
	}
	if !strings.Contains(out, "is not in your PATH") {
		t.Fatalf("installer did not retain the PATH warning:\n%s", out)
	}
}

func TestInstallScriptSystemModeUsesSudoForTargetWrites(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installFakeUID(t, fixture.fakeBin, "1000")
	sudoLog := filepath.Join(t.TempDir(), "sudo.log")
	sudoPath := installFakeSudo(t, fixture.fakeBin)
	t.Setenv("FAKE_SUDO_LOG", sudoLog)
	t.Setenv("PATH", "/usr/bin:/bin")

	installDir := filepath.Join(t.TempDir(), "system", "bin")
	out := runInstallScriptArgs(t, fixture, true,
		"--version", "v"+testInstallVersion,
		"--system",
		"--dir", installDir,
	)
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
	if _, err := os.Lstat(filepath.Join(installDir, testArchiveExtraEntry)); !os.IsNotExist(err) {
		t.Fatalf("system installer unexpectedly created %s: %v", testArchiveExtraEntry, err)
	}

	raw, err := os.ReadFile(sudoLog)
	if err != nil {
		t.Fatal(err)
	}
	logOutput := string(raw)
	for _, want := range []string{"-n -v", "mkdir -p " + installDir, "install -m 0755"} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("sudo log does not contain %q:\n%s", want, logOutput)
		}
	}

	targetPath := filepath.Join(installDir, "vmbench")
	privilegedCommand := `"` + sudoPath + `" "` + targetPath + `"`
	if !strings.Contains(out, "Verify the root-owned system installation with:\n  "+privilegedCommand+" version") ||
		!strings.Contains(out, `"`+targetPath+`" --preset quick`) {
		t.Fatalf("system install did not report root startup commands:\n%s", out)
	}
}

func TestInstallScriptSystemModeRequiresSudoBeforeDownload(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("VMBENCH_SUDO", filepath.Join(t.TempDir(), "missing-sudo"))
	curlMarker := filepath.Join(t.TempDir(), "curl-called")
	t.Setenv("FAKE_CURL_MARKER", curlMarker)
	writeExecutable(t, filepath.Join(fixture.fakeBin, "curl"), `#!/bin/sh
set -eu
: >"$FAKE_CURL_MARKER"
exit 99
`)

	out := runInstallScriptArgs(t, fixture, false,
		"--version", "v"+testInstallVersion,
		"--system",
	)
	if !strings.Contains(out, "configured sudo command is not executable") {
		t.Fatalf("unexpected missing-sudo failure:\n%s", out)
	}
	if _, err := os.Lstat(curlMarker); !os.IsNotExist(err) {
		t.Fatalf("installer reached the download before rejecting sudo: %v", err)
	}
}

func TestInstallScriptRootSystemModeDoesNotUseSudo(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installFakeUID(t, fixture.fakeBin, "0")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("VMBENCH_SUDO", filepath.Join(t.TempDir(), "missing-sudo"))
	installDir := filepath.Join(t.TempDir(), "system", "bin")

	out := runInstallScriptArgs(t, fixture, true,
		"--version", "v"+testInstallVersion,
		"--system",
		"--dir", installDir,
	)
	targetPath := filepath.Join(installDir, "vmbench")
	assertFileContent(t, targetPath, "test-binary\n")
	if strings.Contains(out, "root-owned system installation") ||
		!strings.Contains(out, `"`+targetPath+`" --preset quick`) {
		t.Fatalf("root system install did not report a direct startup command:\n%s", out)
	}
}

func TestInstallScriptRootUpgradeReusesExistingUserInstall(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(installDir, "vmbench"), "old-binary\n")
	const existingNotes = "operator-owned notes\n"
	if err := os.WriteFile(filepath.Join(installDir, testArchiveExtraEntry), []byte(existingNotes), 0o600); err != nil {
		t.Fatal(err)
	}
	installFakeUID(t, fixture.fakeBin, "0")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", "/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true, "--version", "v"+testInstallVersion)
	assertFileContent(t, filepath.Join(installDir, "vmbench"), "test-binary\n")
	assertFileContent(t, filepath.Join(installDir, testArchiveExtraEntry), existingNotes)
	if !strings.Contains(out, "Using existing installation directory: "+installDir) {
		t.Fatalf("installer did not reuse the existing root install:\n%s", out)
	}
	assertFileContentContains(t, filepath.Join(home, ".zshrc"), "export PATH="+installDir+":$PATH")
}

func TestInstallScriptReusesActiveConventionalInstallBeforeStaleCopy(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	staleDir := filepath.Join(home, ".local", "bin")
	activeDir := filepath.Join(home, "bin")
	for _, dir := range []string{staleDir, activeDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable(t, filepath.Join(staleDir, "vmbench"), "stale-binary\n")
	writeExecutable(t, filepath.Join(activeDir, "vmbench"), "active-binary\n")
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", activeDir+string(os.PathListSeparator)+"/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true, "--version", "v"+testInstallVersion)
	assertFileContent(t, filepath.Join(activeDir, "vmbench"), "test-binary\n")
	assertFileContent(t, filepath.Join(staleDir, "vmbench"), "stale-binary\n")
	if !strings.Contains(out, "Using existing installation directory: "+activeDir) {
		t.Fatalf("installer did not reuse the active install:\n%s", out)
	}
}

func TestInstallScriptUnsupportedShellDoesNotWriteProfile(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	home := t.TempDir()
	installFakeUID(t, fixture.fakeBin, "1000")
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/fish")
	t.Setenv("PATH", "/usr/bin:/bin")

	out := runInstallScriptArgs(t, fixture, true, "--version", "v"+testInstallVersion)
	if _, err := os.Lstat(filepath.Join(home, ".profile")); !os.IsNotExist(err) {
		t.Fatalf("unsupported shell unexpectedly modified .profile: %v", err)
	}
	if !strings.Contains(out, "unsupported shell fish") {
		t.Fatalf("installer did not warn about the unsupported shell:\n%s", out)
	}
}

func TestInstallScriptPrintInstallDirUsesStdoutOnly(t *testing.T) {
	fixture := newInstallFixture(t, testArchiveExtraEntry)
	installDir := filepath.Join(t.TempDir(), "install dir")
	cmd := installScriptCommand(t, fixture,
		"--version", "v"+testInstallVersion,
		"--dir", installDir,
		"--print-install-dir",
	)
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("installer failed: %v", err)
	}
	if got, want := string(stdout), installDir+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func requireInstallScriptTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a Unix installer")
	}
	for _, name := range []string{"bash", "curl", "tar"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is required to test install.sh", name)
		}
	}
}

func installArchiveName(t *testing.T, version string) string {
	t.Helper()
	osName := runtime.GOOS
	if osName != "linux" && osName != "darwin" {
		t.Skipf("install.sh does not support %s", osName)
	}
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		t.Skipf("install.sh does not support %s", arch)
	}
	return fmt.Sprintf("vmbench-%s-%s-%s.tar.gz", strings.TrimPrefix(version, "v"), osName, arch)
}

func makeInstallArchive(t *testing.T) []byte {
	t.Helper()

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("#!/usr/bin/env sh\nprintf 'vmbench install test\\n'\n")
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "vmbench",
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("write archive header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("write archive content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar archive: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip archive: %v", err)
	}
	return archive.Bytes()
}

func installTestEnv(overrides map[string]string) []string {
	blocked := map[string]bool{
		"GH_TOKEN":                  true,
		"GITHUB_TOKEN":              true,
		"VMBENCH_ARCHIVE_URL":       true,
		"VMBENCH_CHECKSUMS_URL":     true,
		"VMBENCH_DOWNLOAD_HEADER_1": true,
		"VMBENCH_DOWNLOAD_HEADER_2": true,
		"VMBENCH_INSTALL_DIR":       true,
		"VMBENCH_NO_MODIFY_PATH":    true,
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if !blocked[name] {
			env = append(env, item)
		}
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}

type installFixture struct {
	archive   string
	checksums string
	fakeBin   string
}

func newInstallFixture(t *testing.T, extraEntry string) installFixture {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("install.sh does not support %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("installer does not support test architecture %s", runtime.GOARCH)
	}
	dir := t.TempDir()
	archiveName := fmt.Sprintf("vmbench-%s-%s-%s.tar.gz", testInstallVersion, runtime.GOOS, runtime.GOARCH)
	archivePath := filepath.Join(dir, archiveName)
	writeInstallArchive(t, archivePath, extraEntry)
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	checksumsPath := filepath.Join(dir, "checksums.txt")
	sum := sha256.Sum256(raw)
	if err := os.WriteFile(checksumsPath, []byte(fmt.Sprintf("%x  %s\n", sum, archiveName)), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeCurl := `#!/bin/sh
set -eu
[ -z "${FAKE_CURL_LOG:-}" ] || printf '%s\n' "$@" >>"$FAKE_CURL_LOG"
[ -z "${FAKE_CURL_MARKER:-}" ] || : >"$FAKE_CURL_MARKER"
out=""
url=""
need_out=0
for arg in "$@"; do
  if [ "$need_out" -eq 1 ]; then
    out="$arg"
    need_out=0
    continue
  fi
  case "$arg" in
    -o) need_out=1 ;;
    http://*|https://*) url="$arg" ;;
  esac
done
case "$url" in
	*/releases/latest)
		printf '  "tag_name": "%s",\n' "$FAKE_VERSION"
		exit 0
		;;
	*checksums.txt) source="$FAKE_CHECKSUMS" ;;
	*.tar.gz) source="$FAKE_ARCHIVE" ;;
  *) echo "unexpected URL: $url" >&2; exit 1 ;;
esac
if [ -n "$out" ]; then
  cp "$source" "$out"
else
  cat "$source"
fi
`
	if err := os.WriteFile(filepath.Join(fakeBin, "curl"), []byte(fakeCurl), 0o700); err != nil {
		t.Fatal(err)
	}
	return installFixture{archive: archivePath, checksums: checksumsPath, fakeBin: fakeBin}
}

func writeInstallArchive(t *testing.T, path, extraEntry string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	entries := []struct {
		name string
		mode int64
		data string
	}{
		{name: "vmbench", mode: 0o755, data: "test-binary\n"},
	}
	if extraEntry != "" {
		entries = append(entries, struct {
			name string
			mode int64
			data string
		}{name: extraEntry, mode: 0o644, data: "reference-notes\n"})
	}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.data))}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func runInstallScript(t *testing.T, fixture installFixture, installDir string, wantSuccess bool) string {
	t.Helper()
	return runInstallScriptArgs(t, fixture, wantSuccess,
		"--version", "v"+testInstallVersion,
		"--dir", installDir,
	)
}

func runInstallScriptArgs(t *testing.T, fixture installFixture, wantSuccess bool, args ...string) string {
	t.Helper()
	cmd := installScriptCommand(t, fixture, args...)
	out, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("installer failed: %v\n%s", err, out)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("installer unexpectedly succeeded:\n%s", out)
	}
	return string(out)
}

func installScriptCommand(t *testing.T, fixture installFixture, args ...string) *exec.Cmd {
	t.Helper()
	cmdArgs := append([]string{"install.sh"}, args...) //nolint:gosec -- repository script under test
	cmd := exec.Command("bash", cmdArgs...)            //nolint:gosec -- repository script under test
	cmd.Env = append(os.Environ(),
		"PATH="+fixture.fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_ARCHIVE="+fixture.archive,
		"FAKE_CHECKSUMS="+fixture.checksums,
		"FAKE_VERSION=v"+testInstallVersion,
	)
	return cmd
}

func installFakeUID(t *testing.T, fakeBin, uid string) {
	t.Helper()
	t.Setenv("FAKE_UID", uid)
	writeExecutable(t, filepath.Join(fakeBin, "id"), `#!/bin/sh
set -eu
if [ "${1:-}" = "-u" ]; then
  printf '%s\n' "$FAKE_UID"
  exit 0
fi
exec /usr/bin/id "$@"
`)
}

func installFakeSudo(t *testing.T, fakeBin string) string {
	t.Helper()
	sudoPath := filepath.Join(fakeBin, "sudo")
	t.Setenv("VMBENCH_SUDO", sudoPath)
	writeExecutable(t, sudoPath, `#!/bin/sh
set -eu
printf '%s\n' "$*" >>"$FAKE_SUDO_LOG"
if [ "${1:-}" = "-n" ] && [ "${2:-}" = "-v" ]; then
  exit 0
fi
if [ "${1:-}" = "-v" ]; then
  exit 0
fi
exec "$@"
`)
	return sudoPath
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("%s content = %q, want %q", path, raw, want)
	}
}

func assertFileContentContains(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), want) {
		t.Fatalf("%s does not contain %q: %q", path, want, raw)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

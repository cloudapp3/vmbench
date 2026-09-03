package vmbench_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

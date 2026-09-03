package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/suite"
)

func TestWriteFileAtomicallyReplacesWithOwnerOnlyMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(path, func(w io.Writer) error {
		_, err := w.Write([]byte("new"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("report content = %q, want new", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("report mode = %o, want 600", got)
		}
	}
}

func TestWriteFilePreservesExistingReportOnEncodeFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("encode failed")
	if err := writeFile(path, func(io.Writer) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("writeFile() error = %v, want %v", err, wantErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("report content = %q, want old", data)
	}
}

func TestSuiteProgressPrinterWritesSectionLifecycle(t *testing.T) {
	var output bytes.Buffer
	printer := suiteProgressPrinterTo(&output, true)
	printer(suite.Event{Kind: suite.EventSectionStart, Section: suite.SectionHardware, Status: "running"})
	printer(suite.Event{Kind: suite.EventSectionDone, Section: suite.SectionHardware, Status: "ok", Message: "2 ok"})
	printer(suite.Event{Kind: suite.EventSuiteDone, Status: "ok", Message: "1/1 sections ok"})
	for _, want := range []string{"hardware", "running", "2 ok", "complete", "1/1 sections ok"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("progress output = %q, want %q", output.String(), want)
		}
	}
}

func TestLinuxHardwarePackagesOnlyReturnsPackageManagedTools(t *testing.T) {
	got := linuxHardwarePackages([]string{
		catalog.HardwareToolSysbench,
		catalog.HardwareToolFio,
		catalog.HardwareToolGeekbench,
		catalog.HardwareToolFio,
	})
	if strings.Join(got, ",") != "sysbench,fio" {
		t.Fatalf("linuxHardwarePackages() = %v, want [sysbench fio]", got)
	}
}

func TestHardwareToolPreflightRespectsWorkloadFilter(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	tools := []string{catalog.HardwareToolFio, catalog.HardwareToolMBW}

	var output bytes.Buffer
	printHardwareToolPreflight(&output, tools, regexp.MustCompile(`^Memory$`))
	if got := output.String(); !strings.Contains(got, "mbw") || strings.Contains(got, "fio") {
		t.Fatalf("filtered preflight output = %q, want mbw only", got)
	}

	output.Reset()
	printHardwareToolPreflight(&output, tools, regexp.MustCompile(`^CPU$`))
	if output.Len() != 0 {
		t.Fatalf("non-matching preflight output = %q, want empty", output.String())
	}
}

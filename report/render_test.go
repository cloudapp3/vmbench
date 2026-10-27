package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/sysinfo"
)

func evidenceDocument() Document {
	return Document{
		SchemaVersion: currentSchemaVersion,
		System: sysinfo.SystemInfo{
			CPU: sysinfo.CPUInfo{
				Model: "Test CPU", PhysicalCores: 2, LogicalCores: 4,
				CacheSizes: map[string]int64{"L1d": 32 << 10, "L2": 4 << 20, "L3": 16 << 20},
			},
			Network: sysinfo.NetworkInfo{PrimaryDriver: "virtio_net", PrimaryPCI: "1af4:1000"},
			Platform: sysinfo.PlatformDiagnostics{
				VirtioBalloon: "present",
				KSM:           "disabled",
			},
		},
	}
}

func TestWriteConsoleIncludesHardwareEvidence(t *testing.T) {
	var out bytes.Buffer
	if err := WriteConsole(&out, evidenceDocument()); err != nil {
		t.Fatal(err)
	}
	console := out.String()
	for _, want := range []string{
		"L1d 32 KiB, L2 4 MiB, L3 16 MiB",
		"virtio_net (1af4:1000)",
		"balloon=present (!)",
		"ksm=disabled",
	} {
		if !strings.Contains(console, want) {
			t.Fatalf("console system header missing %q:\n%s", want, console)
		}
	}
}

func TestWriteConsoleOmitsEvidenceLinesWhenAbsent(t *testing.T) {
	var out bytes.Buffer
	if err := WriteConsole(&out, Document{}); err != nil {
		t.Fatal(err)
	}
	console := out.String()
	for _, banned := range []string{"Cache:", "NIC:", "Oversell:"} {
		if strings.Contains(console, banned) {
			t.Fatalf("console must skip absent evidence, found %q:\n%s", banned, console)
		}
	}
}

// TestFormatTimeAdaptiveUnits pins the ≥10s switch from milliseconds to
// seconds so long workloads stop printing as five-digit millisecond values.
func TestFormatTimeAdaptiveUnits(t *testing.T) {
	cases := []struct {
		medianMS float64
		want     string
	}{
		{123.4, "123.4ms"},
		{9999.9, "9999.9ms"},
		{10000, "10.00 s"},
		{65432.1, "65.43 s"},
	}
	for _, tc := range cases {
		got := formatTime(&ResultEntry{MedianMS: tc.medianMS})
		if got != tc.want {
			t.Fatalf("formatTime(%vms) = %q, want %q", tc.medianMS, got, tc.want)
		}
	}
}

// TestWriteConsoleFailureSummary pins the N/M failure line: present when a
// workload errored, absent when everything succeeded.
func TestWriteConsoleFailureSummary(t *testing.T) {
	doc := evidenceDocument()
	doc.Results.Workloads = []WorkloadEntry{
		{Name: "ok-workload", Category: "CPU", Result: &ResultEntry{MedianMS: 10, Detail: "fine"}},
		{Name: "bad-workload", Category: "CPU", Result: &ResultEntry{Error: "tool missing"}},
	}
	var out bytes.Buffer
	if err := WriteConsole(&out, doc); err != nil {
		t.Fatal(err)
	}
	console := out.String()
	if !strings.Contains(console, "1/2 workloads failed") {
		t.Fatalf("console missing failure summary:\n%s", console)
	}

	doc.Results.Workloads[1] = WorkloadEntry{Name: "bad-workload", Category: "CPU", Result: &ResultEntry{MedianMS: 5}}
	out.Reset()
	if err := WriteConsole(&out, doc); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "workloads failed") {
		t.Fatalf("console must not print a failure summary for a clean run:\n%s", out.String())
	}
}

func TestWriteHTMLIncludesSysExtraLine(t *testing.T) {
	var out bytes.Buffer
	if err := WriteHTML(&out, evidenceDocument()); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{
		`class="sys-extra"`,
		"L1d 32 KiB",
		"virtio_net (1af4:1000)",
		"balloon=present (!)",
		"ksm=disabled",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML sys-extra line missing %q", want)
		}
	}
}

func TestWriteHTMLOmitsSysExtraLineWhenAbsent(t *testing.T) {
	var out bytes.Buffer
	if err := WriteHTML(&out, Document{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), `class="sys-extra"`) {
		t.Fatal("HTML must not render the sys-extra line without evidence")
	}
}

package report

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func markdownDocument() Document {
	doc := evidenceDocument()
	doc.Version = "v0.14.0-test"
	doc.Timestamp = time.Date(2026, 9, 11, 7, 0, 0, 0, time.UTC)
	doc.Config = RunConfig{CatalogSource: "embedded", CatalogRevision: "rev-42"}
	doc.Results = ResultsSection{Workloads: []WorkloadEntry{
		{
			Name:     "CPU 1-thread",
			Category: "CPU",
			Result: &ResultEntry{
				MedianMS:         12.3,
				ThroughputPerSec: 8123,
				ThroughputUnit:   "ops/s",
				AvgNSPerAccess:   15300,
			},
		},
	}}
	doc.Warnings = []string{"sample warning"}
	return doc
}

func TestWriteMarkdownRendersHeadingsAndFencedTables(t *testing.T) {
	var out bytes.Buffer
	if err := WriteMarkdown(&out, markdownDocument()); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, want := range []string{
		"# VMBench v0.14.0-test — Benchmark Report",
		"> vmbench v0.14.0-test · 2026-09-11T07:00:00Z · catalog embedded@rev-42",
		"## System",
		"```",
		"CPU 1-thread",
		"8123 ops/s",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if got := strings.Count(markdown, "```"); got%2 != 0 || got < 4 {
		t.Fatalf("expected balanced fenced blocks, got %d fence markers:\n%s", got, markdown)
	}
}

func TestWriteMarkdownKeepsConsoleEvidenceAndOmitsHostname(t *testing.T) {
	var out bytes.Buffer
	if err := WriteMarkdown(&out, markdownDocument()); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, want := range []string{
		"L1d 32 KiB, L2 4 MiB, L3 16 MiB",
		"virtio_net (1af4:1000)",
		"balloon=present (!)",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown system card missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "Hostname") {
		t.Fatalf("markdown must not leak the hostname:\n%s", markdown)
	}
}

func TestWriteMarkdownOmitsAbsentBlocks(t *testing.T) {
	var out bytes.Buffer
	if err := WriteMarkdown(&out, Document{}); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, banned := range []string{"## Extensions", "## Warnings", "Cache:", "NIC:"} {
		if strings.Contains(markdown, banned) {
			t.Fatalf("markdown must skip absent blocks, found %q:\n%s", banned, markdown)
		}
	}
}

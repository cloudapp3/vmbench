package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCurrentECSDiffSnapshot(t *testing.T) {
	snapshot := currentECSDiffSnapshot()
	if snapshot.SnapshotDate != ecsDiffSnapshotDate {
		t.Fatalf("snapshot date = %q, want %q", snapshot.SnapshotDate, ecsDiffSnapshotDate)
	}
	if len(snapshot.Rows) < 10 {
		t.Fatalf("rows = %d, want at least 10", len(snapshot.Rows))
	}
	if row, ok := ecsDiffArea(snapshot, "网站/TG 可达性"); !ok || row.Status == "gap" {
		t.Fatalf("website/TG row = %+v, want implemented status", row)
	}
	if !hasECSDiffArea(snapshot, "评分策略") {
		t.Fatalf("expected scoring strategy row")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !strings.Contains(string(data), "P1/v0.2") {
		t.Fatalf("JSON snapshot should include P1/v0.2 requirement")
	}
}

func TestWriteECSDiffText(t *testing.T) {
	var buf bytes.Buffer
	if err := writeECSDiffText(&buf, currentECSDiffSnapshot()); err != nil {
		t.Fatalf("write text: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"VMBench vs ECS 差异快照",
		"https://github.com/spiritLHLS/ecs",
		"网站/TG 可达性",
		"不重新引入综合评分",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}

func hasECSDiffArea(snapshot ecsDiffSnapshot, area string) bool {
	_, ok := ecsDiffArea(snapshot, area)
	return ok
}

func ecsDiffArea(snapshot ecsDiffSnapshot, area string) (ecsDiffRow, bool) {
	for _, row := range snapshot.Rows {
		if row.Area == area {
			return row, true
		}
	}
	return ecsDiffRow{}, false
}

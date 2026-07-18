package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInspectRecognizesCurrentAndLegacyReports(t *testing.T) {
	tests := []struct {
		name string
		json string
		kind Kind
	}{
		{name: "current run", json: `{"report_kind":"run","results":{"workloads":[]}}`, kind: KindRun},
		{name: "legacy run", json: `{"timestamp":"2026-07-13T01:02:03Z","results":{"workloads":[]}}`, kind: KindRun},
		{name: "current suite", json: `{"report_kind":"suite","config":{},"future_section":{"enabled":true}}`, kind: KindSuite},
		{name: "legacy suite", json: `{"version":1,"started_time":1700000000,"config":{},"hardware":{"enabled":true}}`, kind: KindSuite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := Inspect([]byte(tt.json))
			if err != nil {
				t.Fatal(err)
			}
			if meta.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q", meta.Kind, tt.kind)
			}
		})
	}
	if _, err := Inspect([]byte(`{"hello":"world"}`)); err == nil {
		t.Fatal("Inspect accepted an unknown JSON document")
	}
}

func TestStoreLifecycleAndOrdering(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	older := []byte(`{"timestamp":"2026-07-12T01:00:00Z","results":{"workloads":[]}}`)
	newer := []byte(`{"schema_version":2,"report_kind":"suite","report_id":"suite-new","started_at":"2026-07-13T01:00:00Z","config":{},"ping":{"enabled":true}}`)
	oldRecord, err := store.Add(older, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	newRecord, err := store.Add(newer, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if oldRecord.Kind != KindRun || newRecord.Kind != KindSuite || newRecord.SourceReportID != "suite-new" {
		t.Fatalf("stored records = %+v %+v", oldRecord, newRecord)
	}

	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != newRecord.ID || records[1].ID != oldRecord.ID {
		t.Fatalf("List order = %+v", records)
	}
	latest, err := store.Latest(2)
	if err != nil {
		t.Fatal(err)
	}
	if latest[0].ID != oldRecord.ID || latest[1].ID != newRecord.ID {
		t.Fatalf("Latest comparison order = %+v", latest)
	}
	loaded, err := store.Get(oldRecord.ID)
	if err != nil || !json.Valid(loaded.Report) || loaded.Tag != "baseline" {
		t.Fatalf("Get = %+v, %v", loaded, err)
	}
	if err := store.Delete(oldRecord.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(oldRecord.ID); err == nil {
		t.Fatal("Get succeeded after Delete")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("temporary history file was not removed: %s", entry.Name())
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, newRecord.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("history file mode = %o, want 600", got)
		}
		info, err = os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("history directory mode = %o, want 700", got)
		}
	}
}

func TestStoreRejectsEmptyDirectoryForEveryOperation(t *testing.T) {
	store := &Store{}
	if _, err := store.Add([]byte(`{}`), ""); err == nil {
		t.Fatal("Add accepted an empty directory")
	}
	if _, err := store.List(); err == nil {
		t.Fatal("List accepted an empty directory")
	}
	if _, err := store.Get("valid-id"); err == nil {
		t.Fatal("Get accepted an empty directory")
	}
	if err := store.Delete("valid-id"); err == nil {
		t.Fatal("Delete accepted an empty directory")
	}
}

func TestDefaultDirUsesXDGDataHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("VMBENCH_HISTORY_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdg)
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "vmbench", "history")
	if dir != want {
		t.Fatalf("DefaultDir() = %q, want %q", dir, want)
	}
}

func TestStoreValidatesTagAndLatestCount(t *testing.T) {
	store, _ := Open(t.TempDir())
	report := []byte(`{"timestamp":"2026-07-13T01:00:00Z","results":{"workloads":[]}}`)
	if _, err := store.Add(report, "bad\ntag"); err == nil {
		t.Fatal("Add accepted a control character in tag")
	}
	if _, err := store.Add(report, strings.Repeat("x", 129)); err == nil {
		t.Fatal("Add accepted an oversized tag")
	}
	if _, err := store.Latest(1); err == nil || !strings.Contains(err.Error(), "contains 0") {
		t.Fatalf("Latest error = %v", err)
	}
}

func TestRecordReportTimeFallsBackToAddTime(t *testing.T) {
	store, _ := Open(t.TempDir())
	before := time.Now().UTC().Add(-time.Second)
	record, err := store.Add([]byte(`{"results":{"workloads":[]}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if record.ReportTime.Before(before) || record.ReportTime.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("fallback report time = %s", record.ReportTime)
	}
}

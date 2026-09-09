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
		{name: "current checkup", json: `{"report_kind":"checkup","config":{},"future_section":{"enabled":true}}`, kind: KindCheckup},
		{name: "legacy suite kind (pre-v0.11.0)", json: `{"report_kind":"suite","config":{},"future_section":{"enabled":true}}`, kind: KindCheckup},
		{name: "legacy checkup", json: `{"version":1,"started_time":1700000000,"config":{},"hardware":{"enabled":true}}`, kind: KindCheckup},
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

// TestListNormalizesLegacyCheckupRecordKind stores a record exactly as a
// pre-v0.11.0 release would have written it (kind "suite" on the record and
// report_kind "suite" in the embedded report) and verifies it reads back as
// the current checkup kind.
func TestListNormalizesLegacyCheckupRecordKind(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"schema_version":2,"report_kind":"suite","report_id":"legacy-checkup","started_at":"2026-07-13T01:00:00Z","config":{},"ping":{"enabled":true}}`)
	meta, err := Inspect(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Kind != KindCheckup {
		t.Fatalf("legacy report kind = %q, want %q", meta.Kind, KindCheckup)
	}
	added := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	record := Record{
		StorageVersion: storageVersion,
		Kind:           kindLegacySuite,
		AddedAt:        added,
		ReportTime:     added,
		SourceReportID: "legacy-checkup",
		SchemaVersion:  meta.SchemaVersion,
		Report:         append(json.RawMessage(nil), legacy...),
	}
	if record.ID, err = store.availableID(added, legacy, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.writeRecord(record); err != nil {
		t.Fatal(err)
	}

	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Kind != KindCheckup {
		t.Fatalf("stored kind = %q, want normalized %q", records[0].Kind, KindCheckup)
	}
	got, err := store.Get(records[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindCheckup {
		t.Fatalf("loaded kind = %q, want normalized %q", got.Kind, KindCheckup)
	}
}

func TestStoreLifecycleAndOrdering(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	older := []byte(`{"timestamp":"2026-07-12T01:00:00Z","results":{"workloads":[]}}`)
	newer := []byte(`{"schema_version":2,"report_kind":"checkup","report_id":"checkup-new","started_at":"2026-07-13T01:00:00Z","config":{},"ping":{"enabled":true}}`)
	oldRecord, err := store.Add(older, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	newRecord, err := store.Add(newer, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if oldRecord.Kind != KindRun || newRecord.Kind != KindCheckup || newRecord.SourceReportID != "checkup-new" {
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

func TestDefaultRootMirrorsDefaultDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("VMBENCH_HISTORY_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdg)
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "vmbench"); root != want {
		t.Fatalf("DefaultRoot() = %q, want %q", root, want)
	}
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if parent := filepath.Dir(dir); parent != root {
		t.Fatalf("DefaultDir parent = %q, want DefaultRoot %q", parent, root)
	}
}

// TestDefaultRootIgnoresHistoryOverride pins the contract uninstall relies
// on: a redirected VMBENCH_HISTORY_DIR never becomes the vmbench-owned root.
func TestDefaultRootIgnoresHistoryOverride(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("VMBENCH_HISTORY_DIR", filepath.Join(xdg, "elsewhere"))
	root, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "vmbench"); root != want {
		t.Fatalf("DefaultRoot() = %q, want %q", root, want)
	}
}

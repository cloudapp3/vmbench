// Package history stores vmbench reports as local, append-only JSON records.
package history

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Kind identifies a supported vmbench report family.
type Kind string

const (
	KindRun   Kind = "run"
	KindSuite Kind = "suite"

	storageVersion = 1
)

var validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Metadata is the minimum report envelope understood by history and compare.
type Metadata struct {
	Kind          Kind
	ReportID      string
	SchemaVersion int
	ReportTime    time.Time
}

// Record is one history file. Report contains the original report JSON.
type Record struct {
	StorageVersion int             `json:"storage_version"`
	ID             string          `json:"id"`
	Kind           Kind            `json:"kind"`
	Tag            string          `json:"tag,omitempty"`
	AddedAt        time.Time       `json:"added_at"`
	ReportTime     time.Time       `json:"report_time"`
	SourceReportID string          `json:"source_report_id,omitempty"`
	SchemaVersion  int             `json:"schema_version,omitempty"`
	Report         json.RawMessage `json:"report"`
}

// Store manages a directory of standalone history records.
type Store struct {
	Dir string
}

// DefaultDir returns the platform data directory used for history records.
func DefaultDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("VMBENCH_HISTORY_DIR")); override != "" {
		return filepath.Clean(override), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "vmbench", "history"), nil
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user data directory: %w", err)
		}
		return filepath.Join(base, "vmbench", "history"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "vmbench", "history"), nil
}

// Open returns a Store. An empty dir selects DefaultDir.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	return &Store{Dir: filepath.Clean(dir)}, nil
}

// Inspect validates a report and identifies legacy and current run/suite JSON.
func Inspect(data []byte) (Metadata, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return Metadata{}, fmt.Errorf("invalid report JSON: %w", err)
	}
	if object == nil {
		return Metadata{}, errors.New("report must be a JSON object")
	}

	kind, err := detectKind(object)
	if err != nil {
		return Metadata{}, err
	}
	meta := Metadata{Kind: kind}
	_ = unmarshalString(object["report_id"], &meta.ReportID)
	_ = json.Unmarshal(object["schema_version"], &meta.SchemaVersion)
	meta.ReportTime = reportTime(object)
	return meta, nil
}

// AddFile reads and stores one report file.
func (s *Store) AddFile(path, tag string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read report %s: %w", path, err)
	}
	return s.Add(data, tag)
}

// Add stores a report using an atomic, owner-only JSON file.
func (s *Store) Add(data []byte, tag string) (Record, error) {
	if s == nil || strings.TrimSpace(s.Dir) == "" {
		return Record{}, errors.New("history store directory is empty")
	}
	meta, err := Inspect(data)
	if err != nil {
		return Record{}, err
	}
	tag = strings.TrimSpace(tag)
	if err := validateTag(tag); err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return Record{}, fmt.Errorf("create history directory: %w", err)
	}
	if err := os.Chmod(s.Dir, 0o700); err != nil {
		return Record{}, fmt.Errorf("secure history directory: %w", err)
	}

	now := time.Now().UTC()
	reportAt := meta.ReportTime
	if reportAt.IsZero() {
		reportAt = now
	}
	record := Record{
		StorageVersion: storageVersion,
		Kind:           meta.Kind,
		Tag:            tag,
		AddedAt:        now,
		ReportTime:     reportAt.UTC(),
		SourceReportID: meta.ReportID,
		SchemaVersion:  meta.SchemaVersion,
		Report:         append(json.RawMessage(nil), data...),
	}
	record.ID, err = s.availableID(reportAt, data, tag)
	if err != nil {
		return Record{}, err
	}
	if err := s.writeRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// List returns records newest report first, with added time as a tie-breaker.
func (s *Store) List() ([]Record, error) {
	if s == nil || strings.TrimSpace(s.Dir) == "" {
		return nil, errors.New("history store directory is empty")
	}
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read history directory: %w", err)
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		record, err := s.readRecord(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].ReportTime.Equal(records[j].ReportTime) {
			return records[i].AddedAt.After(records[j].AddedAt)
		}
		return records[i].ReportTime.After(records[j].ReportTime)
	})
	return records, nil
}

// Latest returns the newest n records in chronological comparison order.
func (s *Store) Latest(n int) ([]Record, error) {
	if n < 1 {
		return nil, errors.New("history count must be positive")
	}
	records, err := s.List()
	if err != nil {
		return nil, err
	}
	if n > len(records) {
		return nil, fmt.Errorf("history contains %d reports, need %d", len(records), n)
	}
	records = append([]Record(nil), records[:n]...)
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	return records, nil
}

// Get loads one history record by ID.
func (s *Store) Get(id string) (Record, error) {
	if s == nil || strings.TrimSpace(s.Dir) == "" {
		return Record{}, errors.New("history store directory is empty")
	}
	if err := validateID(id); err != nil {
		return Record{}, err
	}
	record, err := s.readRecord(filepath.Join(s.Dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("history report %q not found", id)
	}
	return record, err
}

// Delete removes one history record by ID.
func (s *Store) Delete(id string) error {
	if s == nil || strings.TrimSpace(s.Dir) == "" {
		return errors.New("history store directory is empty")
	}
	if err := validateID(id); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, id+".json")
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("history report %q not found", id)
		}
		return fmt.Errorf("delete history report %q: %w", id, err)
	}
	return nil
}

func detectKind(object map[string]json.RawMessage) (Kind, error) {
	var explicit string
	if raw, ok := object["report_kind"]; ok {
		if err := unmarshalString(raw, &explicit); err != nil {
			return "", fmt.Errorf("invalid report_kind: %w", err)
		}
		switch Kind(strings.ToLower(strings.TrimSpace(explicit))) {
		case KindRun:
			if hasRunShape(object) {
				return KindRun, nil
			}
			return "", errors.New("report_kind is run but results.workloads is missing")
		case KindSuite:
			if hasSuiteShape(object) {
				return KindSuite, nil
			}
			return "", errors.New("report_kind is suite but config or sections are missing")
		default:
			return "", fmt.Errorf("unsupported report_kind %q", explicit)
		}
	}

	if hasRunShape(object) {
		return KindRun, nil
	}
	if hasSuiteShape(object) {
		return KindSuite, nil
	}
	return "", errors.New("JSON is not a recognized vmbench run or suite report")
}

func hasRunShape(object map[string]json.RawMessage) bool {
	raw, ok := object["results"]
	if !ok {
		return false
	}
	var results map[string]json.RawMessage
	if json.Unmarshal(raw, &results) != nil {
		return false
	}
	_, exists := results["workloads"]
	return exists
}

func hasSuiteShape(object map[string]json.RawMessage) bool {
	if _, hasConfig := object["config"]; !hasConfig {
		return false
	}
	for key, raw := range object {
		switch key {
		case "schema_version", "report_kind", "report_id", "app", "system", "config", "version", "timestamp",
			"started_at", "finished_at", "duration_ms", "started_time", "updated_time", "finished_time", "status", "message", "warnings":
			continue
		}
		var section map[string]json.RawMessage
		if json.Unmarshal(raw, &section) != nil || section == nil {
			continue
		}
		for _, marker := range []string{"enabled", "status", "result", "results"} {
			if _, exists := section[marker]; exists {
				return true
			}
		}
	}
	return false
}

func reportTime(object map[string]json.RawMessage) time.Time {
	for _, key := range []string{"started_at", "timestamp", "finished_at"} {
		var value time.Time
		if raw := object[key]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil && !value.IsZero() {
			return value.UTC()
		}
	}
	for _, key := range []string{"started_time", "finished_time", "updated_time"} {
		var value int64
		if raw := object[key]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value > 0 {
			return time.Unix(value, 0).UTC()
		}
	}
	return time.Time{}
}

func (s *Store) availableID(reportAt time.Time, data []byte, tag string) (string, error) {
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		seed = []byte(fmt.Sprintf("%d-%d", time.Now().UTC().UnixNano(), os.Getpid()))
	}
	payload := append(append(append([]byte(nil), data...), []byte(tag)...), seed...)
	hash := sha256.Sum256(payload)
	base := reportAt.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(hash[:4])
	for suffix := 0; ; suffix++ {
		id := base
		if suffix > 0 {
			id = fmt.Sprintf("%s-%d", base, suffix)
		}
		_, err := os.Stat(filepath.Join(s.Dir, id+".json"))
		if errors.Is(err, os.ErrNotExist) {
			return id, nil
		}
		if err != nil {
			return "", fmt.Errorf("reserve history report ID: %w", err)
		}
	}
}

func (s *Store) writeRecord(record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode history report: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.Dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temporary history file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	closeWithError := func() {
		_ = tmp.Close()
	}
	if err := tmp.Chmod(0o600); err != nil {
		closeWithError()
		return fmt.Errorf("secure temporary history file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		closeWithError()
		return fmt.Errorf("write temporary history file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		closeWithError()
		return fmt.Errorf("sync temporary history file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary history file: %w", err)
	}
	finalPath := filepath.Join(s.Dir, record.ID+".json")
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("commit history report: %w", err)
	}
	if err := os.Chmod(finalPath, 0o600); err != nil {
		return fmt.Errorf("secure history report: %w", err)
	}
	if dir, err := os.Open(s.Dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Store) readRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("parse history file %s: %w", path, err)
	}
	if record.StorageVersion != storageVersion || !validID.MatchString(record.ID) || len(record.Report) == 0 {
		return Record{}, fmt.Errorf("invalid history file %s", path)
	}
	meta, err := Inspect(record.Report)
	if err != nil || meta.Kind != record.Kind {
		return Record{}, fmt.Errorf("invalid embedded report in %s", path)
	}
	return record, nil
}

func validateID(id string) error {
	if id != strings.TrimSpace(id) || !validID.MatchString(id) {
		return fmt.Errorf("invalid history report ID %q", id)
	}
	return nil
}

func validateTag(tag string) error {
	if len(tag) > 128 {
		return errors.New("history tag must not exceed 128 bytes")
	}
	for _, r := range tag {
		if unicode.IsControl(r) {
			return errors.New("history tag must not contain control characters")
		}
	}
	return nil
}

func unmarshalString(raw json.RawMessage, target *string) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

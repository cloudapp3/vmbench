package uninstall

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/history"
)

// isolateEnv redirects every vmbench-owned directory into base so tests
// never touch the real machine layout.
func isolateEnv(t *testing.T, base string) {
	t.Helper()
	t.Setenv("VMBENCH_HISTORY_DIR", "")
	t.Setenv("VMBENCH_CONFIG", "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("HOME", base)
}

func itemOfKind(items []Item, kind Kind) *Item {
	for i := range items {
		if items[i].Kind == kind {
			return &items[i]
		}
	}
	return nil
}

func TestPlanRemovesOwnedDirectoriesWithBinaryLast(t *testing.T) {
	base := t.TempDir()
	isolateEnv(t, base)

	dataRoot := filepath.Join(base, "data", "vmbench")
	if err := os.MkdirAll(filepath.Join(dataRoot, "history"), 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(base, "config", "vmbench")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binaries := filepath.Join(base, "cache", "vmbench", "binaries")
	if err := os.MkdirAll(binaries, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fio_x64", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(binaries, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store, err := history.Open(filepath.Join(dataRoot, "history"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add([]byte(`{"results":{"workloads":[]}}`), ""); err != nil {
		t.Fatal(err)
	}

	items, kept, warnings := Plan()
	if len(kept) != 0 {
		t.Fatalf("unexpected kept entries: %v", kept)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	data := itemOfKind(items, KindData)
	if data == nil {
		t.Fatal("plan has no data item")
	}
	if data.Records != 1 {
		t.Fatalf("data item Records = %d, want 1", data.Records)
	}
	if data.Path != dataRoot {
		t.Fatalf("data item Path = %q, want %q", data.Path, dataRoot)
	}
	config := itemOfKind(items, KindConfigDir)
	if config == nil || config.Path != configDir {
		t.Fatalf("config item = %+v, want path %q", config, configDir)
	}
	cache := itemOfKind(items, KindCache)
	if cache == nil {
		t.Fatal("plan has no cache item")
	}
	if !slices.Equal(cache.Tools, []string{"fio"}) {
		t.Fatalf("cache Tools = %v, want [fio]", cache.Tools)
	}
	binary := itemOfKind(items, KindBinary)
	if binary == nil || !binary.Self {
		t.Fatalf("binary item = %+v, want Self", binary)
	}
	if items[len(items)-1].Kind != KindBinary {
		t.Fatalf("binary must be planned last, got %+v", items[len(items)-1])
	}
}

func TestPlanKeepsCustomHistoryDir(t *testing.T) {
	base := t.TempDir()
	isolateEnv(t, base)
	custom := filepath.Join(base, "elsewhere", "reports")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMBENCH_HISTORY_DIR", custom)

	items, kept, _ := Plan()
	if itemOfKind(items, KindData) != nil {
		t.Fatal("redirected history must not be planned for removal")
	}
	if len(kept) != 1 || kept[0].Path != custom || kept[0].Reason != KeptReasonHistoryOverride {
		t.Fatalf("kept = %+v, want [%s] with reason %s", kept, custom, KeptReasonHistoryOverride)
	}
}

func TestPlanRedirectedConfigRemovesFileOnly(t *testing.T) {
	base := t.TempDir()
	isolateEnv(t, base)
	configFile := filepath.Join(base, "custom", "vmbench.json")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VMBENCH_CONFIG", configFile)

	items, _, _ := Plan()
	if item := itemOfKind(items, KindConfigFile); item == nil || item.Path != configFile {
		t.Fatalf("config-file item = %+v, want %q", itemOfKind(items, KindConfigFile), configFile)
	}
	if item := itemOfKind(items, KindConfigDir); item != nil {
		t.Fatalf("redirected config must not plan its directory, got %+v", item)
	}
}

// TestPlanDedupsConfigIntoDataRoot simulates the macOS layout on any
// platform: both resolution paths land in the same directory, so only the
// data item may be planned.
func TestPlanDedupsConfigIntoDataRoot(t *testing.T) {
	base := t.TempDir()
	isolateEnv(t, base)
	shared := filepath.Join(base, "shared")
	t.Setenv("XDG_DATA_HOME", shared)
	t.Setenv("XDG_CONFIG_HOME", shared)
	if err := os.MkdirAll(filepath.Join(shared, "vmbench"), 0o755); err != nil {
		t.Fatal(err)
	}

	items, _, warnings := Plan()
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	count := 0
	for _, item := range items {
		if item.Kind == KindData || item.Kind == KindConfigDir {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one data/config item for the shared root, got %d: %+v", count, items)
	}
}

func TestDirRemovalBlocker(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path string
		want string
	}{
		{dir, ""},
		{link, "symbolic link"},
		{file, "not a directory"},
		{"/", "protected path"},
		{"/etc", "protected path"},
		{base, ""}, // HOME is base, but base itself is only protected by equality below
	}
	for _, tc := range cases {
		if got := dirRemovalBlocker(tc.path); got != tc.want {
			t.Errorf("dirRemovalBlocker(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
	t.Setenv("HOME", base)
	if got := dirRemovalBlocker(base); got != "protected path" {
		t.Errorf("dirRemovalBlocker(HOME) = %q, want protected path", got)
	}
	if got := dirRemovalBlocker("relative/path"); got != "not an absolute path" {
		t.Errorf("relative path blocker = %q", got)
	}
}

func TestFileRemovalBlocker(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if got := fileRemovalBlocker(file); got != "" {
		t.Errorf("fileRemovalBlocker(regular file) = %q", got)
	}
	if got := fileRemovalBlocker(link); got != "symbolic link" {
		t.Errorf("fileRemovalBlocker(symlink) = %q", got)
	}
	if got := fileRemovalBlocker(base); got != "not a regular file" {
		t.Errorf("fileRemovalBlocker(directory) = %q", got)
	}
	if got := fileRemovalBlocker("/bin"); got != "protected path" {
		t.Errorf("fileRemovalBlocker(/bin) = %q", got)
	}
}

func TestExecuteRemovesDirectoriesThenBinary(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "owned")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(dir, "inner.txt")
	if err := os.WriteFile(inner, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(base, "config.json")
	if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(base, "vmbench")
	if err := os.WriteFile(binary, []byte("elf"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	result := Execute(&out, []Item{
		{Kind: KindData, Path: dir},
		{Kind: KindConfigFile, Path: configFile},
		{Kind: KindBinary, Path: binary, Self: true},
	})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	removed := strings.Join(result.Removed, "\n")
	if !strings.Contains(removed, dir) || !strings.Contains(removed, configFile) || !strings.Contains(removed, binary) {
		t.Fatalf("removed = %v", result.Removed)
	}
	if !strings.Contains(result.Removed[len(result.Removed)-1], binary) {
		t.Fatalf("binary must be removed last, removed = %v", result.Removed)
	}
	for _, path := range []string{dir, configFile, binary} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s still exists: %v", path, err)
		}
	}
	if !strings.Contains(out.String(), "removed") {
		t.Fatalf("progress output missing: %q", out.String())
	}
}

func TestExecuteKeepsBinaryWhenRemovalFails(t *testing.T) {
	base := t.TempDir()
	// A symlink standing in for "directory changed since planning": the
	// removal must be refused and the binary kept for retry.
	target := filepath.Join(base, "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "owned")
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(base, "vmbench")
	if err := os.WriteFile(binary, []byte("elf"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	result := Execute(&out, []Item{
		{Kind: KindData, Path: dir},
		{Kind: KindBinary, Path: binary, Self: true},
	})
	if len(result.Errors) < 2 {
		t.Fatalf("expected refusal and kept-binary errors, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[len(result.Errors)-1], "kept") {
		t.Fatalf("last error must report the kept binary, got %v", result.Errors)
	}
	if _, err := os.Lstat(binary); err != nil {
		t.Fatalf("binary must survive a failed removal: %v", err)
	}
	if _, err := os.Lstat(dir); err != nil {
		t.Fatalf("symlink must survive the refusal: %v", err)
	}
}

func TestExecuteToleratesAbsentPaths(t *testing.T) {
	base := t.TempDir()
	var out strings.Builder
	result := Execute(&out, []Item{
		{Kind: KindData, Path: filepath.Join(base, "gone")},
		{Kind: KindConfigFile, Path: filepath.Join(base, "gone.json")},
	})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("absent paths must not be reported removed: %v", result.Removed)
	}
}

func TestExecuteRefusesChangedDirectory(t *testing.T) {
	base := t.TempDir()
	other := filepath.Join(base, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "owned")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Swap in a symlink after "planning": the directory removal must refuse
	// instead of following it.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, dir); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	result := Execute(&out, []Item{{Kind: KindData, Path: dir}})
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "refusing") {
		t.Fatalf("errors = %v, want a refusal", result.Errors)
	}
	if _, err := os.Lstat(dir); err != nil {
		t.Fatalf("symlink must survive the refusal: %v", err)
	}
	if _, err := os.Lstat(other); err != nil {
		t.Fatalf("symlink target must be untouched: %v", err)
	}
}

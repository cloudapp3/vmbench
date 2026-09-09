// Package uninstall plans and performs the removal of everything vmbench
// creates on a machine: the benchmark history, the TUI preferences, the
// fetched-tool cache, and finally the running binary. Shell startup files
// are deliberately untouched — installer-owned PATH entries belong to
// install.sh, which delegates here and cleans them afterwards — and a
// custom VMBENCH_HISTORY_DIR or VMBENCH_CONFIG location is preserved.
//
// The flow is plan → confirm → execute. Plan probes the machine and returns
// the ordered items to remove (owned directories first, the running binary
// last). Execute re-validates every item immediately before removal and
// keeps the binary whenever an earlier removal failed, so a failed
// uninstall stays retryable.
package uninstall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/toolbin"
	"github.com/cloudapp3/vmbench/tui"
)

// Kind classifies a removal item; the command layer maps it to localized
// text.
type Kind string

const (
	KindData       Kind = "data"        // benchmark history root
	KindConfigDir  Kind = "config-dir"  // TUI preferences directory
	KindConfigFile Kind = "config-file" // TUI preferences file (VMBENCH_CONFIG redirect)
	KindCache      Kind = "cache"       // fetched static tool binaries
	KindBinary     Kind = "binary"      // the running executable
)

// KeptReasonHistoryOverride marks a history location redirected through
// VMBENCH_HISTORY_DIR, which uninstall deliberately preserves.
const KeptReasonHistoryOverride = "custom_history_dir"

// Item is one path planned for removal.
type Item struct {
	Kind Kind
	Path string
	// Self marks the running binary, which is always removed last.
	Self bool
	// Records is the number of stored benchmark reports (KindData only).
	Records int
	// Tools names the fetched static tool binaries (KindCache only).
	Tools []string
}

// Kept describes a location uninstall deliberately preserves.
type Kept struct {
	Path   string
	Reason string
}

// Result reports what Execute removed and the problems it hit. Errors is
// non-empty exactly when the removal was incomplete.
type Result struct {
	Removed []string
	Errors  []string
}

// Plan probes the machine and returns the removal plan, the locations kept
// on purpose, and non-fatal warnings. Only paths that currently exist are
// planned.
func Plan() (items []Item, kept []Kept, warnings []string) {
	p := &planner{}
	p.addData()
	p.addConfig()
	p.addCache()
	p.addBinary()
	return p.items, p.kept, p.warnings
}

type planner struct {
	items    []Item
	kept     []Kept
	warnings []string
}

func (p *planner) warnf(format string, args ...any) {
	p.warnings = append(p.warnings, fmt.Sprintf(format, args...))
}

// addData plans the vmbench data root unless VMBENCH_HISTORY_DIR redirected
// history outside vmbench-owned storage; the custom location is then kept
// and nothing under the default root is touched.
func (p *planner) addData() {
	if override := strings.TrimSpace(os.Getenv("VMBENCH_HISTORY_DIR")); override != "" {
		p.kept = append(p.kept, Kept{Path: filepath.Clean(override), Reason: KeptReasonHistoryOverride})
		return
	}
	root, err := history.DefaultRoot()
	if err != nil {
		p.warnf("could not resolve the vmbench data directory: %v", err)
		return
	}
	if !pathLexists(root) {
		return
	}
	if blocker := dirRemovalBlocker(root); blocker != "" {
		p.warnf("leaving data directory %s in place: %s", root, blocker)
		return
	}
	item := Item{Kind: KindData, Path: filepath.Clean(root)}
	if store, err := history.Open(filepath.Join(root, "history")); err == nil {
		if records, err := store.List(); err == nil {
			item.Records = len(records)
		}
	}
	p.items = append(p.items, item)
}

// addConfig plans the TUI preferences directory, or only the preferences
// file when VMBENCH_CONFIG redirected it outside vmbench-owned storage. On
// platforms where the preferences directory equals the data root (macOS),
// the data item already covers it.
func (p *planner) addConfig() {
	file, dir := tui.ConfigPaths()
	if dir != "" {
		if !pathLexists(dir) || p.plansPath(dir) {
			return
		}
		if blocker := dirRemovalBlocker(dir); blocker != "" {
			p.warnf("leaving config directory %s in place: %s", dir, blocker)
			return
		}
		p.items = append(p.items, Item{Kind: KindConfigDir, Path: filepath.Clean(dir)})
		return
	}
	if file == "" || !pathLexists(file) {
		return
	}
	if blocker := fileRemovalBlocker(file); blocker != "" {
		p.warnf("leaving config file %s in place: %s", file, blocker)
		return
	}
	p.items = append(p.items, Item{Kind: KindConfigFile, Path: filepath.Clean(file)})
}

// addCache plans the fetched-tool cache root: the parent of the directory
// where `vmbench tools fetch` installs pinned static tool binaries.
func (p *planner) addCache() {
	binaries, err := toolbin.CacheDir()
	if err != nil {
		p.warnf("could not resolve the fetched-tools cache directory: %v", err)
		return
	}
	root := filepath.Dir(binaries)
	if !pathLexists(root) || p.plansPath(root) {
		return
	}
	if blocker := dirRemovalBlocker(root); blocker != "" {
		p.warnf("leaving fetched-tools cache directory %s in place: %s", root, blocker)
		return
	}
	p.items = append(p.items, Item{Kind: KindCache, Path: filepath.Clean(root), Tools: fetchedToolNames(binaries)})
}

// addBinary plans the running executable last. It warns when the binary is
// managed by a package manager (removal leaves the package database stale)
// and when a manually created service unit still references vmbench —
// uninstall removes files, never services.
func (p *planner) addBinary() {
	exe, err := os.Executable()
	if err != nil {
		p.warnf("could not determine the vmbench binary path: %v", err)
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if owner := packageOwner(exe); owner != "" {
		p.warnf("binary %s is owned by a package manager (%s); removing it leaves the package database stale — `apt remove`/`yum remove` is cleaner", exe, owner)
	}
	if unit := manualServiceUnit(); unit != "" && pathLexists(unit) {
		p.warnf("a manually created service unit %s was found; vmbench does not stop or remove it — run install.sh --uninstall (which stops the service safely) or remove it manually", unit)
	}
	if blocker := fileRemovalBlocker(exe); blocker != "" {
		p.warnf("leaving binary %s in place: %s", exe, blocker)
		return
	}
	p.items = append(p.items, Item{Kind: KindBinary, Path: filepath.Clean(exe), Self: true})
}

// plansPath reports whether an already-planned directory item covers path.
func (p *planner) plansPath(path string) bool {
	path = filepath.Clean(path)
	for _, item := range p.items {
		if item.Kind == KindBinary || item.Kind == KindConfigFile {
			continue
		}
		dir := filepath.Clean(item.Path)
		if samePath(dir, path) {
			return true
		}
		if rel, err := filepath.Rel(dir, path); err == nil && rel != "." && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

// Execute removes the planned items. Directories go first in the caller's
// order; the running binary goes last and is kept whenever an earlier
// removal failed, leaving the uninstall retryable. Every item is
// re-validated immediately before removal: a path that changed since Plan
// is refused, and an already-absent path is skipped without error, so
// re-running is safe.
func Execute(w io.Writer, items []Item) Result {
	result := Result{}
	run := func(item Item) {
		var problems []string
		removed := false
		switch item.Kind {
		case KindBinary:
			if blocker := fileRemovalBlocker(item.Path); blocker != "" {
				problems = append(problems, fmt.Sprintf("refusing to remove %s: %s", item.Path, blocker))
				break
			}
			removed, problems = removeExecutable(item.Path)
		case KindConfigFile:
			removed, problems = removeFile(w, item.Path)
		default:
			removed, problems = removeDir(w, item.Path)
		}
		if len(problems) == 0 && removed {
			result.Removed = append(result.Removed, item.Path)
		}
		result.Errors = append(result.Errors, problems...)
	}
	var binary *Item
	for _, item := range items {
		if item.Self {
			binary = &Item{Kind: item.Kind, Path: item.Path, Self: item.Self}
			continue
		}
		run(item)
	}
	if binary == nil {
		return result
	}
	if len(result.Errors) > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("kept %s because earlier removals failed; rerun to retry", binary.Path))
		return result
	}
	run(*binary)
	return result
}

// removeDir re-validates and recursively removes one planned directory. An
// already-absent path is not an error. removed reports whether the path was
// actually removed.
func removeDir(w io.Writer, path string) (removed bool, problems []string) {
	if blocker := dirRemovalBlocker(path); blocker != "" {
		return false, []string{fmt.Sprintf("refusing to remove %s: %s", path, blocker)}
	}
	if _, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			return false, []string{fmt.Sprintf("%s: %v", path, err)}
		}
		return false, nil
	}
	if err := os.RemoveAll(path); err != nil {
		return false, []string{fmt.Sprintf("%s: %v", path, err)}
	}
	printRemoved(w, path)
	return true, nil
}

// removeFile re-validates and removes one planned regular file.
func removeFile(w io.Writer, path string) (removed bool, problems []string) {
	if blocker := fileRemovalBlocker(path); blocker != "" {
		return false, []string{fmt.Sprintf("refusing to remove %s: %s", path, blocker)}
	}
	err := os.Remove(path)
	switch {
	case err == nil:
		printRemoved(w, path)
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, []string{fmt.Sprintf("%s: %v", path, err)}
	}
}

func printRemoved(w io.Writer, path string) {
	fmt.Fprintln(w, i18n.Tf("cli.uninstall.output.removed", map[string]any{"Path": path}))
}

// dirRemovalBlocker returns why path must not be removed recursively, or ""
// when removal is safe. Absent paths are not blockers; callers skip them.
func dirRemovalBlocker(path string) string {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "not an absolute path"
	}
	if isProtectedPath(clean) {
		return "protected path"
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symbolic link"
	}
	if !info.IsDir() {
		return "not a directory"
	}
	return ""
}

// fileRemovalBlocker guards regular-file removals (the redirected config
// file and the binary): absolute, unprotected, a regular file — never a
// directory or symlink.
func fileRemovalBlocker(path string) string {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "not an absolute path"
	}
	if isProtectedPath(clean) {
		return "protected path"
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symbolic link"
	}
	if !info.Mode().IsRegular() {
		return "not a regular file"
	}
	return ""
}

// isProtectedPath reports whether removing clean would destroy a system
// root or the user's home directory.
func isProtectedPath(clean string) bool {
	if platformProtectedPath(clean) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return samePath(clean, filepath.Clean(home))
	}
	return false
}

// fetchedToolNames maps the pinned tool binaries fetched by
// `vmbench tools fetch` (<name>_<arch> files) back to tool names; unknown
// files are ignored.
func fetchedToolNames(binaries string) []string {
	entries, err := os.ReadDir(binaries)
	if err != nil {
		return nil
	}
	suffix := "_" + toolbin.ArchSuffix(runtime.GOARCH)
	var names []string
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), suffix)
		if name == entry.Name() {
			continue
		}
		if _, ok := toolbin.Find(name); ok && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// packageOwner returns a "tool: package" label when path is managed by dpkg
// or rpm, or "" otherwise. Missing package managers (macOS, Windows, most
// containers) simply resolve to "".
func packageOwner(path string) string {
	for _, argv := range [][]string{
		{"dpkg-query", "-S", path},
		{"rpm", "-qf", path},
	} {
		out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(out))
		if s == "" {
			continue
		}
		// dpkg-query prints "package: path"; rpm prints the package name.
		owner := s
		if pkg, _, found := strings.Cut(s, ":"); found {
			owner = strings.TrimSpace(pkg)
		}
		return argv[0] + ": " + owner
	}
	return ""
}

func pathLexists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

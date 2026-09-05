package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

// Definition describes a workload without instantiating its heavy data payloads.
type Definition struct {
	Name        string
	Category    string
	Description string
	Factory     func(string) bench.Workload
}

// HardwareToolSpec describes a selectable external hardware benchmark tool.
type HardwareToolSpec struct {
	ID          string
	Name        string
	Description string
	Default     bool
}

const (
	ScopeAll      = "all"
	ScopeHardware = "hardware"
	ScopeNetwork  = "network"
)

const (
	HardwareToolSysbench  = "sysbench"
	HardwareToolOpenSSL   = "openssl"
	HardwareToolFio       = "fio"
	HardwareToolDD        = "dd"
	HardwareToolStream    = "stream"
	HardwareToolMBW       = "mbw"
	HardwareToolGeekbench = "geekbench"
	HardwareToolWinSAT    = "winsat"
)

var defaultHardwareToolOrder = []string{HardwareToolSysbench, HardwareToolOpenSSL, HardwareToolFio}

var hardwareToolOrder = []string{
	HardwareToolSysbench,
	HardwareToolOpenSSL,
	HardwareToolFio,
	HardwareToolDD,
	HardwareToolStream,
	HardwareToolMBW,
	HardwareToolGeekbench,
	HardwareToolWinSAT,
}

var hardwareToolSpecs = map[string]HardwareToolSpec{
	HardwareToolSysbench: {
		ID:          HardwareToolSysbench,
		Name:        "sysbench",
		Description: "CPU single/multi-core prime and memory bandwidth.",
		Default:     true,
	},
	HardwareToolOpenSSL: {
		ID:          HardwareToolOpenSSL,
		Name:        "OpenSSL",
		Description: "CPU crypto throughput via openssl speed.",
		Default:     true,
	},
	HardwareToolFio: {
		ID:          HardwareToolFio,
		Name:        "fio",
		Description: "Disk sequential and random 4K I/O.",
		Default:     true,
	},
	HardwareToolDD: {
		ID:          HardwareToolDD,
		Name:        "dd",
		Description: "Disk sequential write/read throughput.",
	},
	HardwareToolStream: {
		ID:          HardwareToolStream,
		Name:        "STREAM",
		Description: "Memory bandwidth from STREAM Copy/Scale/Add/Triad.",
	},
	HardwareToolMBW: {
		ID:          HardwareToolMBW,
		Name:        "mbw",
		Description: "Memory copy bandwidth via mbw.",
	},
	HardwareToolGeekbench: {
		ID:          HardwareToolGeekbench,
		Name:        "Geekbench",
		Description: "Optional upstream Geekbench CPU score; not enabled by default.",
	},
	HardwareToolWinSAT: {
		ID:          HardwareToolWinSAT,
		Name:        "WinSAT",
		Description: "Optional Windows System Assessment Tool CPU/memory/disk probes.",
	},
}

// DefinitionsForEngine returns workloads based on the engine selection.
func DefinitionsForEngine(engine, diskPath string, iperfHosts []string) []Definition {
	return DefinitionsForScope(engine, ScopeAll, diskPath, iperfHosts)
}

// DefinitionsForScope returns workloads based on engine and high-level scope.
//
// Hardware measurements are external-tool based. Legacy engine values such as
// "native" and "full" are accepted for API compatibility, but they no longer
// register in-process CPU/memory/disk benchmark workloads.
func DefinitionsForScope(engine, scope, diskPath string, iperfHosts []string) []Definition {
	return DefinitionsForScopeWithHardwareTools(engine, scope, diskPath, iperfHosts, nil)
}

// DefinitionsForScopeWithHardwareTools returns workloads for a scope using the
// selected external hardware tools. Empty hardwareTools selects the default set.
func DefinitionsForScopeWithHardwareTools(engine, scope, diskPath string, iperfHosts []string, hardwareTools []string) []Definition {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case ScopeHardware:
		return ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
	case ScopeNetwork:
		return NetworkDefinitions(iperfHosts)
	case ScopeAll:
		all := ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
		all = append(all, NetworkDefinitions(iperfHosts)...)
		_ = engine
		return all
	default:
		return ExternalHardwareDefinitionsForTools(diskPath, hardwareTools)
	}
}

// NativeDefinitions is retained for API compatibility.
//
// Deprecated: hardware benchmark workloads are external-tool based; this
// function returns the same external definitions as DefaultDefinitions.
func NativeDefinitions(diskPath string) []Definition {
	defs := ExternalHardwareDefinitionsForTools(diskPath, nil)
	defs = append(defs, NetworkDefinitions(nil)...)
	return defs
}

// NativeHardwareDefinitions is retained for API compatibility.
//
// Deprecated: hardware benchmark workloads are external-tool based; this
// function returns ExternalHardwareDefinitions.
func NativeHardwareDefinitions(diskPath string) []Definition {
	return ExternalHardwareDefinitionsForTools(diskPath, nil)
}

// ExternalDefinitions returns workloads that wrap external tools plus network diagnostics.
func ExternalDefinitions(diskPath string, iperfHosts []string) []Definition {
	defs := ExternalHardwareDefinitionsForTools(diskPath, nil)
	defs = append(defs, NetworkDefinitions(iperfHosts)...)
	return defs
}

// ExternalHardwareDefinitions returns hardware workloads that wrap external
// benchmark tools. Missing tools are reported by the workload itself; this
// registry intentionally does not fall back to in-process benchmark code.
func ExternalHardwareDefinitions(diskPath string) []Definition {
	return ExternalHardwareDefinitionsForTools(diskPath, nil)
}

// ExternalHardwareDefinitionsForTools returns hardware workloads that wrap the
// selected external tools. Missing tools are reported by workload execution.
func ExternalHardwareDefinitionsForTools(diskPath string, hardwareTools []string) []Definition {
	defs := []Definition{}
	tools := StandardizeHardwareTools(hardwareTools)
	if len(tools) == 0 && len(hardwareTools) == 0 {
		tools = DefaultHardwareTools()
	}
	enabled := make(map[string]bool, len(tools))
	for _, tool := range tools {
		enabled[tool] = true
	}

	// CPU + memory — sysbench from PATH or an optional local Linux binary.
	if enabled[HardwareToolSysbench] {
		defs = append(defs,
			Definition{Name: "CPU Single-Core (sysbench)", Category: "CPU", Description: "sysbench CPU prime, 1 thread", Factory: func(string) bench.Workload {
				return &sysbenchWorkload{threads: 1, maxPrime: 20000}
			}},
			Definition{Name: "CPU Multi-Core (sysbench)", Category: "CPU", Description: "sysbench CPU prime, all threads", Factory: func(string) bench.Workload {
				return &sysbenchWorkload{threads: runtime.NumCPU(), maxPrime: 20000, multi: true}
			}},
			Definition{Name: "Memory Read Bandwidth (sysbench)", Category: bench.CategoryMemory, Description: "sysbench sequential memory read bandwidth", Factory: func(string) bench.Workload {
				return &sysbenchMemoryWorkload{threads: 1, operation: "read", accessMode: "seq", blockSize: "1M", totalSize: "10G", durationSeconds: 3}
			}},
			Definition{Name: "Memory Write Bandwidth (sysbench)", Category: bench.CategoryMemory, Description: "sysbench sequential memory write bandwidth", Factory: func(string) bench.Workload {
				return &sysbenchMemoryWorkload{threads: 1, operation: "write", accessMode: "seq", blockSize: "1M", totalSize: "10G", durationSeconds: 3}
			}},
			Definition{Name: "Memory Random Read Latency (sysbench)", Category: bench.CategoryMemory, Description: "sysbench random memory read latency, 64B blocks", Factory: func(string) bench.Workload {
				return &sysbenchMemoryWorkload{threads: 1, operation: "read", accessMode: "rnd", blockSize: "64", totalSize: "10G", durationSeconds: 3}
			}},
		)
	}

	// CPU — openssl speed.
	if enabled[HardwareToolOpenSSL] {
		defs = append(defs,
			Definition{Name: "OpenSSL AES-256-CBC", Category: "CPU", Description: "openssl speed aes-256-cbc", Factory: func(string) bench.Workload {
				return &opensslWorkload{algo: "aes-256-cbc", seconds: 3}
			}},
			Definition{Name: "OpenSSL SHA256", Category: "CPU", Description: "openssl speed sha256", Factory: func(string) bench.Workload {
				return &opensslWorkload{algo: "sha256", seconds: 3}
			}},
		)
	}

	// Disk — fio from PATH or an optional local Linux binary.
	if enabled[HardwareToolFio] {
		defs = append(defs,
			Definition{Name: "Disk 4K Random Read Q1 (fio)", Category: "Disk", Description: "fio 4K random read, iodepth=1", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "randread", bs: "4k", size: "256M", iodepth: 1, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 4K Random Read Q32 (fio)", Category: "Disk", Description: "fio 4K random read, iodepth=32", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "randread", bs: "4k", size: "256M", iodepth: 32, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 4K Random Write Q1 (fio)", Category: "Disk", Description: "fio 4K random write, iodepth=1", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "randwrite", bs: "4k", size: "256M", iodepth: 1, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 4K Random Write Q32 (fio)", Category: "Disk", Description: "fio 4K random write, iodepth=32", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "randwrite", bs: "4k", size: "256M", iodepth: 32, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 1M Sequential Read Q1 (fio)", Category: "Disk", Description: "fio 1M sequential read, iodepth=1", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "read", bs: "1M", size: "256M", iodepth: 1, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 1M Sequential Read Q8 (fio)", Category: "Disk", Description: "fio 1M sequential read, iodepth=8", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "read", bs: "1M", size: "256M", iodepth: 8, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 1M Sequential Write Q1 (fio)", Category: "Disk", Description: "fio 1M sequential write, iodepth=1", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "write", bs: "1M", size: "256M", iodepth: 1, runtimeSeconds: 3}
			}},
			Definition{Name: "Disk 1M Sequential Write Q8 (fio)", Category: "Disk", Description: "fio 1M sequential write, iodepth=8", Factory: func(dp string) bench.Workload {
				return &fioWorkload{dir: dp, rw: "write", bs: "1M", size: "256M", iodepth: 8, runtimeSeconds: 3}
			}},
		)
	}

	if enabled[HardwareToolDD] {
		defs = append(defs,
			Definition{Name: "Disk Write (dd)", Category: "Disk", Description: "dd sequential write, 256 MiB", Factory: func(dp string) bench.Workload {
				return &ddWorkload{dir: dp, operation: "write", sizeMiB: 256}
			}},
			Definition{Name: "Disk Read (dd)", Category: "Disk", Description: "dd sequential read, 256 MiB", Factory: func(dp string) bench.Workload {
				return &ddWorkload{dir: dp, operation: "read", sizeMiB: 256}
			}},
		)
	}

	if enabled[HardwareToolStream] {
		defs = append(defs, Definition{Name: "Memory Bandwidth (STREAM)", Category: bench.CategoryMemory, Description: "STREAM memory bandwidth best kernel", Factory: func(string) bench.Workload {
			return &streamWorkload{}
		}})
	}

	if enabled[HardwareToolMBW] {
		defs = append(defs, Definition{Name: "Memory Bandwidth (mbw)", Category: bench.CategoryMemory, Description: "mbw memory copy bandwidth, 256 MiB", Factory: func(string) bench.Workload {
			return &mbwWorkload{sizeMiB: 256}
		}})
	}

	if enabled[HardwareToolGeekbench] {
		defs = append(defs, Definition{Name: "Geekbench CPU", Category: "CPU", Description: "Geekbench CPU benchmark, upstream score", Factory: func(string) bench.Workload {
			return &geekbenchWorkload{}
		}})
	}

	if enabled[HardwareToolWinSAT] {
		defs = append(defs,
			Definition{Name: "WinSAT CPU", Category: "CPU", Description: "winsat cpu external benchmark", Factory: func(string) bench.Workload {
				return &winsatWorkload{kind: "cpu"}
			}},
			Definition{Name: "WinSAT Memory", Category: bench.CategoryMemory, Description: "winsat mem external benchmark", Factory: func(string) bench.Workload {
				return &winsatWorkload{kind: "mem"}
			}},
			Definition{Name: "WinSAT Disk", Category: "Disk", Description: "winsat disk sequential read benchmark", Factory: func(dp string) bench.Workload {
				return &winsatWorkload{kind: "disk", dir: dp}
			}},
		)
	}

	_ = diskPath
	return defs
}

// HardwareTools returns selectable external hardware tool metadata in display order.
func HardwareTools() []HardwareToolSpec {
	out := make([]HardwareToolSpec, 0, len(hardwareToolOrder))
	defaults := make(map[string]struct{})
	for _, id := range DefaultHardwareTools() {
		defaults[id] = struct{}{}
	}
	for _, id := range hardwareToolOrder {
		spec := hardwareToolSpecs[id]
		_, spec.Default = defaults[id]
		out = append(out, spec)
	}
	return out
}

// HardwareToolIDs returns selectable external hardware tool IDs in display order.
func HardwareToolIDs() []string {
	return append([]string(nil), hardwareToolOrder...)
}

// MissingHardwareTools returns selected adapters whose executable cannot be
// resolved from PATH or the executable-adjacent binaries directory.
func MissingHardwareTools(tools []string) []string {
	return MissingHardwareToolsForFilter(tools, nil)
}

// MissingHardwareToolsForFilter returns missing adapters only when at least
// one of their workload definitions matches the same name/category filter used
// by the runner.
func MissingHardwareToolsForFilter(tools []string, filter *regexp.Regexp) []string {
	selected := StandardizeHardwareTools(tools)
	if len(selected) == 0 && len(tools) == 0 {
		selected = DefaultHardwareTools()
	}
	missing := make([]string, 0, len(selected))
	for _, tool := range selected {
		if !hardwareToolMatchesFilter(tool, filter) {
			continue
		}
		if hardwareToolAvailable(tool) {
			continue
		}
		missing = append(missing, tool)
	}
	return missing
}

func hardwareToolMatchesFilter(tool string, filter *regexp.Regexp) bool {
	if filter == nil {
		return true
	}
	for _, def := range ExternalHardwareDefinitionsForTools("", []string{tool}) {
		if filter.MatchString(def.Name) || filter.MatchString(def.Category) {
			return true
		}
	}
	return false
}

func hardwareToolAvailable(tool string) bool {
	var err error
	switch tool {
	case HardwareToolSysbench, HardwareToolOpenSSL, HardwareToolFio, HardwareToolDD, HardwareToolMBW, HardwareToolWinSAT:
		_, err = resolveTool(tool)
	case HardwareToolStream:
		_, _, err = resolveAnyTool("stream", "stream_c")
	case HardwareToolGeekbench:
		_, _, err = resolveAnyTool("geekbench6", "geekbench5", "geekbench4", "geekbench")
	default:
		return false
	}
	return err == nil
}

// DefaultHardwareTools returns the default hardware tools. Geekbench/WinSAT and
// auxiliary dd/STREAM/mbw probes are opt-in.
func DefaultHardwareTools() []string {
	return defaultHardwareTools(runtime.GOOS)
}

func defaultHardwareTools(goos string) []string {
	switch goos {
	case "darwin":
		return []string{HardwareToolOpenSSL}
	case "windows":
		return []string{HardwareToolWinSAT}
	default:
		return append([]string(nil), defaultHardwareToolOrder...)
	}
}

// StandardizeHardwareTools normalizes aliases, removes duplicates, and returns
// tools in stable display order. The special value "all" enables every adapter.
func StandardizeHardwareTools(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		value := normalizeHardwareToolID(item)
		if value == "" {
			continue
		}
		if value == "all" {
			for _, id := range hardwareToolOrder {
				seen[id] = struct{}{}
			}
			continue
		}
		if _, ok := hardwareToolSpecs[value]; !ok {
			continue
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, id := range hardwareToolOrder {
		if _, ok := seen[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func normalizeHardwareToolID(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "all", "*":
		return "all"
	case "sysbench", "sb":
		return HardwareToolSysbench
	case "openssl", "ssl":
		return HardwareToolOpenSSL
	case "fio":
		return HardwareToolFio
	case "dd":
		return HardwareToolDD
	case "stream", "stream_c", "stream-c":
		return HardwareToolStream
	case "mbw":
		return HardwareToolMBW
	case "geekbench", "gb", "gb4", "gb5", "gb6", "geekbench4", "geekbench5", "geekbench6":
		return HardwareToolGeekbench
	case "winsat", "win-sat":
		return HardwareToolWinSAT
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// NetworkDefinitions returns network diagnostics and speed tests.
func NetworkDefinitions(iperfHosts []string) []Definition {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return NetworkDefinitionsWithManifest(iperfHosts, manifest, "v4")
}

// NetworkDefinitionsWithManifest returns network workloads pinned to one
// validated catalog snapshot. No workload falls back to embedded nodes.
func NetworkDefinitionsWithManifest(iperfHosts []string, manifest nodecatalog.Manifest, ipFamily string) []Definition {
	nodes := netio.SpeedNodesFromManifest(manifest)
	defs := make([]Definition, 0, len(nodes)+6+len(iperfHosts))
	for _, node := range nodes {
		n := node
		defs = append(defs, Definition{
			Name:        fmt.Sprintf("Net Download (%s)", n.Name),
			Category:    bench.CategoryNetwork,
			Description: fmt.Sprintf("HTTP download from %s [%s]", n.Name, n.Region),
			Factory:     func(string) bench.Workload { return netio.NewDownloadWorkload(n) },
		})
	}
	defs = append(defs,
		Definition{Name: "Net Ping", Category: bench.CategoryNetwork, Description: "TCP latency / jitter / packet loss to versioned nodes", Factory: func(string) bench.Workload {
			return netio.NewPingWorkloadWithManifest(manifest, ipFamily)
		}},
		Definition{Name: "Net Multi-Thread Download", Category: bench.CategoryNetwork, Description: "Concurrent download (4 threads, Cloudflare)", Factory: func(string) bench.Workload { return netio.NewMultiDownloadWorkload() }},
		Definition{Name: "Net Upload", Category: bench.CategoryNetwork, Description: "Upload speed via Cloudflare (50MB)", Factory: func(string) bench.Workload { return netio.NewUploadWorkload() }},
		Definition{Name: "Net Streaming Unlock", Category: bench.CategoryNetwork, Description: "UnlockTests streaming / AI platform unlock detection", Factory: func(string) bench.Workload { return netio.NewStreamingUnlockWorkload() }},
		Definition{Name: "Net Traceroute", Category: bench.CategoryNetwork, Description: "TCP traceroute to versioned China carrier, CERNET, and CSTNET targets", Factory: func(string) bench.Workload {
			return netio.NewTracerouteWorkloadWithManifest(manifest, ipFamily)
		}},
		Definition{Name: "Net IP Quality", Category: bench.CategoryNetwork, Description: "IP reputation / DNSBL / mail port detection", Factory: func(string) bench.Workload { return netio.NewIPQualityWorkload() }},
	)
	for _, host := range iperfHosts {
		h := host
		defs = append(defs, Definition{
			Name:        "Network Bandwidth (iperf3)",
			Category:    bench.CategoryNetwork,
			Description: "iperf3 TCP to " + h,
			Factory:     func(string) bench.Workload { return netio.NewIperfWorkload(h, 10) },
		})
	}
	return defs
}

// DefaultDefinitions returns the default external-tool catalog (for list command).
func DefaultDefinitions(includeExtensions bool) []Definition {
	defs := ExternalHardwareDefinitionsForTools("", nil)
	if includeExtensions {
		defs = append(defs, NetworkDefinitions(nil)...)
	}
	return defs
}

// DefaultWorkloads instantiates the default external-tool workload registry.
func DefaultWorkloads(diskPath string, includeExtensions bool, filter *regexp.Regexp) []bench.Workload {
	defs := ExternalHardwareDefinitionsForTools(diskPath, nil)
	if includeExtensions {
		defs = append(defs, NetworkDefinitions(nil)...)
	}
	out := make([]bench.Workload, 0, len(defs))
	for _, def := range defs {
		if filter != nil && !filter.MatchString(def.Name) && !filter.MatchString(def.Category) {
			continue
		}
		out = append(out, def.Factory(diskPath))
	}
	return out
}

// --- helpers ---

func resolveTool(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, candidate := range localToolCandidates(name) {
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH or executable-adjacent binaries directory", name)
}

func resolveAnyTool(names ...string) (string, string, error) {
	var errs []string
	for _, name := range names {
		path, err := resolveTool(name)
		if err == nil {
			return path, name, nil
		}
		errs = append(errs, err.Error())
	}
	return "", "", errors.New(strings.Join(errs, "; "))
}

func localToolCandidates(name string) []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return nil
	}
	bin := name + "_" + arch
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(exe); resolveErr == nil {
			exe = resolved
		}
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "binaries", bin),
			filepath.Join(dir, bin),
		)
	}
	return candidates
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func formatCommand(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, filepath.Base(bin))
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func runExternalCommand(ctx context.Context, timeout time.Duration, bin string, args ...string) (time.Duration, string, error) {
	child, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	out, err := exec.CommandContext(child, bin, args...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, string(out), fmt.Errorf("%s: %w: %s", filepath.Base(bin), err, out)
	}
	return elapsed, string(out), nil
}

func parseLastFloat(pattern, text string) (float64, bool) {
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if len(matches[i]) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(matches[i][1]), 64)
		if err == nil {
			return value, true
		}
	}
	return 0, false
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// --- External tool workload wrappers ---

// sysbenchWorkload wraps sysbench CPU benchmark.
type sysbenchWorkload struct {
	threads  int
	maxPrime int
	multi    bool
	events   float64
	command  string
}

func (w *sysbenchWorkload) Name() string {
	if w.multi {
		return "CPU Multi-Core (sysbench)"
	}
	return "CPU Single-Core (sysbench)"
}
func (w *sysbenchWorkload) Category() string { return "CPU" }
func (w *sysbenchWorkload) Description() string {
	return fmt.Sprintf("sysbench CPU --threads=%d --cpu-max-prime=%d", w.threads, w.maxPrime)
}
func (w *sysbenchWorkload) Validate() error  { return nil }
func (w *sysbenchWorkload) SkipWarmup() bool { return true }
func (w *sysbenchWorkload) Detail() string   { return w.command }
func (w *sysbenchWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.events, "events/sec"
}

func (w *sysbenchWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	bin, err := resolveTool("sysbench")
	if err != nil {
		return 0, 0, fmt.Errorf("sysbench: %w", err)
	}
	args := []string{
		"cpu",
		fmt.Sprintf("--threads=%d", w.threads),
		fmt.Sprintf("--cpu-max-prime=%d", w.maxPrime),
		"run",
	}
	w.command = formatCommand(bin, args)
	start := time.Now()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return 0, 0, fmt.Errorf("sysbench: %w: %s", err, out)
	}
	re := regexp.MustCompile(`events per second:\s+([\d.]+)`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) < 2 {
		return elapsed, 0, fmt.Errorf("sysbench: events/sec metric missing from output")
	}
	events, parseErr := strconv.ParseFloat(matches[1], 64)
	if parseErr != nil || events <= 0 {
		return elapsed, 0, fmt.Errorf("sysbench: invalid events/sec metric %q", matches[1])
	}
	w.events = events
	return elapsed, int64(events), nil
}

// sysbenchMemoryWorkload wraps sysbench memory bandwidth/latency benchmarks.
type sysbenchMemoryWorkload struct {
	threads         int
	operation       string
	accessMode      string
	blockSize       string
	totalSize       string
	durationSeconds int
	mibPerSec       float64
	operations      float64
	totalOperations int64
	averageNS       float64
	command         string
}

func (w *sysbenchMemoryWorkload) Name() string {
	if w.isLatencyProbe() {
		return "Memory Random Read Latency (sysbench)"
	}
	switch w.operation {
	case "write":
		return "Memory Write Bandwidth (sysbench)"
	default:
		return "Memory Read Bandwidth (sysbench)"
	}
}
func (w *sysbenchMemoryWorkload) Category() string { return bench.CategoryMemory }
func (w *sysbenchMemoryWorkload) Description() string {
	return fmt.Sprintf("sysbench memory %s/%s --threads=%d --memory-block-size=%s --memory-total-size=%s --time=%d",
		defaultString(w.operation, "read"),
		defaultString(w.accessMode, "seq"),
		w.threads,
		defaultString(w.blockSize, "1M"),
		defaultString(w.totalSize, "10G"),
		defaultPositive(w.durationSeconds, 3),
	)
}
func (w *sysbenchMemoryWorkload) Validate() error  { return nil }
func (w *sysbenchMemoryWorkload) SkipWarmup() bool { return true }
func (w *sysbenchMemoryWorkload) Detail() string   { return w.command }
func (w *sysbenchMemoryWorkload) Throughput(int64, time.Duration) (float64, string) {
	if w.isLatencyProbe() || (w.mibPerSec <= 0 && w.operations > 0) {
		return w.operations, "ops/s"
	}
	return w.mibPerSec, "MiB/s"
}
func (w *sysbenchMemoryWorkload) AverageLatencyNS(int64, time.Duration) float64 {
	return w.averageNS
}

func (w *sysbenchMemoryWorkload) ProcessedKind() bench.ProcessedKind {
	if w.isLatencyProbe() && w.totalOperations > 0 {
		return bench.ProcessedOperations
	}
	return bench.ProcessedUnknown
}

func (w *sysbenchMemoryWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	w.mibPerSec = 0
	w.operations = 0
	w.totalOperations = 0
	w.averageNS = 0

	bin, err := resolveTool("sysbench")
	if err != nil {
		return 0, 0, fmt.Errorf("sysbench: %w", err)
	}
	args := []string{
		"memory",
		fmt.Sprintf("--threads=%d", w.threads),
		"--memory-block-size=" + defaultString(w.blockSize, "1M"),
		"--memory-total-size=" + defaultString(w.totalSize, "10G"),
		"--memory-oper=" + defaultString(w.operation, "read"),
		"--memory-access-mode=" + defaultString(w.accessMode, "seq"),
		fmt.Sprintf("--time=%d", defaultPositive(w.durationSeconds, 3)),
		"run",
	}
	w.command = formatCommand(bin, args)
	start := time.Now()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return 0, 0, fmt.Errorf("sysbench memory: %w: %s", err, out)
	}
	reTransfer := regexp.MustCompile(`(?m)([\d.]+)\s+MiB transferred\s+\(([\d.]+)\s+MiB/sec\)`)
	if matches := reTransfer.FindStringSubmatch(string(out)); len(matches) >= 3 {
		w.mibPerSec, _ = strconv.ParseFloat(matches[2], 64)
	}
	reOps := regexp.MustCompile(`(?m)Total operations:\s+\d+\s+\(([\d.]+)\s+per second\)`)
	if matches := reOps.FindStringSubmatch(string(out)); len(matches) >= 2 {
		w.operations, _ = strconv.ParseFloat(matches[1], 64)
	}
	reTotalOps := regexp.MustCompile(`(?m)total number of events:\s+(\d+)`)
	if matches := reTotalOps.FindStringSubmatch(string(out)); len(matches) >= 2 {
		w.totalOperations, _ = strconv.ParseInt(matches[1], 10, 64)
	}
	reTotalTime := regexp.MustCompile(`(?m)total time:\s+([\d.]+)s`)
	if matches := reTotalTime.FindStringSubmatch(string(out)); len(matches) >= 2 && w.totalOperations > 0 {
		totalSeconds, _ := strconv.ParseFloat(matches[1], 64)
		if totalSeconds > 0 {
			w.averageNS = totalSeconds / float64(w.totalOperations) * 1e9
		}
	}
	if w.averageNS <= 0 {
		reAvgMS := regexp.MustCompile(`(?m)avg:\s+([\d.]+)`)
		if matches := reAvgMS.FindStringSubmatch(string(out)); len(matches) >= 2 {
			avgMS, _ := strconv.ParseFloat(matches[1], 64)
			w.averageNS = avgMS * 1e6
		}
	}
	switch {
	case w.isLatencyProbe() && w.totalOperations > 0:
		return elapsed, w.totalOperations, nil
	case w.isLatencyProbe() && w.operations > 0:
		return elapsed, int64(w.operations), nil
	case w.mibPerSec > 0:
		return elapsed, int64(w.mibPerSec), nil
	case w.operations > 0:
		return elapsed, int64(w.operations), nil
	default:
		return elapsed, 0, fmt.Errorf("sysbench memory: expected throughput or operation metric missing from output")
	}
}

func (w *sysbenchMemoryWorkload) isLatencyProbe() bool {
	return strings.EqualFold(w.accessMode, "rnd") || strings.EqualFold(w.blockSize, "64")
}

// opensslWorkload wraps openssl speed.
type opensslWorkload struct {
	algo    string
	seconds int
	bps     float64
	command string
}

func (w *opensslWorkload) Name() string        { return "OpenSSL " + strings.ToUpper(w.algo) }
func (w *opensslWorkload) Category() string    { return "CPU" }
func (w *opensslWorkload) Description() string { return "openssl speed " + w.algo }
func (w *opensslWorkload) Validate() error     { return nil }
func (w *opensslWorkload) SkipWarmup() bool    { return true }
func (w *opensslWorkload) Detail() string      { return w.command }
func (w *opensslWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.bps / (1024 * 1024), "MiB/s"
}

func (w *opensslWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, err := resolveTool("openssl")
	if err != nil {
		return 0, 0, fmt.Errorf("openssl: %w", err)
	}
	args := []string{"speed", "-mr"}
	if w.seconds > 0 {
		args = append(args, fmt.Sprintf("-seconds=%d", w.seconds))
	}
	args = append(args, w.algo)
	w.command = formatCommand(bin, args)

	start := time.Now()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return 0, 0, fmt.Errorf("openssl: %w: %s", err, out)
	}
	w.bps = parseOpenSSLMRBestBPS(string(out))
	if w.bps <= 0 {
		return elapsed, 0, fmt.Errorf("openssl: throughput metric missing from machine-readable output")
	}
	return elapsed, int64(w.bps), nil
}

func parseOpenSSLMRBestBPS(output string) float64 {
	var best float64
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+F:") {
			continue
		}
		parts := strings.Split(line, ":")
		for _, value := range parts[3:] {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil && parsed > 0 {
				best = parsed
			}
		}
	}
	return best
}

// fioWorkload wraps fio disk benchmark.
type fioWorkload struct {
	dir            string
	rw             string // randread, randwrite, read, or write
	bs             string // 1M or 4k
	size           string // 256M
	iodepth        int
	runtimeSeconds int
	bwBytes        int64
	iops           float64
	latencyNS      float64
	command        string
}

func (w *fioWorkload) Name() string {
	switch w.rw {
	case "randread":
		return fmt.Sprintf("Disk 4K Random Read Q%d (fio)", w.iodepth)
	case "randwrite":
		return fmt.Sprintf("Disk 4K Random Write Q%d (fio)", w.iodepth)
	case "write":
		return fmt.Sprintf("Disk 1M Sequential Write Q%d (fio)", w.iodepth)
	default:
		return fmt.Sprintf("Disk 1M Sequential Read Q%d (fio)", w.iodepth)
	}
}
func (w *fioWorkload) Category() string { return "Disk" }
func (w *fioWorkload) Description() string {
	return fmt.Sprintf("fio %s bs=%s iodepth=%d size=%s runtime=%ds",
		defaultString(w.rw, "read"),
		defaultString(w.bs, "1M"),
		defaultPositive(w.iodepth, 1),
		defaultString(w.size, "256M"),
		defaultPositive(w.runtimeSeconds, 3),
	)
}
func (w *fioWorkload) Validate() error  { return nil }
func (w *fioWorkload) SkipWarmup() bool { return true }
func (w *fioWorkload) Detail() string   { return w.command }
func (w *fioWorkload) Throughput(int64, time.Duration) (float64, string) {
	if w.isRandom() {
		return w.iops, "IOPS"
	}
	return float64(w.bwBytes) / (1024 * 1024), "MiB/s"
}
func (w *fioWorkload) AverageLatencyNS(int64, time.Duration) float64 {
	return w.latencyNS
}

func (w *fioWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	bin, err := resolveTool("fio")
	if err != nil {
		return 0, 0, fmt.Errorf("fio: %w", err)
	}
	runtimeSeconds := defaultPositive(w.runtimeSeconds, 3)
	dir := strings.TrimSpace(w.dir)
	if dir == "" {
		dir = os.TempDir()
	}

	filename := filepath.Join(dir, fmt.Sprintf(".vmbench-fio-%d-%d", os.Getpid(), time.Now().UnixNano()))
	defer os.Remove(filename)
	args := []string{
		"--name=vmbench", "--ioengine=" + fioIOEngine(),
		"--rw=" + defaultString(w.rw, "read"),
		"--bs=" + defaultString(w.bs, "1M"),
		"--size=" + defaultString(w.size, "256M"),
		fmt.Sprintf("--iodepth=%d", defaultPositive(w.iodepth, 1)),
		"--direct=1", "--output-format=json", "--unlink=1",
		"--group_reporting", "--eta=never",
		"--time_based", fmt.Sprintf("--runtime=%d", runtimeSeconds),
		"--filename=" + filename,
	}
	if w.isWrite() {
		args = append(args, "--overwrite=1")
	}
	w.command = formatCommand(bin, args)

	start := time.Now()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return 0, 0, fmt.Errorf("fio: %w: %s", err, out)
	}

	var result struct {
		Jobs []struct {
			Read struct {
				IOPS    float64 `json:"iops"`
				BWBytes int64   `json:"bw_bytes"`
				LatNS   struct {
					Mean float64 `json:"mean"`
				} `json:"lat_ns"`
				ClatNS struct {
					Mean float64 `json:"mean"`
				} `json:"clat_ns"`
			} `json:"read"`
			Write struct {
				IOPS    float64 `json:"iops"`
				BWBytes int64   `json:"bw_bytes"`
				LatNS   struct {
					Mean float64 `json:"mean"`
				} `json:"lat_ns"`
				ClatNS struct {
					Mean float64 `json:"mean"`
				} `json:"clat_ns"`
			} `json:"write"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return elapsed, 0, fmt.Errorf("fio: invalid JSON output: %w", err)
	}
	if len(result.Jobs) == 0 {
		return elapsed, 0, fmt.Errorf("fio: JSON output contains no jobs")
	}
	if w.isWrite() {
		w.iops = result.Jobs[0].Write.IOPS
		w.bwBytes = result.Jobs[0].Write.BWBytes
		w.latencyNS = firstPositiveFloat(result.Jobs[0].Write.LatNS.Mean, result.Jobs[0].Write.ClatNS.Mean)
	} else {
		w.iops = result.Jobs[0].Read.IOPS
		w.bwBytes = result.Jobs[0].Read.BWBytes
		w.latencyNS = firstPositiveFloat(result.Jobs[0].Read.LatNS.Mean, result.Jobs[0].Read.ClatNS.Mean)
	}

	metric := w.iops
	if !w.isRandom() {
		metric = float64(w.bwBytes)
	}
	if metric <= 0 {
		metricName := "bandwidth"
		if w.isRandom() {
			metricName = "IOPS"
		}
		return elapsed, 0, fmt.Errorf("fio: expected %s metric missing from JSON output", metricName)
	}
	return elapsed, int64(metric), nil
}

func fioIOEngine() string {
	switch runtime.GOOS {
	case "darwin":
		return "posixaio"
	case "windows":
		return "windowsaio"
	default:
		return "libaio"
	}
}

func (w *fioWorkload) isRandom() bool {
	return strings.HasPrefix(w.rw, "rand")
}

func (w *fioWorkload) isWrite() bool {
	return strings.Contains(w.rw, "write")
}

// ddWorkload wraps dd sequential disk throughput.
type ddWorkload struct {
	dir           string
	operation     string
	sizeMiB       int
	throughputMiB float64
	command       string
}

func (w *ddWorkload) Name() string {
	if w.operation == "read" {
		return "Disk Read (dd)"
	}
	return "Disk Write (dd)"
}
func (w *ddWorkload) Category() string { return "Disk" }
func (w *ddWorkload) Description() string {
	if w.operation == "read" {
		return fmt.Sprintf("dd direct-I/O sequential read, %d MiB", w.sizeMiB)
	}
	return fmt.Sprintf("dd sequential %s, %d MiB", w.operation, w.sizeMiB)
}
func (w *ddWorkload) Validate() error  { return nil }
func (w *ddWorkload) SkipWarmup() bool { return true }
func (w *ddWorkload) Detail() string   { return w.command }
func (w *ddWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.throughputMiB, "MiB/s"
}
func (*ddWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *ddWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, err := resolveTool("dd")
	if err != nil {
		return 0, 0, fmt.Errorf("dd: %w", err)
	}
	dir := strings.TrimSpace(w.dir)
	if dir == "" {
		dir = os.TempDir()
	}
	if w.sizeMiB <= 0 {
		w.sizeMiB = 256
	}
	tmp, err := os.CreateTemp(dir, "vmbench-dd-*")
	if err != nil {
		return 0, 0, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)

	blockSize := "1048576"
	count := strconv.Itoa(w.sizeMiB)
	var elapsed time.Duration
	var out string
	if w.operation == "read" {
		if runtime.GOOS != "linux" {
			return 0, 0, fmt.Errorf("dd read: uncached direct I/O is unsupported on %s; use fio instead", runtime.GOOS)
		}
		setupArgs := []string{"if=/dev/zero", "of=" + path, "bs=" + blockSize, "count=" + count, "conv=fsync"}
		if _, _, err := runExternalCommand(ctx, 180*time.Second, bin, setupArgs...); err != nil {
			return 0, 0, fmt.Errorf("dd setup: %w", err)
		}
		args := []string{"if=" + path, "of=/dev/null", "bs=" + blockSize, "iflag=direct"}
		w.command = formatCommand(bin, args)
		elapsed, out, err = runExternalCommand(ctx, 180*time.Second, bin, args...)
	} else {
		args := []string{"if=/dev/zero", "of=" + path, "bs=" + blockSize, "count=" + count, "conv=fsync"}
		w.command = formatCommand(bin, args)
		elapsed, out, err = runExternalCommand(ctx, 180*time.Second, bin, args...)
	}
	if err != nil {
		return 0, 0, err
	}
	if elapsed > 0 {
		w.throughputMiB = float64(w.sizeMiB) / elapsed.Seconds()
	}
	if parsed, ok := parseDDSpeedMiB(out); ok {
		w.throughputMiB = parsed
	}
	return elapsed, int64(w.sizeMiB) * 1024 * 1024, nil
}

func parseDDSpeedMiB(output string) (float64, bool) {
	re := regexp.MustCompile(`(?i),\s*([\d.]+)\s*([KMGT]?i?B|[KMGT]?B)/s`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	value, err := strconv.ParseFloat(last[1], 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToUpper(last[2]) {
	case "B":
		return value / (1024 * 1024), true
	case "KB":
		return value * 1000 / (1024 * 1024), true
	case "KIB":
		return value / 1024, true
	case "MB":
		return value * 1000 * 1000 / (1024 * 1024), true
	case "MIB":
		return value, true
	case "GB":
		return value * 1000 * 1000 * 1000 / (1024 * 1024), true
	case "GIB":
		return value * 1024, true
	case "TB":
		return value * 1000 * 1000 * 1000 * 1000 / (1024 * 1024), true
	case "TIB":
		return value * 1024 * 1024, true
	default:
		return value, true
	}
}

// streamWorkload wraps STREAM memory bandwidth.
type streamWorkload struct {
	mbPerSec float64
	command  string
}

func (w *streamWorkload) Name() string        { return "Memory Bandwidth (STREAM)" }
func (w *streamWorkload) Category() string    { return bench.CategoryMemory }
func (w *streamWorkload) Description() string { return "STREAM memory bandwidth best kernel" }
func (w *streamWorkload) Validate() error     { return nil }
func (w *streamWorkload) SkipWarmup() bool    { return true }
func (w *streamWorkload) Detail() string      { return w.command }
func (w *streamWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.mbPerSec, "MB/s"
}

func (w *streamWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, _, err := resolveAnyTool("stream", "stream_c")
	if err != nil {
		return 0, 0, fmt.Errorf("stream: %w", err)
	}
	w.command = formatCommand(bin, nil)
	elapsed, out, err := runExternalCommand(ctx, 180*time.Second, bin)
	if err != nil {
		return 0, 0, err
	}
	if value, ok := parseStreamBestRateMB(out); ok {
		w.mbPerSec = value
		return elapsed, int64(value), nil
	}
	return elapsed, 0, fmt.Errorf("stream: unable to parse bandwidth")
}

func parseStreamBestRateMB(output string) (float64, bool) {
	best := 0.0
	re := regexp.MustCompile(`(?im)^\s*(Copy|Scale|Add|Triad):\s*([\d.]+)`)
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		value, err := strconv.ParseFloat(m[2], 64)
		if err == nil && value > best {
			best = value
		}
	}
	return best, best > 0
}

// mbwWorkload wraps mbw memory bandwidth.
type mbwWorkload struct {
	sizeMiB   int
	mibPerSec float64
	command   string
}

func (w *mbwWorkload) Name() string     { return "Memory Bandwidth (mbw)" }
func (w *mbwWorkload) Category() string { return bench.CategoryMemory }
func (w *mbwWorkload) Description() string {
	return fmt.Sprintf("mbw memory copy bandwidth, %d MiB", w.sizeMiB)
}
func (w *mbwWorkload) Validate() error  { return nil }
func (w *mbwWorkload) SkipWarmup() bool { return true }
func (w *mbwWorkload) Detail() string   { return w.command }
func (w *mbwWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.mibPerSec, "MiB/s"
}

func (w *mbwWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, err := resolveTool("mbw")
	if err != nil {
		return 0, 0, fmt.Errorf("mbw: %w", err)
	}
	if w.sizeMiB <= 0 {
		w.sizeMiB = 256
	}
	args := []string{"-n", "5", strconv.Itoa(w.sizeMiB)}
	w.command = formatCommand(bin, args)
	elapsed, out, err := runExternalCommand(ctx, 180*time.Second, bin, args...)
	if err != nil {
		return 0, 0, err
	}
	if value, ok := parseMBWMiB(out); ok {
		w.mibPerSec = value
		return elapsed, int64(value), nil
	}
	return elapsed, 0, fmt.Errorf("mbw: unable to parse bandwidth")
}

func parseMBWMiB(output string) (float64, bool) {
	best := 0.0
	re := regexp.MustCompile(`(?i)([\d.]+)\s*MiB/s`)
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		value, err := strconv.ParseFloat(m[1], 64)
		if err == nil && value > best {
			best = value
		}
	}
	return best, best > 0
}

// geekbenchWorkload wraps optional Geekbench CPU benchmark.
type geekbenchWorkload struct {
	score   float64
	detail  string
	command string
}

func (w *geekbenchWorkload) Name() string        { return "Geekbench CPU" }
func (w *geekbenchWorkload) Category() string    { return "CPU" }
func (w *geekbenchWorkload) Description() string { return "Geekbench CPU benchmark, upstream score" }
func (w *geekbenchWorkload) Validate() error     { return nil }
func (w *geekbenchWorkload) SkipWarmup() bool    { return true }
func (w *geekbenchWorkload) Detail() string {
	if w.detail != "" {
		return w.detail + " | " + w.command
	}
	return w.command
}
func (w *geekbenchWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.score, "score"
}

func (w *geekbenchWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, name, err := resolveAnyTool("geekbench6", "geekbench5", "geekbench4", "geekbench")
	if err != nil {
		return 0, 0, fmt.Errorf("geekbench: %w", err)
	}
	args := geekbenchArgs(name)
	w.command = formatCommand(bin, args)
	elapsed, out, err := runExternalCommand(ctx, 45*time.Minute, bin, args...)
	if err != nil {
		return 0, 0, err
	}
	single, _ := parseLastFloat(`(?i)Single[- ]Core Score\s+([\d.]+)`, out)
	multi, ok := parseLastFloat(`(?i)Multi[- ]Core Score\s+([\d.]+)`, out)
	if !ok {
		if value, ok := parseLastFloat(`(?i)Geekbench Score\s+([\d.]+)`, out); ok {
			multi = value
			ok = true
		}
	}
	if !ok {
		return elapsed, 0, fmt.Errorf("geekbench: unable to parse CPU score")
	}
	w.score = multi
	if single > 0 {
		w.detail = fmt.Sprintf("single=%.0f multi=%.0f", single, multi)
	} else {
		w.detail = fmt.Sprintf("score=%.0f", multi)
	}
	return elapsed, int64(w.score), nil
}

func geekbenchArgs(name string) []string {
	if strings.Contains(name, "geekbench") {
		return []string{"--cpu", "--no-upload"}
	}
	return nil
}

// winsatWorkload wraps optional Windows WinSAT probes.
type winsatWorkload struct {
	kind    string
	dir     string
	value   float64
	command string
}

func (w *winsatWorkload) Name() string {
	switch w.kind {
	case "mem":
		return "WinSAT Memory"
	case "disk":
		return "WinSAT Disk"
	default:
		return "WinSAT CPU"
	}
}
func (w *winsatWorkload) Category() string {
	if w.kind == "mem" {
		return bench.CategoryMemory
	}
	if w.kind == "disk" {
		return "Disk"
	}
	return "CPU"
}
func (w *winsatWorkload) Description() string { return "winsat " + w.kind + " external benchmark" }
func (w *winsatWorkload) Validate() error     { return nil }
func (w *winsatWorkload) SkipWarmup() bool    { return true }
func (w *winsatWorkload) Detail() string      { return w.command }
func (w *winsatWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.value, "MB/s"
}

func (w *winsatWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	bin, err := resolveTool("winsat")
	if err != nil {
		return 0, 0, fmt.Errorf("winsat: %w", err)
	}
	args := w.winsatArgs()
	w.command = formatCommand(bin, args)
	elapsed, out, err := runExternalCommand(ctx, 10*time.Minute, bin, args...)
	if err != nil {
		return 0, 0, err
	}
	if value, ok := parseLastFloat(`(?i)([\d.]+)\s*MB/s`, out); ok {
		w.value = value
		return elapsed, int64(value), nil
	}
	if value, ok := parseLastFloat(`(?i)([\d.]+)\s*MBps`, out); ok {
		w.value = value
		return elapsed, int64(value), nil
	}
	return elapsed, 0, fmt.Errorf("winsat: unable to parse throughput")
}

func (w *winsatWorkload) winsatArgs() []string {
	switch w.kind {
	case "mem":
		return []string{"mem"}
	case "disk":
		drive := "c"
		if len(w.dir) >= 2 && w.dir[1] == ':' {
			drive = strings.ToLower(w.dir[:1])
		}
		return []string{"disk", "-seq", "-read", "-drive", drive}
	default:
		return []string{"cpu", "-compression"}
	}
}

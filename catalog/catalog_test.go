package catalog

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

func TestExternalHardwareDefinitionsDoNotRegisterNativeWorkloads(t *testing.T) {
	assertNoNativeHardwareDefinitions(t, ExternalHardwareDefinitions(""))
}

func TestDefaultDefinitionsAreHardwareOnly(t *testing.T) {
	for _, def := range DefaultDefinitions() {
		if def.Category == "Network" {
			t.Fatalf("hardware catalog contains network workload %q", def.Name)
		}
	}
}

func TestLocalToolCandidatesAreExecutableAdjacent(t *testing.T) {
	for _, candidate := range localToolCandidates("sysbench") {
		if !filepath.IsAbs(candidate) {
			t.Fatalf("local tool candidate must not depend on cwd: %q", candidate)
		}
	}
}

func TestLocalToolCandidatesPreferAdjacentOverUserCache(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	candidates := localToolCandidates("fio")
	if len(candidates) == 0 {
		t.Fatal("expected candidates on linux")
	}
	wantCache := filepath.Join(cache, "vmbench", "binaries", "fio_x64")
	if got := candidates[len(candidates)-1]; got != wantCache {
		t.Fatalf("last candidate = %q, want user cache %q", got, wantCache)
	}
	for _, candidate := range candidates[:len(candidates)-1] {
		if candidate == wantCache {
			t.Fatalf("user cache candidate %q must appear exactly once, candidates = %v", wantCache, candidates)
		}
		if strings.Contains(candidate, filepath.Join("vmbench", "binaries")) && !strings.HasPrefix(candidate, wantCache) {
			// adjacent "<exe-dir>/binaries/..." is allowed; only the user
			// cache path may reference the cache layout
			t.Fatalf("unexpected cache-layout candidate %q", candidate)
		}
	}
}

func TestResolveToolFindsUserCacheBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("adjacent/cache lookup is linux-only")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir := filepath.Join(cache, "vmbench", "binaries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mbw_x64")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	got, err := ResolveTool("mbw")
	if err != nil {
		t.Fatalf("ResolveTool(mbw) error = %v", err)
	}
	if got != path {
		t.Fatalf("ResolveTool(mbw) = %q, want %q", got, path)
	}
}

func TestDefaultExternalHardwareDefinitionsUseDetailedMemoryAndFioWorkloads(t *testing.T) {
	defs := ExternalHardwareDefinitionsForTools("", []string{HardwareToolSysbench, HardwareToolOpenSSL, HardwareToolFio})
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
	}
	for _, want := range []string{
		"Memory Read Bandwidth (sysbench)",
		"Memory Write Bandwidth (sysbench)",
		"Memory Random Read Latency (sysbench)",
		"Disk 4K Random Read Q1 (fio)",
		"Disk 4K Random Read Q32 (fio)",
		"Disk 4K Random Write Q1 (fio)",
		"Disk 4K Random Write Q32 (fio)",
		"Disk 1M Sequential Read Q1 (fio)",
		"Disk 1M Sequential Read Q8 (fio)",
		"Disk 1M Sequential Write Q1 (fio)",
		"Disk 1M Sequential Write Q8 (fio)",
	} {
		if !names[want] {
			t.Fatalf("definitions missing %q; got %v", want, names)
		}
	}
	for _, oldName := range []string{"Memory Bandwidth (sysbench)", "Disk Sequential (fio)", "Disk Random 4K (fio)"} {
		if names[oldName] {
			t.Fatalf("old coarse workload %q should not be registered by default", oldName)
		}
	}
}

func TestDefaultHardwareToolsByPlatform(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{goos: "linux", want: []string{HardwareToolSysbench, HardwareToolOpenSSL, HardwareToolFio}},
		{goos: "darwin", want: []string{HardwareToolOpenSSL}},
		{goos: "windows", want: []string{HardwareToolWinSAT}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got := defaultHardwareTools(tt.goos)
			if len(got) != len(tt.want) {
				t.Fatalf("defaultHardwareTools(%q) = %v, want %v", tt.goos, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("defaultHardwareTools(%q)[%d] = %q, want %q", tt.goos, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseOpenSSLMRBestBPS(t *testing.T) {
	output := `+H:16:64:256:1024:8192:16384
+F:6:sha256:92267894.95:293314873.47:697558367.68:1174145567.35:1443240080.81:1465597952.00`
	got := parseOpenSSLMRBestBPS(output)
	if got != 1465597952.00 {
		t.Fatalf("expected last +F throughput, got %f", got)
	}
}

func TestStandardizeHardwareTools(t *testing.T) {
	got := StandardizeHardwareTools([]string{"gb6", "fio", "all", "stream_c", "unknown"})
	want := HardwareToolIDs()
	if len(got) != len(want) {
		t.Fatalf("len(StandardizeHardwareTools) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("StandardizeHardwareTools[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestExternalHardwareDefinitionsForTools(t *testing.T) {
	defs := ExternalHardwareDefinitionsForTools("", []string{"dd", "stream", "mbw", "geekbench", "winsat"})
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
	}
	for _, want := range []string{"Disk Write (dd)", "Disk Read (dd)", "Memory Bandwidth (STREAM)", "Memory Bandwidth (mbw)", "Geekbench CPU", "WinSAT CPU", "WinSAT Memory", "WinSAT Disk"} {
		if !names[want] {
			t.Fatalf("definitions missing %q; got %v", want, names)
		}
	}
	if names["CPU Single-Core (sysbench)"] {
		t.Fatalf("sysbench should not be registered when only optional tools selected")
	}
}

func TestMissingHardwareToolsUsesSelectedAdapterCommands(t *testing.T) {
	dir := t.TempDir()
	// Isolate the toolbin cache so a dev machine that already fetched pinned
	// static binaries cannot satisfy tool resolution behind the test's back.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
		t.Setenv("PATHEXT", ".EXE")
	}
	for _, name := range []string{"sysbench", "openssl", "fio", "stream_c", "geekbench6"} {
		path := filepath.Join(dir, name+suffix)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	selected := []string{HardwareToolSysbench, HardwareToolOpenSSL, HardwareToolFio, HardwareToolStream, HardwareToolGeekbench}
	if got := MissingHardwareTools(selected); len(got) != 0 {
		t.Fatalf("MissingHardwareTools(%v) = %v, want none", selected, got)
	}
	if got := MissingHardwareTools([]string{HardwareToolMBW}); len(got) != 1 || got[0] != HardwareToolMBW {
		t.Fatalf("MissingHardwareTools(mbw) = %v, want [mbw]", got)
	}
}

func TestMissingHardwareToolsForFilterOnlyChecksMatchingAdapters(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	// Isolate the toolbin cache: resolveTool falls back to the user cache
	// directory, so a fetched pinned fio binary would otherwise mask the
	// missing-tool evidence this test asserts.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	tools := []string{HardwareToolFio, HardwareToolMBW}
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{name: "workload name", filter: `^Disk 4K Random`, want: HardwareToolFio},
		{name: "category", filter: `^Memory$`, want: HardwareToolMBW},
		{name: "no hardware match", filter: `^OpenSSL`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MissingHardwareToolsForFilter(tools, regexp.MustCompile(tt.filter))
			if strings.Join(got, ",") != tt.want {
				t.Fatalf("MissingHardwareToolsForFilter(%v, %q) = %v, want %q", tools, tt.filter, got, tt.want)
			}
		})
	}
}

func TestParseExternalToolOutputs(t *testing.T) {
	if got, ok := parseDDSpeedMiB("268435456 bytes copied, 0.123 s, 2.1 GB/s"); !ok || got < 1900 {
		t.Fatalf("parseDDSpeedMiB = %f,%v; want about 2000 MiB/s", got, ok)
	}
	if got, ok := parseStreamBestRateMB("Copy: 1234.5 0 0 0\nTriad: 3456.7 0 0 0"); !ok || got != 3456.7 {
		t.Fatalf("parseStreamBestRateMB = %f,%v", got, ok)
	}
	if got, ok := parseMBWMiB("AVG Method: MEMCPY Elapsed: 0.01 MiB: 256.0 Copy: 9876.5 MiB/s"); !ok || got != 9876.5 {
		t.Fatalf("parseMBWMiB = %f,%v", got, ok)
	}
	if got, ok := parseLastFloat(`(?i)Multi[- ]Core Score\s+([\d.]+)`, "Multi-Core Score 12345"); !ok || got != 12345 {
		t.Fatalf("parseLastFloat geekbench = %f,%v", got, ok)
	}
}

func TestDDReadRequiresDirectIOOnLinux(t *testing.T) {
	w := &ddWorkload{operation: "read", sizeMiB: 256}
	if !strings.Contains(w.Description(), "direct-I/O") {
		t.Fatalf("Description() = %q, want direct-I/O evidence", w.Description())
	}
	if runtime.GOOS != "linux" {
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "dd")
	script := "#!/bin/sh\ncase \" $* \" in\n  *\" iflag=direct \"*) printf '%s\\n' '268435456 bytes copied, 1 s, 256 MiB/s' ;;\n  *) printf '%s\\n' '268435456 bytes copied, 1 s, 256 MiB/s' ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if _, _, err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(w.Detail(), "iflag=direct") {
		t.Fatalf("Detail() = %q, want direct-I/O argument", w.Detail())
	}
}

func TestMalformedExternalToolOutputFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	assertMalformedToolFails(t, "sysbench", "not sysbench metrics", func() error {
		_, _, err := (&sysbenchWorkload{threads: 1, maxPrime: 20000}).Run(context.Background())
		return err
	})
	assertMalformedToolFails(t, "sysbench", "not sysbench memory metrics", func() error {
		workload := &sysbenchMemoryWorkload{
			accessMode:      "rnd",
			mibPerSec:       100,
			operations:      200,
			totalOperations: 300,
			averageNS:       400,
		}
		_, _, err := workload.Run(context.Background())
		return err
	})
	assertMalformedToolFails(t, "openssl", "not openssl metrics", func() error {
		_, _, err := (&opensslWorkload{algo: "sha256", seconds: 1}).Run(context.Background())
		return err
	})
	assertMalformedToolFails(t, "fio", "not json", func() error {
		_, _, err := (&fioWorkload{rw: "read", bs: "1M", size: "1M", iodepth: 1, runtimeSeconds: 1}).Run(context.Background())
		return err
	})
}

func TestExternalWorkloadProcessedMetricSemantics(t *testing.T) {
	if got := processedKind(&ddWorkload{}); got != bench.ProcessedBytes {
		t.Fatalf("dd processed kind = %v, want bytes", got)
	}
	latency := &sysbenchMemoryWorkload{accessMode: "rnd", totalOperations: 42}
	if got := processedKind(latency); got != bench.ProcessedOperations {
		t.Fatalf("sysbench latency processed kind = %v, want operations", got)
	}
	latency.totalOperations = 0
	latency.operations = 1000
	if got := processedKind(latency); got != bench.ProcessedUnknown {
		t.Fatalf("sysbench operations/sec fallback processed kind = %v, want unknown", got)
	}

	rateWorkloads := []bench.Workload{
		&sysbenchWorkload{},
		&opensslWorkload{},
		&fioWorkload{},
		&streamWorkload{},
		&mbwWorkload{},
		&geekbenchWorkload{},
		&winsatWorkload{},
	}
	for _, workload := range rateWorkloads {
		if got := processedKind(workload); got != bench.ProcessedUnknown {
			t.Errorf("%T processed kind = %v, want unknown", workload, got)
		}
	}
}

func processedKind(workload bench.Workload) bench.ProcessedKind {
	reporter, ok := workload.(bench.ProcessedMetricReporter)
	if !ok {
		return bench.ProcessedUnknown
	}
	return reporter.ProcessedKind()
}

func assertMalformedToolFails(t *testing.T, name, output string, run func() error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nprintf '%s\\n' '" + strings.ReplaceAll(output, "'", "'\\''") + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+originalPath)
	if err := run(); err == nil {
		t.Fatalf("%s malformed output returned success", name)
	}
}

func assertNoNativeHardwareDefinitions(t *testing.T, defs []Definition) {
	t.Helper()
	forbidden := map[string]bool{
		"AES-256-GCM":     true,
		"SHA-256":         true,
		"SHA-512":         true,
		"LZ4 Compress":    true,
		"Zstd Compress":   true,
		"Sort":            true,
		"Regex":           true,
		"Dijkstra":        true,
		"FFT":             true,
		"N-Body":          true,
		"Ray Trace":       true,
		"MatMul":          true,
		"Mandelbrot":      true,
		"Mem Bandwidth":   true,
		"Mem Latency":     true,
		"Disk Sequential": true,
		"Disk Random 4K":  true,
	}
	for _, def := range defs {
		if forbidden[def.Name] {
			t.Fatalf("registered native hardware workload %q", def.Name)
		}
	}
}

func writeFakeFio(t *testing.T, json string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fio")
	script := "#!/bin/sh\nprintf '%s' '" + json + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestFioWorkloadParsesLatencyP99(t *testing.T) {
	writeFakeFio(t, `{"jobs":[{"read":{"iops":91234.5,"bw_bytes":1048576,"lat_ns":{"mean":150000},"clat_ns":{"mean":149000,"percentile":{"50.000000":140000,"99.000000":342016}}},"write":{"iops":12,"bw_bytes":4096,"lat_ns":{"mean":9000},"clat_ns":{"mean":8800,"percentile":{"99.000000":777}}}}]}`)
	read := &fioWorkload{rw: "randread", bs: "4k", size: "1M", iodepth: 1, runtimeSeconds: 1}
	if _, _, err := read.Run(context.Background()); err != nil {
		t.Fatalf("read Run() error = %v", err)
	}
	if got := read.LatencyP99NS(); got != 342016 {
		t.Fatalf("read LatencyP99NS() = %f, want 342016", got)
	}
	write := &fioWorkload{rw: "randwrite", bs: "4k", size: "1M", iodepth: 1, runtimeSeconds: 1}
	if _, _, err := write.Run(context.Background()); err != nil {
		t.Fatalf("write Run() error = %v", err)
	}
	if got := write.LatencyP99NS(); got != 777 {
		t.Fatalf("write LatencyP99NS() = %f, want 777", got)
	}
}

func TestFioWorkloadWithoutPercentileLeavesP99Zero(t *testing.T) {
	writeFakeFio(t, `{"jobs":[{"read":{"iops":10,"bw_bytes":4096,"lat_ns":{"mean":150000},"clat_ns":{"mean":149000}}}]}`)
	workload := &fioWorkload{rw: "randread", bs: "4k", size: "1M", iodepth: 1, runtimeSeconds: 1}
	if _, _, err := workload.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := workload.LatencyP99NS(); got != 0 {
		t.Fatalf("LatencyP99NS() = %f, want 0", got)
	}
}

func TestCPUStealWorkloadComputesStealPercent(t *testing.T) {
	first := []uint64{100, 0, 50, 300, 0, 0, 10, 4, 0, 0}
	second := []uint64{200, 0, 150, 400, 0, 0, 10, 104, 0, 0}
	calls := 0
	workload := &cpuStealWorkload{interval: time.Millisecond}
	workload.readStat = func() ([]uint64, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}
	elapsed, _, err := workload.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if elapsed <= 0 {
		t.Fatalf("elapsed = %v, want positive", elapsed)
	}
	// total delta = 100+100+100+100 = 400, steal delta = 100 -> 25%
	if got := workload.stealPercent; got != 25 {
		t.Fatalf("stealPercent = %f, want 25", got)
	}
	throughput, unit := workload.Throughput(0, elapsed)
	if throughput != 25 || unit != "%" {
		t.Fatalf("Throughput() = %f %q, want 25 %%", throughput, unit)
	}
	if !strings.Contains(workload.Detail(), "steal=25.00%") {
		t.Fatalf("Detail() = %q, want steal percentage", workload.Detail())
	}
	if workload.MaxIterations() != 1 || !workload.SkipWarmup() {
		t.Fatalf("steal probe must run once without warmup")
	}
	if workload.Name() != "CPU Steal (/proc/stat)" || workload.Category() != "CPU" {
		t.Fatalf("unexpected identity %q / %q", workload.Name(), workload.Category())
	}
}

func TestCPUStealWorkloadFailsWhenCountersDoNotAdvance(t *testing.T) {
	static := []uint64{100, 0, 50, 300, 0, 0, 10, 4}
	workload := &cpuStealWorkload{interval: time.Millisecond, readStat: func() ([]uint64, error) {
		return static, nil
	}}
	_, _, err := workload.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("Run() error = %v, want counters-did-not-advance", err)
	}
	if err := workload.Validate(); err == nil {
		t.Fatal("Validate() must fail without samples")
	}
}

func TestCPUStealWorkloadFailsWhenCountersMoveBackwards(t *testing.T) {
	first := []uint64{100, 0, 50, 300, 0, 0, 10, 4}
	backwards := []uint64{50, 0, 50, 300, 0, 0, 10, 4}
	calls := 0
	workload := &cpuStealWorkload{interval: time.Millisecond}
	workload.readStat = func() ([]uint64, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return backwards, nil
	}
	if _, _, err := workload.Run(context.Background()); err == nil {
		t.Fatal("Run() must fail when counters move backwards")
	}
}

func TestStealProbeRegisteredPerPlatform(t *testing.T) {
	defs := DefaultDefinitions()
	found := false
	for _, def := range defs {
		if def.Name == "CPU Steal (/proc/stat)" {
			found = true
		}
	}
	if runtime.GOOS == "linux" && !found {
		t.Fatal("steal probe missing on linux")
	}
	if runtime.GOOS != "linux" && found {
		t.Fatal("steal probe registered outside linux")
	}
}

func TestReadProcStatCPULinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc/stat is Linux-only")
	}
	values, err := readProcStatCPU()
	if err != nil {
		t.Fatalf("readProcStatCPU() error = %v", err)
	}
	if len(values) < 8 {
		t.Fatalf("readProcStatCPU() = %v, want at least 8 counters", values)
	}
}

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/bench"
)

func TestExternalHardwareDefinitionsDoNotRegisterNativeWorkloads(t *testing.T) {
	assertNoNativeHardwareDefinitions(t, ExternalHardwareDefinitions(""))
}

func TestLegacyNativeEngineDoesNotRegisterNativeHardware(t *testing.T) {
	assertNoNativeHardwareDefinitions(t, DefinitionsForScope("native", ScopeHardware, "", nil))
	assertNoNativeHardwareDefinitions(t, DefinitionsForScope("full", ScopeHardware, "", nil))
}

func TestScopeSelectionIsExplicit(t *testing.T) {
	for _, def := range DefinitionsForScopeWithHardwareTools("external", ScopeNetwork, "", nil, nil) {
		if def.Category != "Network" {
			t.Fatalf("network scope contains %q category %q", def.Name, def.Category)
		}
	}
	for _, def := range DefinitionsForScopeWithHardwareTools("external", "unknown", "", nil, nil) {
		if def.Category == "Network" {
			t.Fatalf("unknown scope must default to hardware, got %q", def.Name)
		}
	}
	for _, def := range DefaultDefinitions(false) {
		if def.Category == "Network" {
			t.Fatalf("extensions=false contains network workload %q", def.Name)
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

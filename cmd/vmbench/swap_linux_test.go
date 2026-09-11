//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/catalog"
)

func TestNeedsTempSwapThresholds(t *testing.T) {
	const (
		gib  = uint64(1) << 30
		mib  = uint64(1) << 20
		half = tempSwapTargetBytes / 2
	)
	tests := []struct {
		name      string
		mem, swap uint64
		need      bool
		size      uint64
	}{
		{"small ram no swap", 512 * mib, 0, true, tempSwapTargetBytes - 512*mib},
		{"small ram enough swap", 512 * mib, gib, false, 0},
		{"small ram small swap", 512 * mib, 256 * mib, true, tempSwapTargetBytes - 768*mib},
		{"ample ram no swap", gib, 0, false, 0},
		{"ample ram any swap", 2 * gib, 0, false, 0},
		{"tiny deficit clamps to minimum", 1023 * mib, 473 * mib, true, tempSwapMinBytes},
		{"unused half", half, 0, true, half},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsTempSwap(tt.mem, tt.swap); got != tt.need {
				t.Fatalf("needsTempSwap(%d, %d) = %v, want %v", tt.mem, tt.swap, got, tt.need)
			}
			if tt.size == 0 {
				return
			}
			if got := tempSwapSize(tt.mem, tt.swap); got != tt.size {
				t.Fatalf("tempSwapSize(%d, %d) = %d, want %d", tt.mem, tt.swap, got, tt.size)
			}
		})
	}
}

// swapRecorder fakes every swapRuntime effect and records the commands and
// swapfile paths seen along the way.
type swapRecorder struct {
	commands []string
	paths    []string
	failAt   string // command name (e.g. "swapon") that returns an error
	env      swapRuntime
}

func newSwapRecorder(mem, swap uint64, euid int, terminal bool) *swapRecorder {
	rec := &swapRecorder{}
	rec.env = swapRuntime{
		runCommand: func(name string, args ...string) error {
			if rec.failAt == name {
				return errors.New(name + " failed")
			}
			rec.commands = append(rec.commands, name)
			if len(args) > 0 {
				rec.paths = append(rec.paths, args[len(args)-1])
			}
			return nil
		},
		statfs: func(dir string) (uint64, uint64, error) {
			return 0xEF53 /* ext4 magic */, 16 << 30, nil
		},
		geteuid:    func() int { return euid },
		meminfo:    func() (uint64, uint64, error) { return mem, swap, nil },
		isTerminal: func() bool { return terminal },
		zeroFill: func(path string, size uint64) error {
			return os.WriteFile(path, nil, 0o600)
		},
	}
	return rec
}

func swapPath(rec *swapRecorder) string {
	if len(rec.paths) == 0 {
		return ""
	}
	return rec.paths[0]
}

func TestMaybeSetupTempSwapSkipsUnrelatedRuns(t *testing.T) {
	cases := []struct {
		name   string
		tools  []string
		filter *regexp.Regexp
		mem    uint64
	}{
		{"geekbench not selected", nil, nil, 512 << 20},
		{"geekbench filtered out", []string{catalog.HardwareToolGeekbench}, regexp.MustCompile(`^Memory$`), 512 << 20},
		{"enough memory", []string{catalog.HardwareToolGeekbench}, nil, 2 << 30},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			rec := newSwapRecorder(tt.mem, 0, 0, true)
			teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, tt.tools, tt.filter, rec.env, &out, strings.NewReader("y\n"))
			if out.Len() != 0 {
				t.Fatalf("expected no output, got:\n%s", out.String())
			}
			if len(rec.commands) != 0 {
				t.Fatalf("expected no swap commands, got %v", rec.commands)
			}
			teardown()
		})
	}
}

func TestMaybeSetupTempSwapNonInteractiveOnlyWarns(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, false)
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	if !strings.Contains(out.String(), "geekbench needs") {
		t.Fatalf("expected the low-memory warning, got:\n%s", out.String())
	}
	if len(rec.commands) != 0 {
		t.Fatalf("non-terminal runs must not create swap, got %v", rec.commands)
	}
	teardown()
}

func TestMaybeSetupTempSwapPromptDeclined(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, true)
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("n\n"))
	if !strings.Contains(out.String(), "temporary swapfile") {
		t.Fatalf("expected the prompt, got:\n%s", out.String())
	}
	if len(rec.commands) != 0 {
		t.Fatalf("a declined prompt must not create swap, got %v", rec.commands)
	}
	teardown()
}

func TestMaybeSetupTempSwapNonRootSkipsCreation(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 1000, true)
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	if !strings.Contains(out.String(), "requires root") {
		t.Fatalf("expected the root warning, got:\n%s", out.String())
	}
	if len(rec.commands) != 0 {
		t.Fatalf("non-root runs must not create swap, got %v", rec.commands)
	}
	teardown()
}

func TestTempSwapCreateAndTeardownSequence(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, true)
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	if !strings.Contains(out.String(), "temporary swap enabled") {
		t.Fatalf("expected the enabled notice, got:\n%s", out.String())
	}
	if strings.Join(rec.commands, ",") != "mkswap,swapon" {
		t.Fatalf("setup commands = %v, want [mkswap swapon]", rec.commands)
	}
	path := swapPath(rec)
	if path == "" {
		t.Fatal("swapfile path was not recorded")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("swapfile should exist while active: %v", err)
	}
	teardown()
	if strings.Join(rec.commands, ",") != "mkswap,swapon,swapoff" {
		t.Fatalf("commands after teardown = %v, want swapoff last", rec.commands)
	}
	if !strings.Contains(out.String(), "temporary swap removed") {
		t.Fatalf("expected the removed notice, got:\n%s", out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("swapfile must be deleted after teardown, stat err = %v", err)
	}
}

func TestTempSwapTeardownKeepsFileWhenSwapoffFails(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, true)
	rec.failAt = "swapoff"
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	path := swapPath(rec)
	if path == "" {
		t.Fatal("swapfile path was not recorded")
	}
	teardown()
	if !strings.Contains(out.String(), "swapoff") || !strings.Contains(out.String(), path) {
		t.Fatalf("expected teardown warning naming %q, got:\n%s", path, out.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("swapfile must be kept when swapoff fails: %v", err)
	}
	os.Remove(path)
}

func TestTempSwapRejectsTmpfsLocations(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, true)
	rec.env.statfs = func(dir string) (uint64, uint64, error) {
		return linuxTmpfsMagic, 16 << 30, nil
	}
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	if !strings.Contains(out.String(), "no suitable non-tmpfs location") {
		t.Fatalf("expected the location warning, got:\n%s", out.String())
	}
	if len(rec.commands) != 0 {
		t.Fatalf("tmpfs-only hosts must not create swap, got %v", rec.commands)
	}
	teardown()
}

func TestTempSwapSetupFailureCleansUp(t *testing.T) {
	rec := newSwapRecorder(512<<20, 0, 0, true)
	rec.failAt = "mkswap"
	var out strings.Builder
	teardown := maybeSetupTempSwapWith(&benchmarkFlags{autoSwap: true}, []string{catalog.HardwareToolGeekbench}, nil, rec.env, &out, strings.NewReader("y\n"))
	if !strings.Contains(out.String(), "setup failed") {
		t.Fatalf("expected the setup failure warning, got:\n%s", out.String())
	}
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), ".vmbench-swap-*"))
	if len(matches) != 0 {
		t.Fatalf("failed setup must remove the partial swapfile, found %v", matches)
	}
	teardown()
}

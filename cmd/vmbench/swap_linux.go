//go:build linux

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// Geekbench refuses or OOMs on hosts with little RAM and little combined
// memory. The thresholds mirror the widely used swap workaround: offer a
// temporary swapfile when RAM < 1 GiB and RAM+swap < 1.5 GiB.
const (
	tempSwapTargetBytes  = uint64(3) << 29 // 1.5 GiB combined target
	tempSwapMinBytes     = uint64(64) << 20
	tempSwapMaxBytes     = uint64(2) << 30
	tempSwapSpaceMargin  = uint64(256) << 20
	tempSwapWriteChunk   = 4 << 20
	linuxTmpfsMagic      = 0x01021994
	tempSwapFileTemplate = ".vmbench-swap-%d"
)

// swapRuntime carries the effects a swap setup depends on, injectable for
// tests (the ping ICMP fallback uses the same pattern).
type swapRuntime struct {
	runCommand func(name string, args ...string) error
	statfs     func(dir string) (fsType, availBytes uint64, err error)
	geteuid    func() int
	meminfo    func() (memTotal, swapTotal uint64, err error)
	zeroFill   func(path string, size uint64) error
	isTerminal func() bool
}

func defaultSwapRuntime() swapRuntime {
	return swapRuntime{
		runCommand: func(name string, args ...string) error {
			return runSwapCommand(name, args...)
		},
		statfs:     statfsForSwap,
		geteuid:    syscall.Geteuid,
		meminfo:    sysinfo.MeminfoTotals,
		zeroFill:   zeroFillSwapfile,
		isTerminal: stdinIsTerminal,
	}
}

func runSwapCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}

func statfsForSwap(dir string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	return uint64(st.Type), uint64(st.Bsize) * uint64(st.Bavail), nil
}

// maybeSetupTempSwap checks whether geekbench will run on a low-memory host
// and, with the user's consent (--auto-swap, a terminal prompt, or nothing on
// a non-terminal stdin), creates a temporary swapfile sized to close the gap
// to tempSwapTargetBytes. It returns a teardown function that must run when
// the benchmark finishes; setup failures only warn and never block the run.
func maybeSetupTempSwap(bf *benchmarkFlags, tools []string, filter *regexp.Regexp) func() {
	return maybeSetupTempSwapWith(bf, tools, filter, defaultSwapRuntime(), os.Stdout, os.Stdin)
}

func maybeSetupTempSwapWith(bf *benchmarkFlags, tools []string, filter *regexp.Regexp, env swapRuntime, out io.Writer, in io.Reader) func() {
	noop := func() {}
	if !catalog.HardwareToolActiveForFilter(tools, catalog.HardwareToolGeekbench, filter) {
		return noop
	}
	memTotal, swapTotal, err := env.meminfo()
	if err != nil {
		return noop
	}
	if !needsTempSwap(memTotal, swapTotal) {
		return noop
	}
	fmt.Fprintf(out, "%s\n", i18n.Tf("cli.notice.lowMemoryGeekbench", map[string]any{
		"Mem":  formatSwapBytes(memTotal),
		"Swap": formatSwapBytes(swapTotal),
	}))
	if !bf.autoSwap {
		if !env.isTerminal() {
			return noop
		}
		if !confirmPrompt(out, in, "cli.prompt.tempSwap") {
			return noop
		}
	}
	if env.geteuid() != 0 {
		fmt.Fprintf(out, "%s\n", i18n.T("cli.notice.swapNeedsRoot"))
		return noop
	}
	path, teardown, ok := createTempSwap(env, memTotal, swapTotal, out)
	if !ok {
		return noop
	}
	fmt.Fprintf(out, "%s\n", i18n.Tf("cli.notice.swapEnabled", map[string]any{
		"Path": path,
		"Size": formatSwapBytes(tempSwapSize(memTotal, swapTotal)),
	}))
	return teardown
}

// createTempSwap writes a zero-filled swapfile, activates it, and returns the
// idempotent teardown. The zero-filled write is deliberate: swapfiles must
// not be sparse, and doing it in-process avoids dd/coreutils variance.
func createTempSwap(env swapRuntime, memTotal, swapTotal uint64, out io.Writer) (string, func(), bool) {
	dir := pickSwapDir(env)
	if dir == "" {
		fmt.Fprintf(out, "%s\n", i18n.T("cli.error.swapNoLocation"))
		return "", nil, false
	}
	size := tempSwapSize(memTotal, swapTotal)
	if fsType, avail, err := env.statfs(dir); err != nil || fsType == linuxTmpfsMagic || avail < size+tempSwapSpaceMargin {
		fmt.Fprintf(out, "%s\n", i18n.T("cli.error.swapNoLocation"))
		return "", nil, false
	}
	path := filepath.Join(dir, fmt.Sprintf(tempSwapFileTemplate, os.Getpid()))
	if err := env.zeroFill(path, size); err != nil {
		os.Remove(path)
		warnSwapSetup(out, err)
		return "", nil, false
	}
	if err := env.runCommand("mkswap", path); err != nil {
		os.Remove(path)
		warnSwapSetup(out, err)
		return "", nil, false
	}
	if err := env.runCommand("swapon", path); err != nil {
		os.Remove(path)
		warnSwapSetup(out, err)
		return "", nil, false
	}

	// While the swapfile is active the usual die-on-signal behavior would
	// leak it, so signals are intercepted for the swap's lifetime only and
	// routed through the same idempotent teardown as normal completion.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var once sync.Once
	teardown := func() {
		once.Do(func() {
			signal.Stop(signals)
			close(signals)
			if err := env.runCommand("swapoff", path); err != nil {
				fmt.Fprintf(out, "%s\n", i18n.Tf("cli.error.swapTeardown", map[string]any{
					"Path": path,
					"Err":  err.Error(),
				}))
				return
			}
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(out, "%s\n", i18n.Tf("cli.error.swapTeardown", map[string]any{
					"Path": path,
					"Err":  err.Error(),
				}))
				return
			}
			fmt.Fprintf(out, "%s\n", i18n.T("cli.notice.swapRemoved"))
		})
	}
	go func() {
		if sig, ok := <-signals; ok {
			teardown()
			if sig == syscall.SIGTERM {
				os.Exit(143)
			}
			os.Exit(130)
		}
	}()
	return path, teardown, true
}

// pickSwapDir prefers the temp directory when it is backed by a real
// filesystem, and falls back to the working directory otherwise (/tmp is
// commonly tmpfs, and a swapfile on tmpfs costs the RAM it is meant to add).
func pickSwapDir(env swapRuntime) string {
	for _, dir := range []string{os.TempDir(), "."} {
		if dir == "" {
			continue
		}
		if fsType, _, err := env.statfs(dir); err == nil && fsType != linuxTmpfsMagic {
			return dir
		}
	}
	return ""
}

// writeZeros fills the file with real zero bytes so the swapfile has no
// holes; swapon rejects sparse files.
func writeZeros(w io.Writer, size uint64) error {
	chunk := make([]byte, tempSwapWriteChunk)
	var written uint64
	for written < size {
		n := uint64(len(chunk))
		if remaining := size - written; remaining < n {
			n = remaining
		}
		if _, err := w.Write(chunk[:n]); err != nil {
			return err
		}
		written += n
	}
	return nil
}

// zeroFillSwapfile creates the swapfile and fills it with non-sparse zeros.
func zeroFillSwapfile(path string, size uint64) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := writeZeros(file, size); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// needsTempSwap reports whether the host is small enough that geekbench is
// expected to fail without extra swap.
func needsTempSwap(memTotal, swapTotal uint64) bool {
	return memTotal < 1<<30 && memTotal+swapTotal < tempSwapTargetBytes
}

// tempSwapSize returns the swapfile size that lifts combined memory to the
// target, clamped to sane bounds and rounded up to whole MiB.
func tempSwapSize(memTotal, swapTotal uint64) uint64 {
	deficit := int64(tempSwapTargetBytes) - int64(memTotal+swapTotal)
	if deficit <= 0 {
		return tempSwapMinBytes
	}
	const mib = uint64(1) << 20
	size := (uint64(deficit) + mib - 1) / mib * mib
	if size < tempSwapMinBytes {
		size = tempSwapMinBytes
	}
	if size > tempSwapMaxBytes {
		size = tempSwapMaxBytes
	}
	return size
}

func warnSwapSetup(out io.Writer, err error) {
	fmt.Fprintf(out, "%s\n", i18n.Tf("cli.error.swapSetup", map[string]any{"Err": err.Error()}))
}

func formatSwapBytes(value uint64) string {
	const mib = uint64(1) << 20
	if value >= mib {
		return fmt.Sprintf("%d MiB", (value+mib-1)/mib)
	}
	return fmt.Sprintf("%d KiB", (value+1023)/1024)
}

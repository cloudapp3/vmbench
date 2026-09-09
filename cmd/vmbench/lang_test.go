package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/i18n"
)

// Locale golden tests: the CLI surface must render coherently in every
// supported language. run() resolves the language from VMBENCH_LANG/config/OS
// on every invocation, so these tests drive the selection through the env the
// same way a real shell session would. printUsage/printRootHelp-based cases
// set the active language directly (package-global state; never run in
// parallel).

func withLang(t *testing.T, lang string) {
	t.Helper()
	if !i18n.SetLang(lang) {
		t.Fatalf("SetLang(%q) failed", lang)
	}
	t.Cleanup(func() { i18n.SetLang("en") })
}

func withLangEnv(t *testing.T, lang string) {
	t.Helper()
	if lang == "" {
		t.Setenv("VMBENCH_LANG", "")
		return
	}
	t.Setenv("VMBENCH_LANG", lang)
}

func captureStderr(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	original := os.Stderr
	file, err := os.CreateTemp(t.TempDir(), "stderr-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.Stderr = original
		_ = file.Close()
	}()
	os.Stderr = file
	code := fn()
	os.Stderr = original
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(output), code
}

func TestUsageLocalized(t *testing.T) {
	cases := []struct {
		lang string
		want []string
	}{
		{"en", []string{"cross-platform CPU and network benchmark toolkit", "Usage:", "run benchmarks", "self-update to the latest GitHub release", "show version"}},
		{"zh-CN", []string{"跨平台 CPU 与网络基准测试工具集", "用法:", "运行基准测试", "自升级到最新 GitHub 发布版本", "显示版本"}},
	}
	for _, c := range cases {
		withLang(t, c.lang)
		var buf bytes.Buffer
		printUsage(&buf)
		for _, want := range c.want {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("printUsage [%s] missing %q:\n%s", c.lang, want, buf.String())
			}
		}
	}
}

func TestRootHelpLocalized(t *testing.T) {
	cases := []struct {
		lang string
		want []string
	}{
		{"en", []string{"vmbench [flags]", "hardware benchmark only", "Presets:", "Speed providers:", "Hardware tools:", "Flags:", "iterations per workload (1-9)", "YABS-like quick run"}},
		{"zh-CN", []string{"vmbench [flags]", "仅运行硬件基准", "预设:", "测速提供方:", "硬件测试工具:", "参数:", "每个工作负载的迭代次数 (1-9)", "类 YABS 快速测试"}},
	}
	for _, c := range cases {
		withLang(t, c.lang)
		var buf bytes.Buffer
		printRootHelp(&buf)
		for _, want := range c.want {
			if !strings.Contains(buf.String(), want) {
				t.Errorf("printRootHelp [%s] missing %q:\n%s", c.lang, want, buf.String())
			}
		}
	}
}

func TestValidationErrorLocalized(t *testing.T) {
	cases := []struct {
		lang string
		want string
	}{
		{"", "error: --iterations must be between 1 and 9"},
		{"zh-CN", "错误: --iterations 必须在 1 到 9 之间"},
	}
	for _, c := range cases {
		withLangEnv(t, c.lang)
		output, code := captureStderr(t, func() int { return run([]string{"--iterations", "0"}) })
		if code != 2 {
			t.Fatalf("--iterations 0 [%s] exit = %d", c.lang, code)
		}
		if !strings.Contains(output, c.want) {
			t.Errorf("[%s] stderr missing %q:\n%s", c.lang, c.want, output)
		}
	}
}

func TestLangFlagOverridesEnv(t *testing.T) {
	withLangEnv(t, "en")
	output, code := captureStderr(t, func() int { return run([]string{"--lang", "zh-CN", "--iterations", "0"}) })
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(output, "错误: --iterations 必须在 1 到 9 之间") {
		t.Errorf("--lang zh-CN did not override VMBENCH_LANG=en:\n%s", output)
	}
}

func TestMergedCommandErrorLocalized(t *testing.T) {
	cases := []struct {
		lang string
		want string
	}{
		{"", `was removed in v0.8.0`},
		{"zh-CN", "已在 v0.8.0 移除"},
	}
	for _, c := range cases {
		withLangEnv(t, c.lang)
		output, code := captureStderr(t, func() int { return run([]string{"run"}) })
		if code != 2 {
			t.Fatalf("run [%s] exit = %d, want 2", c.lang, code)
		}
		if !strings.Contains(output, c.want) {
			t.Errorf("[%s] stderr missing %q:\n%s", c.lang, c.want, output)
		}
	}
}

package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// Catalog files decode into a nested map; key presence is what matters.
func decodeTOML(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	return m
}

func flatten(prefix string, m map[string]any, out map[string]bool) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			flatten(key, nested, out)
			continue
		}
		out[key] = true
	}
}

func catalogKeys(t *testing.T) map[string]map[string]bool {
	t.Helper()
	locales := Supported()
	keys := make(map[string]map[string]bool, len(locales))
	for _, lang := range locales {
		entries, err := os.ReadDir(filepath.Join("messages", lang))
		if err != nil {
			t.Fatalf("read messages/%s: %v", lang, err)
		}
		seen := make(map[string]bool)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join("messages", lang, e.Name()))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			flatten("", decodeTOML(t, data), seen)
		}
		keys[lang] = seen
	}
	return keys
}

func TestCatalogKeySetsMatchAcrossLocales(t *testing.T) {
	keys := catalogKeys(t)
	base, ok := keys["en"]
	if !ok || len(base) == 0 {
		t.Fatal("en catalog is missing or empty")
	}
	for lang, set := range keys {
		if lang == "en" {
			continue
		}
		for k := range base {
			if !set[k] {
				t.Errorf("locale %s missing key %q (present in en)", lang, k)
			}
		}
		for k := range set {
			if !base[k] {
				t.Errorf("locale %s has extra key %q (absent in en)", lang, k)
			}
		}
	}
}

func TestBundleLoadsCleanly(t *testing.T) {
	_ = loadBundle()
	if bundleErr != nil {
		t.Fatalf("embedded catalog failed to parse: %v", bundleErr)
	}
}

func TestSupportedDiscoversLocales(t *testing.T) {
	got := Supported()
	if len(got) < 2 {
		t.Fatalf("expected at least en and zh-CN, got %v", got)
	}
	found := map[string]bool{}
	for _, l := range got {
		found[l] = true
	}
	if !found["en"] || !found["zh-CN"] {
		t.Fatalf("expected en and zh-CN in %v", got)
	}
}

func TestLookupAndFallback(t *testing.T) {
	SetLang("en")
	if got := T("common.yes"); got != "yes" {
		t.Fatalf("en common.yes = %q", got)
	}
	if got := Tf("status.ok", nil); got != "ok" {
		t.Fatalf("en status.ok = %q", got)
	}
	SetLang("zh-CN")
	if got := T("common.yes"); got != "是" {
		t.Fatalf("zh-CN common.yes = %q", got)
	}
	if got := StatusLabel("running"); got != "运行中" {
		t.Fatalf("zh-CN status.running = %q", got)
	}
	// Missing key: falls back to en, then to the key itself.
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key returned %q", got)
	}
	if got := Tfallback("no.such.key", "raw"); got != "raw" {
		t.Fatalf("Tfallback missing = %q", got)
	}
	if got := Tfallback("common.yes", "raw"); got != "是" {
		t.Fatalf("Tfallback hit = %q", got)
	}
	// Unknown tokens pass through untouched.
	if got := StatusLabel("weird-token"); got != "weird-token" {
		t.Fatalf("unknown token = %q", got)
	}
	t.Cleanup(func() { SetLang("en") })
}

func TestInitPrecedence(t *testing.T) {
	t.Cleanup(func() {
		SetLang("en")
		os.Unsetenv("VMBENCH_LANG")
		os.Unsetenv("LC_ALL")
	})

	t.Setenv("VMBENCH_LANG", "")
	// Config file alone selects the language.
	Init("", "zh-CN")
	if Lang() != "zh-CN" {
		t.Fatalf("config-only Init: Lang() = %q", Lang())
	}
	// pref (--lang) beats config.
	Init("en", "zh-CN")
	if Lang() != "en" {
		t.Fatalf("pref should beat config: Lang() = %q", Lang())
	}
	// Env beats config.
	t.Setenv("VMBENCH_LANG", "en")
	Init("", "zh-CN")
	if Lang() != "en" {
		t.Fatalf("env should beat config: Lang() = %q", Lang())
	}
	// pref beats env.
	Init("zh-CN", "")
	if Lang() != "zh-CN" {
		t.Fatalf("pref should beat env: Lang() = %q", Lang())
	}
	// OS locale is the silent fallback.
	t.Setenv("VMBENCH_LANG", "")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	Init("", "")
	if Lang() != "zh-CN" {
		t.Fatalf("OS locale fallback: Lang() = %q", Lang())
	}
	// Unknown explicit pref stops at English with the active locale switched.
	Init("xx", "")
	if Lang() != "en" {
		t.Fatalf("unknown pref should land on en: Lang() = %q", Lang())
	}
}

func TestSetLangUnknownKeepsCurrent(t *testing.T) {
	SetLang("zh-CN")
	if SetLang("klingon") {
		t.Fatal("SetLang(klingon) should fail")
	}
	if Lang() != "zh-CN" {
		t.Fatalf("language changed to %q", Lang())
	}
	t.Cleanup(func() { SetLang("en") })
}

func TestYesNoUnknown(t *testing.T) {
	SetLang("zh-CN")
	if YesNo(true) != "是" || YesNo(false) != "否" {
		t.Fatalf("YesNo zh-CN = %q/%q", YesNo(true), YesNo(false))
	}
	if Unknown() != "未知" {
		t.Fatalf("Unknown zh-CN = %q", Unknown())
	}
	t.Cleanup(func() { SetLang("en") })
}

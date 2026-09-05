// Package i18n provides interface-language selection and message lookup for
// all user-facing surfaces (TUI, CLI, console and HTML reports).
//
// Locale data lives in messages/<locale>/*.toml, is embedded at build time,
// and is discovered automatically: adding a language means adding a directory.
// JSON report field names, status enum tokens, and persisted messages stay
// English; translation happens only at render time.
//
// Usage mirrors the theme package: call Init once at startup, then read
// package-level T/Tf from render code.
package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed messages
var messageFS embed.FS

// defaultLang is the base locale and final fallback for every lookup.
const defaultLang = "en"

var (
	bundleOnce sync.Once
	bundle     *goi18n.Bundle
	bundleErr  error // first embedded catalog parse error; surfaced by TestBundleLoadsCleanly

	supportedOnce sync.Once
	supportedList []string

	active     atomic.Pointer[goi18n.Localizer]
	activeLang atomic.Value // string

	noticed sync.Map // missing-key / unsupported-language notices already printed
)

// Init selects the active language with precedence:
// pref (--lang flag value) > VMBENCH_LANG env > cfgLang (config file) >
// OS locale (LC_ALL/LC_MESSAGES/LANG) > defaultLang.
// An explicit but unknown pref or env value prints a notice and stops at English.
// Call once at startup, before any T call; the --lang flag is applied later
// via ApplyLang after subcommand parsing.
func Init(pref, cfgLang string) {
	if pref != "" {
		explicit(pref)
		return
	}
	if env := os.Getenv("VMBENCH_LANG"); env != "" {
		explicit(env)
		return
	}
	if canon := resolveLang(cfgLang); canon != "" {
		setActive(canon)
		return
	}
	if canon := resolveLang(osLocale()); canon != "" {
		setActive(canon)
		return
	}
	setActive(defaultLang)
}

// explicit resolves a user-provided value: use it, or notice + English.
func explicit(v string) {
	if canon := resolveLang(v); canon != "" {
		setActive(canon)
		return
	}
	noticeUnsupported(v)
	setActive(defaultLang)
}

// SetLang switches the active language at runtime. An unknown value keeps the
// current language, prints a notice, and returns false.
func SetLang(lang string) bool {
	canon := resolveLang(lang)
	if canon == "" {
		noticeUnsupported(lang)
		return false
	}
	setActive(canon)
	return true
}

// ApplyLang applies a non-empty --lang flag value after flag parsing.
func ApplyLang(pref string) {
	if pref != "" {
		SetLang(pref)
	}
}

// Lang returns the canonical active locale (e.g. "en", "zh-CN").
func Lang() string {
	v, _ := activeLang.Load().(string)
	return v
}

// LanguageTag returns the BCP47 tag for HTML output (e.g. <html lang=...>).
func LanguageTag() string { return Lang() }

// Supported lists the discovered locale directory names, sorted.
func Supported() []string {
	supportedOnce.Do(func() {
		entries, err := fs.ReadDir(messageFS, "messages")
		if err != nil {
			supportedList = []string{defaultLang}
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				supportedList = append(supportedList, e.Name())
			}
		}
		if len(supportedList) == 0 {
			supportedList = []string{defaultLang}
		}
		sort.Strings(supportedList)
	})
	return append([]string(nil), supportedList...)
}

func setActive(lang string) {
	b := loadBundle()
	// Active locale first, default second: go-i18n falls back per-language.
	loc := goi18n.NewLocalizer(b, lang, defaultLang)
	active.Store(loc)
	activeLang.Store(lang)
}

func loadBundle() *goi18n.Bundle {
	bundleOnce.Do(func() {
		bundle = goi18n.NewBundle(language.English)
		bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
		for _, lang := range Supported() {
			entries, err := fs.ReadDir(messageFS, "messages/"+lang)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
					continue
				}
				data, err := messageFS.ReadFile("messages/" + lang + "/" + e.Name())
				if err != nil {
					continue
				}
				// go-i18n derives the language tag from the file name,
				// so hand it a virtual "<domain>.<locale>.toml" path.
				virtual := strings.TrimSuffix(e.Name(), ".toml") + "." + lang + ".toml"
				if _, err := bundle.ParseMessageFileBytes(data, virtual); err != nil && bundleErr == nil {
					bundleErr = err
				}
			}
		}
	})
	return bundle
}

// T returns the message for key in the active language, falling back to the
// default locale, then to the key itself (missing keys stay greppable).
func T(key string) string {
	return Tf(key, nil)
}

// Tf returns the message for key with named template data
// (e.g. Tf("x", map[string]any{"Count": 3}) for "{{.Count}} sections ok").
func Tf(key string, data map[string]any) string {
	loc := active.Load()
	if loc == nil {
		setActive(defaultLang)
		loc = active.Load()
	}
	s, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: key, TemplateData: data})
	if err != nil {
		missingKey(key)
		return key
	}
	return s
}

// Tfallback returns the translated message for key, or fallback when the key
// is absent from every catalog (e.g. an untranslated catalog Definition name).
func Tfallback(key, fallback string) string {
	if s := T(key); s != key {
		return s
	}
	return fallback
}

func missingKey(key string) {
	if os.Getenv("VMBENCH_I18N_DEBUG") == "" {
		return
	}
	if _, dup := noticed.LoadOrStore("key:"+key, true); dup {
		return
	}
	fmt.Fprintf(os.Stderr, "i18n: missing message %q [%s]\n", key, Lang())
}

func noticeUnsupported(lang string) {
	if _, dup := noticed.LoadOrStore("lang:"+strings.ToLower(lang), true); dup {
		return
	}
	fmt.Fprintf(os.Stderr, "notice: unsupported language %q; using English (supported: %s)\n",
		lang, strings.Join(Supported(), ", "))
}

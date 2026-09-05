package i18n

import (
	"os"
	"strings"
)

// osLocale returns the raw OS locale string (LC_ALL > LC_MESSAGES > LANG).
func osLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// aliases maps common locale spellings to canonical directory names.
// Keys are lowercase with "-" separators.
var aliases = map[string]string{
	"en":         "en",
	"en-us":      "en",
	"en-gb":      "en",
	"zh":         "zh-CN",
	"zh-cn":      "zh-CN",
	"zh-hans":    "zh-CN",
	"zh-sg":      "zh-CN",
	"zh-hans-cn": "zh-CN",
	"zh-hans-sg": "zh-CN",
}

// resolveLang maps a raw user/env/config/OS value ("zh_CN.UTF-8", "zh-Hans",
// "en_US") to a canonical supported locale, or "" when nothing matches.
// Matching is case-insensitive: exact canonical, alias table, then base
// language (any zh-* resolves to zh-CN while it is the only Chinese locale).
func resolveLang(raw string) string {
	cand := normalizeLocale(raw)
	if cand == "" {
		return ""
	}
	supported := Supported()
	for _, sup := range supported {
		if cand == strings.ToLower(sup) {
			return sup
		}
	}
	if canon, ok := aliases[cand]; ok {
		for _, sup := range supported {
			if sup == canon {
				return canon
			}
		}
	}
	base := cand
	if i := strings.IndexByte(cand, '-'); i >= 0 {
		base = cand[:i]
	}
	for _, sup := range supported {
		if baseOf(strings.ToLower(sup)) == base {
			return sup
		}
	}
	return ""
}

// normalizeLocale lowercases and strips encodings ("zh_CN.UTF-8") and
// modifiers ("en_US@posix"), and unifies "_" to "-".
func normalizeLocale(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ToLower(s)
}

func baseOf(lowered string) string {
	if i := strings.IndexByte(lowered, '-'); i >= 0 {
		return lowered[:i]
	}
	return lowered
}

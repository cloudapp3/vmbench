package i18n

import "testing"

func TestResolveLang(t *testing.T) {
	cases := map[string]string{
		"en":          "en",
		"EN":          "en",
		"en_US":       "en",
		"en_US.UTF-8": "en",
		"en-GB":       "en",
		"zh":          "zh-CN",
		"zh_CN.UTF-8": "zh-CN",
		"zh-CN":       "zh-CN",
		"zh-Hans":     "zh-CN",
		"zh-Hans-CN":  "zh-CN",
		"zh_SG":       "zh-CN",
		"C":           "",
		"C.UTF-8":     "",
		"POSIX":       "",
		"":            "",
		"fr_FR.UTF-8": "",
		"ja_JP.UTF-8": "",
	}
	for raw, want := range cases {
		if got := resolveLang(raw); got != want {
			t.Errorf("resolveLang(%q) = %q, want %q", raw, got, want)
		}
	}
}

// zh-TW resolves to zh-CN via base-language matching: while zh-CN is the only
// Chinese locale, any zh-* is better than English for those users.
func TestResolveLangBaseFallback(t *testing.T) {
	if got := resolveLang("zh_TW"); got != "zh-CN" {
		t.Fatalf("resolveLang(zh_TW) = %q, want zh-CN", got)
	}
}

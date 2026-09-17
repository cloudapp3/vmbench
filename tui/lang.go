package tui

import (
	"github.com/cloudapp3/vmbench/i18n"
)

// Language cycling mirrors theme cycling: a dashboard line plus the "l" key
// walk a ring of states. "" (auto) follows VMBENCH_LANG / OS locale / default;
// the other states are the canonical codes from i18n.Supported().

// langStates returns the cycle ring: auto first, then every supported locale
// in i18n.Supported() order ("" = follow system).
func langStates() []string {
	return append([]string{""}, i18n.Supported()...)
}

// cycleLangState returns the state after cur in the ring; unknown values
// restart from auto.
func cycleLangState(cur string) string {
	states := langStates()
	for i, s := range states {
		if s == cur {
			return states[(i+1)%len(states)]
		}
	}
	return states[0]
}

// applyLangState switches the active language: "" re-resolves from
// VMBENCH_LANG / OS locale / default, anything else is canonical and goes
// through SetLang (never the SetLang("") stderr-notice path).
func applyLangState(explicit string) {
	if explicit == "" {
		i18n.Init("", "")
		return
	}
	i18n.SetLang(explicit)
}

// langNativeName maps canonical codes to each language's own name, shown
// regardless of the active UI language (language-picker convention).
func langNativeName(lang string) string {
	switch lang {
	case "en":
		return "English"
	case "zh-CN":
		return "中文"
	default:
		return lang
	}
}

// langLineLabel renders the value half of the dashboard language line: the
// currently active language by native name, marked with the auto hint when no
// explicit choice is pinned.
func langLineLabel(explicit string) string {
	label := langNativeName(i18n.Lang())
	if explicit == "" {
		label += " · " + i18n.T("tui.dashboard.langAuto")
	}
	return label
}

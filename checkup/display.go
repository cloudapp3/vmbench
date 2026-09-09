package checkup

import "github.com/cloudapp3/vmbench/i18n"

// Localized display helpers for spec structs. The struct fields stay English
// (they feed JSON output and MCP descriptions); renderers call these so the
// active language is applied at display time with the English value as
// fallback for untranslated keys.

func (s PresetSpec) LocalizedName() string {
	return i18n.Tfallback("checkup.preset."+s.ID+".name", s.Name)
}

func (s PresetSpec) LocalizedDescription() string {
	return i18n.Tfallback("checkup.preset."+s.ID+".description", s.Description)
}

func (s RoutePresetSpec) LocalizedName() string {
	return i18n.Tfallback("checkup.route."+s.ID+".name", s.Name)
}

func (s RoutePresetSpec) LocalizedDescription() string {
	return i18n.Tfallback("checkup.route."+s.ID+".description", s.Description)
}

func (s SpeedProviderSpec) LocalizedName() string {
	return i18n.Tfallback("checkup.speed."+s.ID+".name", s.Name)
}

func (s SpeedProviderSpec) LocalizedDescription() string {
	return i18n.Tfallback("checkup.speed."+s.ID+".description", s.Description)
}

func (s SpeedProviderSpec) LocalizedRequires() string {
	if s.Requires == "" {
		return ""
	}
	return i18n.Tfallback("checkup.speed."+s.ID+".requires", s.Requires)
}

func (s IPSourceSpec) LocalizedName() string {
	return i18n.Tfallback("checkup.ipSource."+s.ID+".name", s.Name)
}

func (s IPSourceSpec) LocalizedDescription() string {
	return i18n.Tfallback("checkup.ipSource."+s.ID+".description", s.Description)
}

func (s IPSourceSpec) LocalizedRequires() string {
	if s.Requires == "" {
		return ""
	}
	return i18n.Tfallback("checkup.ipSource."+s.ID+".requires", s.Requires)
}

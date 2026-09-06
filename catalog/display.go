package catalog

import "github.com/cloudapp3/vmbench/i18n"

// Localized display helpers. Struct fields stay English (they feed JSON
// output and MCP descriptions); renderers call these so the active language
// is applied at display time with the English value as fallback.
//
// Workload Definition names are intentionally NOT translated: they are
// functional identifiers matched by --filter and recorded in report data.

func (s HardwareToolSpec) LocalizedName() string {
	return i18n.Tfallback("catalog.tool."+s.ID+".name", s.Name)
}

func (s HardwareToolSpec) LocalizedDescription() string {
	return i18n.Tfallback("catalog.tool."+s.ID+".description", s.Description)
}

package i18n

// Render-time token→label mapping. Status enum tokens ("ok", "fail", ...) and
// section IDs ("hardware", "ip_quality", ...) are machine values shared by
// JSON reports, compare logic, and persistence; they must never be localized
// at the data layer. These helpers are the single translation point used by
// the TUI, console writers, and HTML templates.

// StatusLabel returns the display label for a status token, or the token
// itself when no translation exists.
func StatusLabel(token string) string {
	return Tfallback("status."+token, token)
}

// SectionLabel returns the display label for a suite section ID, or the ID
// itself when no translation exists.
func SectionLabel(id string) string {
	return Tfallback("suite.section."+id, id)
}

// YesNo localizes boolean display values ("yes"/"no").
func YesNo(v bool) string {
	if v {
		return T("common.yes")
	}
	return T("common.no")
}

// Unknown localizes the "unknown" placeholder used in reports.
func Unknown() string {
	return T("common.unknown")
}

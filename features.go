package vmbench

// FeatureCompare gates every compare-report surface: the `vmbench compare`
// CLI command, `vmbench history compare`, and the TUI compare menu, picker,
// and -compare-a/-compare-b flags. Hidden entries fall through to their
// unknown-command paths. The implementation (checkupcompare, report/compare,
// tui compare pages) stays in the tree; flip to true to re-expose everything.
const FeatureCompare = false

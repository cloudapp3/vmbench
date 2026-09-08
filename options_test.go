package vmbench

import (
	"strings"
	"testing"
)

func TestPrepareOptionsUsesSafeDefaults(t *testing.T) {
	_, filter, warnings := prepareOptions(Options{})
	if filter != "" || len(warnings) != 0 {
		t.Fatalf("filter=%q warnings=%v, want empty", filter, warnings)
	}
}

func TestPrepareOptionsInvalidFilterSelectsNothing(t *testing.T) {
	_, filter, warnings := prepareOptions(Options{Filter: "["})
	if filter != "a^" {
		t.Fatalf("filter = %q, want never-match expression", filter)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "no workloads selected") {
		t.Fatalf("warnings = %v, want invalid filter warning", warnings)
	}
}

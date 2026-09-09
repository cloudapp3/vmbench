package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/catalog"
)

func TestHardwareToolPreflightSuggestsFetchForProvisionableTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	// Redirect the toolbin user cache too, so a developer machine that has
	// already run `vmbench tools fetch` does not satisfy the lookup.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	tools := []string{catalog.HardwareToolFio, catalog.HardwareToolMBW}

	var output bytes.Buffer
	printHardwareToolPreflight(&output, tools, regexp.MustCompile(`^Disk$`))
	got := output.String()
	if !strings.Contains(got, "vmbench tools fetch fio") {
		t.Fatalf("preflight output = %q, want fetch hint for fio", got)
	}
	if strings.Contains(got, "mbw") {
		t.Fatalf("preflight output = %q, mbw is not provisionable and must not appear in the fetch hint", got)
	}
}

func TestFetchableToolNames(t *testing.T) {
	got := fetchableToolNames([]string{"fio", "mbw", "sysbench", "geekbench"})
	if strings.Join(got, ",") != "fio,sysbench" {
		t.Fatalf("fetchableToolNames() = %v, want [fio sysbench]", got)
	}
}

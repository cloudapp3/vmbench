package vmbench

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudapp3/vmbench/catalog"
)

// OptionsError reports invalid run configuration before any workload starts.
type OptionsError struct {
	Problems []string
}

func (e *OptionsError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return ""
	}
	return strings.Join(e.Problems, "; ")
}

// ValidateOptions applies the shared run configuration contract used by CLI,
// TUI, and MCP entry points. Zero values which have documented defaults are
// valid; explicit invalid values are rejected.
func ValidateOptions(opts Options) error {
	problems := make([]string, 0, 4)
	if opts.Iterations < 0 || opts.Iterations > 9 {
		problems = append(problems, "iterations must be between 1 and 9")
	}
	if opts.Timeout < 0 {
		problems = append(problems, "timeout must not be negative")
	}
	if filter := strings.TrimSpace(opts.Filter); filter != "" {
		if _, err := regexp.Compile(filter); err != nil {
			problems = append(problems, "invalid filter regex: "+err.Error())
		}
	}

	switch strings.ToLower(strings.TrimSpace(opts.Engine)) {
	case "", "external", "native", "full":
	default:
		problems = append(problems, "engine must be one of: external, native, full")
	}
	if invalid := firstInvalidHardwareTool(opts.HardwareTools); invalid != "" {
		problems = append(problems, fmt.Sprintf("unknown hardware tool %q; available: %s", invalid, strings.Join(catalog.HardwareToolIDs(), ", ")))
	}
	if len(problems) > 0 {
		return &OptionsError{Problems: problems}
	}
	return nil
}

// NormalizeOptions validates and resolves documented defaults without running
// workloads. Callers can persist the returned value as execution provenance.
func NormalizeOptions(opts Options) (Options, error) {
	if err := ValidateOptions(opts); err != nil {
		return Options{}, err
	}
	norm, _, _ := prepareOptions(opts)
	return norm, nil
}

func firstInvalidHardwareTool(values []string) string {
	for _, item := range normalizeStringList(values) {
		if len(catalog.StandardizeHardwareTools([]string{item})) == 0 {
			return item
		}
	}
	return ""
}

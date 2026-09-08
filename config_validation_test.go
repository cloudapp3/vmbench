package vmbench

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeOptionsUsesCanonicalDefaults(t *testing.T) {
	norm, err := NormalizeOptions(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if norm.Iterations != 3 || norm.Timeout != 5*time.Minute {
		t.Fatalf("NormalizeOptions() = %+v, want canonical defaults", norm)
	}
}

func TestValidateOptionsRejectsInvalidRunConfiguration(t *testing.T) {
	err := ValidateOptions(Options{
		Iterations:    10,
		Timeout:       -time.Second,
		Filter:        "[",
		Engine:        "unknown",
		HardwareTools: []string{"openssl", "unknown"},
	})
	if err == nil {
		t.Fatal("ValidateOptions() error = nil")
	}
	for _, want := range []string{"iterations", "timeout", "filter regex", "engine", "unknown hardware tool"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateOptions() error = %q, want %q", err, want)
		}
	}
}

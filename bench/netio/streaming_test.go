package netio

import (
	"testing"

	"github.com/oneclickvirt/UnlockTests/model"
)

func TestMapUnlockStatus(t *testing.T) {
	cases := map[string]string{
		model.StatusYes:         "available",
		model.StatusRestricted:  "available",
		model.StatusCDNRelay:    "available",
		model.StatusNo:          "blocked",
		model.StatusBanned:      "blocked",
		model.StatusTimeout:     "unknown",
		model.StatusRateLimited: "unknown",
		model.StatusNetworkErr:  "unknown",
		model.StatusNoIPv6:      "unknown",
		model.StatusDNSFailed:   "unknown",
		model.StatusUnexpected:  "unknown",
		"FutureStatus":          "unknown",
	}
	for raw, want := range cases {
		if got := mapUnlockStatus(raw); got != want {
			t.Errorf("mapUnlockStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeMediaIPVersion(t *testing.T) {
	cases := map[string]string{
		"v4": "ipv4", "ipv4": "ipv4", "4": "ipv4",
		"v6": "ipv6", "ipv6": "ipv6", "6": "ipv6",
		"dual": "auto", "both": "auto", "": "auto",
	}
	for in, want := range cases {
		if got := normalizeMediaIPVersion(in); got != want {
			t.Errorf("normalizeMediaIPVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMediaSetIDDefaultsToAll(t *testing.T) {
	if got := mediaSetID(""); got != "all" {
		t.Errorf("mediaSetID(\"\") = %q, want all", got)
	}
	if got := mediaSetID(" JP "); got != "jp" {
		t.Errorf("mediaSetID(\" JP \") = %q, want jp", got)
	}
}

func TestValidateMediaSet(t *testing.T) {
	for _, valid := range []string{"", "all", "globe", "jp,kr", "hk"} {
		if err := ValidateMediaSet(valid); err != nil {
			t.Errorf("ValidateMediaSet(%q) unexpected error: %v", valid, err)
		}
	}
	for _, invalid := range []string{"bogus", "jp,bogus"} {
		if err := ValidateMediaSet(invalid); err == nil {
			t.Errorf("ValidateMediaSet(%q) expected error", invalid)
		}
	}
}

func TestMediaIDFromName(t *testing.T) {
	cases := map[string]string{
		"Netflix":      "netflix",
		"Disney Plus":  "disney_plus",
		"ChatGPT Web!": "chatgpt_web",
		"  ":           "",
	}
	for in, want := range cases {
		if got := mediaIDFromName(in); got != want {
			t.Errorf("mediaIDFromName(%q) = %q, want %q", in, got, want)
		}
	}
}

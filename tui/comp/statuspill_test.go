package comp

import "testing"

func TestStatusFromStringRecognizesPartial(t *testing.T) {
	if got := StatusFromString(" PARTIAL "); got != StatusPartial {
		t.Fatalf("StatusFromString(partial) = %q, want %q", got, StatusPartial)
	}
}

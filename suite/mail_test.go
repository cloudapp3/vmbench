package suite

import (
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func TestSummarizeMailResultsSeparatesReachabilityFromProbeErrors(t *testing.T) {
	tests := []struct {
		name        string
		results     []netio.PortProbe
		wantStatus  string
		wantMessage string
	}{
		{name: "none", wantStatus: "error", wantMessage: "no mail ports selected"},
		{
			name: "conclusive blocked ports",
			results: []netio.PortProbe{
				{Status: netio.MailPortStatusOpen},
				{Status: netio.MailPortStatusRefused},
				{Status: netio.MailPortStatusTimeout},
			},
			wantStatus:  "ok",
			wantMessage: "1/3 ports open; 1 refused; 1 timeout",
		},
		{
			name: "partial probe failure",
			results: []netio.PortProbe{
				{Status: netio.MailPortStatusOpen},
				{Status: netio.MailPortStatusError},
			},
			wantStatus:  "partial",
			wantMessage: "1/2 ports open; 1 errors",
		},
		{
			name:        "all probes failed",
			results:     []netio.PortProbe{{Status: netio.MailPortStatusError}, {Status: "unknown"}},
			wantStatus:  "error",
			wantMessage: "0/2 ports open; 2 errors",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := summarizeMailResults(tt.results)
			if status != tt.wantStatus || !strings.Contains(message, tt.wantMessage) {
				t.Fatalf("summarizeMailResults() = %q, %q; want %q, %q", status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}

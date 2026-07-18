package netio

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestResolveTraceTargetSelectsRequestedIPv6(t *testing.T) {
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("192.0.2.10")},
			{IP: net.ParseIP("2001:db8::10")},
		}, nil
	}
	got, err := resolveTraceTargetWith(context.Background(), "dual.example", "v6", lookup)
	if err != nil {
		t.Fatalf("resolveTraceTargetWith() error = %v", err)
	}
	if got != "2001:db8::10" {
		t.Fatalf("resolved target = %q, want IPv6 address", got)
	}
}

func TestTraceCommandSpecsCarryLinuxIPv6Family(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux command flags")
	}
	for _, spec := range traceCommandSpecsForTarget("2001:db8::10", 443, "v6") {
		switch spec.name {
		case "traceroute", "tracepath":
			if !slices.Contains(spec.args, "-6") {
				t.Fatalf("%s args = %v, want -6", spec.name, spec.args)
			}
		}
	}
}

func TestSystemTracerouteEvidenceRecordsActualFallbackAndErrors(t *testing.T) {
	tests := []struct {
		name         string
		available    string
		output       string
		wantProtocol string
		wantTool     string
		wantError    bool
	}{
		{name: "tracepath fallback", available: "tracepath", output: " 1  192.0.2.1  1.0 ms\n", wantProtocol: "udp", wantTool: "tracepath"},
		{name: "attempted command failure", available: "tcptraceroute", output: "no hops\n", wantProtocol: "tcp", wantTool: "tcptraceroute", wantError: true},
		{name: "no command", wantProtocol: "none", wantTool: "none", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookPath := func(name string) (string, error) {
				if name == tt.available {
					return "/test/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			run := func(context.Context, string, ...string) ([]byte, error) {
				if tt.wantError {
					return []byte(tt.output), errors.New("command failed")
				}
				return []byte(tt.output), nil
			}
			_, protocol, tool, err := systemTracerouteWithEvidence(context.Background(), "192.0.2.10", "v4", 80, lookPath, run)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError %v", err, tt.wantError)
			}
			if protocol != tt.wantProtocol || tool != tt.wantTool {
				t.Fatalf("evidence = %s/%s, want %s/%s", protocol, tool, tt.wantProtocol, tt.wantTool)
			}
		})
	}
}

func TestProbeTracerouteTargetsKeepsResolverFailureEvidence(t *testing.T) {
	results, err := probeTracerouteTargetEvidence(context.Background(), []TraceTarget{{
		ID: "bad-family", Name: "bad", IPFamily: "v6", Endpoint: "192.0.2.10",
	}}, systemTracerouteTargetEvidence)
	if err != nil {
		t.Fatalf("probeTracerouteTargetEvidence() error = %v", err)
	}
	if len(results) != 1 || results[0].ProbeProtocol != "none" || results[0].ProbeTool != "resolver" || !strings.Contains(results[0].Error, "does not match") {
		t.Fatalf("result = %+v", results)
	}
}

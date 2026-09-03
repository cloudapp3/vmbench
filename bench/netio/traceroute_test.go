package netio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseTracerouteOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []Hop
	}{
		{
			name: "linux traceroute",
			output: `traceroute to 203.0.113.8 (203.0.113.8), 30 hops max
 1  192.0.2.1  0.321 ms  0.210 ms  0.200 ms
 2  * * *
 3  edge.example (198.51.100.7)  12.75 ms`,
			want: []Hop{
				{TTL: 1, IP: "192.0.2.1", RTTMs: 0.321},
				{TTL: 2, Timeout: true},
				{TTL: 3, IP: "198.51.100.7", RTTMs: 12.75},
			},
		},
		{
			name: "windows tracert",
			output: `Tracing route to 203.0.113.8 over a maximum of 30 hops
  1    <1 ms    <1 ms    <1 ms  192.0.2.1
  2     *        *        *     Request timed out.
  3    15 ms    14 ms    14 ms  203.0.113.8`,
			want: []Hop{
				{TTL: 1, IP: "192.0.2.1", RTTMs: 0.5},
				{TTL: 2, Timeout: true},
				{TTL: 3, IP: "203.0.113.8", RTTMs: 15},
			},
		},
		{
			name: "tracepath duplicate local line",
			output: ` 1?: [LOCALHOST]                      pmtu 1500
 1:  192.0.2.1                                           0.120ms
 2:  no reply
 3:  203.0.113.8                                         8.500ms reached`,
			want: []Hop{
				{TTL: 1, IP: "192.0.2.1", RTTMs: 0.120},
				{TTL: 2, Timeout: true},
				{TTL: 3, IP: "203.0.113.8", RTTMs: 8.5},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTracerouteOutput([]byte(tt.output))
			if err != nil {
				t.Fatalf("parseTracerouteOutput() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseTracerouteOutput() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseTracerouteOutputDoesNotTreatErrorTargetAsHop(t *testing.T) {
	output := "traceroute to 203.0.113.8 (203.0.113.8)\n" +
		"dial tcp 203.0.113.8:80: connect: connection refused\n" +
		" 1  * * *\n"
	_, err := parseTracerouteOutput([]byte(output))
	if err == nil || !strings.Contains(err.Error(), "no valid hops") {
		t.Fatalf("parseTracerouteOutput() error = %v, want target in error text to be ignored", err)
	}
}

func TestParseTracerouteOutputRejectsOnlyTimeouts(t *testing.T) {
	_, err := parseTracerouteOutput([]byte(" 1  * * *\n 2  * * *\n"))
	if err == nil || !strings.Contains(err.Error(), "no valid hops") {
		t.Fatalf("parseTracerouteOutput() error = %v, want no valid hops", err)
	}
}

func TestTraceDestinationReachedMatchesResolvedIP(t *testing.T) {
	tests := []struct {
		name   string
		target string
		hops   []Hop
		want   bool
	}{
		{name: "reached IPv4", target: "203.0.113.8", hops: []Hop{{TTL: 1, IP: "192.0.2.1"}, {TTL: 2, IP: "203.0.113.8"}}, want: true},
		{name: "reached normalized IPv6", target: "2001:db8::8", hops: []Hop{{TTL: 1, IP: "2001:0db8:0:0:0:0:0:8"}}, want: true},
		{name: "only intermediate hops", target: "203.0.113.8", hops: []Hop{{TTL: 1, IP: "192.0.2.1"}, {TTL: 2, Timeout: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := traceDestinationReached(tt.hops, tt.target); got != tt.want {
				t.Fatalf("traceDestinationReached() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestTraceProbeResultEffectiveStatusReadsLegacyJSON(t *testing.T) {
	var result TraceProbeResult
	if err := json.Unmarshal([]byte(`{
  "target":{"name":"legacy","endpoint":"203.0.113.8"},
  "probe_protocol":"tcp","probe_tool":"traceroute",
  "hops":[{"ttl":1,"ip":"192.0.2.1","rtt_ms":1}]
}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.DestinationReached != nil {
		t.Fatalf("legacy destination evidence = %v, want absent", result.DestinationReached)
	}
	if got := result.EffectiveStatus(); got != TraceStatusOK {
		t.Fatalf("legacy EffectiveStatus() = %q, want %q", got, TraceStatusOK)
	}
}

func TestTraceProbeResultJSONIncludesReachabilityEvidence(t *testing.T) {
	reached := false
	result := TraceProbeResult{
		Target:             TraceTarget{Name: "partial", Endpoint: "route.example"},
		ResolvedTarget:     "203.0.113.8",
		DestinationReached: &reached,
		Status:             TraceStatusPartial,
		ProbeProtocol:      "tcp",
		ProbeTool:          "traceroute",
		Hops:               []Hop{{TTL: 1, IP: "192.0.2.1"}},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"resolved_target":"203.0.113.8"`, `"destination_reached":false`, `"status":"partial"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("trace JSON missing %s: %s", field, data)
		}
	}
}

func TestSystemTracerouteReportsMissingCommands(t *testing.T) {
	lookPath := func(name string) (string, error) {
		return "", exec.ErrNotFound
	}
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("command runner called without an available command")
		return nil, nil
	}

	_, err := systemTracerouteWith(context.Background(), "192.0.2.10", lookPath, run)
	if err == nil || !strings.Contains(err.Error(), "no traceroute command available") {
		t.Fatalf("systemTracerouteWith() error = %v, want missing-command error", err)
	}
}

func TestSystemTracerouteRejectsCommandWithoutValidHop(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == traceCommandNames()[0] {
			return "/test/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte(" 1  * * *\n 2  * * *\n"), nil
	}

	_, err := systemTracerouteWith(context.Background(), "192.0.2.10", lookPath, run)
	if err == nil || !strings.Contains(err.Error(), "produced no valid hops") {
		t.Fatalf("systemTracerouteWith() error = %v, want no-valid-hop error", err)
	}
}

func TestProbeTracerouteTargetsReturnsStructuredErrorsAndLimitsConcurrency(t *testing.T) {
	targets := make([]TraceTarget, 12)
	for i := range targets {
		targets[i] = TraceTarget{Name: fmt.Sprintf("target-%d", i), Endpoint: fmt.Sprintf("192.0.2.%d", i+1)}
	}
	var active atomic.Int32
	var maximum atomic.Int32
	probe := func(context.Context, string) ([]Hop, error) {
		current := active.Add(1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return nil, errors.New("trace failed")
	}

	results, err := probeTracerouteTargets(context.Background(), targets, probe)
	if err != nil {
		t.Fatalf("probeTracerouteTargets() error = %v", err)
	}
	if got := maximum.Load(); got > maxConcurrentTraces {
		t.Fatalf("maximum concurrency = %d, want <= %d", got, maxConcurrentTraces)
	}
	for i, result := range results {
		if !strings.Contains(result.Error, "trace failed") {
			t.Errorf("result[%d].Error = %q, want structured trace error", i, result.Error)
		}
		if result.EffectiveStatus() != TraceStatusError || result.DestinationReached == nil || *result.DestinationReached {
			t.Errorf("result[%d] status evidence = %+v", i, result)
		}
	}
}

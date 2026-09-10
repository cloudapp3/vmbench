package netio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseICMPEchoTimesAcrossPingFlavors(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []time.Duration
	}{
		{
			name: "iputils linux",
			out: strings.Join([]string{
				"PING 14.22.48.18 (14.22.48.18) 56(84) bytes of data.",
				"64 bytes from 14.22.48.18: icmp_seq=1 ttl=52 time=10.7 ms",
				"64 bytes from 14.22.48.18: icmp_seq=2 ttl=52 time=11.3 ms",
				"",
				"--- 14.22.48.18 ping statistics ---",
				"2 packets transmitted, 2 received, 0% packet loss, time 1002ms",
				"rtt min/avg/max/mdev = 10.7/11.0/11.3/0.3 ms",
			}, "\n"),
			want: []time.Duration{10700 * time.Microsecond, 11300 * time.Microsecond},
		},
		{
			name: "busybox",
			out: strings.Join([]string{
				"PING 14.22.48.18 (14.22.48.18): 56 data bytes",
				"64 bytes from 14.22.48.18: seq=0 ttl=52 time=9.824 ms",
				"--- 14.22.48.18 ping statistics ---",
				"1 packets transmitted, 1 packets received, 0% packet loss",
			}, "\n"),
			want: []time.Duration{9824 * time.Microsecond},
		},
		{
			name: "macos",
			out: strings.Join([]string{
				"PING 14.22.48.18: 56 data bytes",
				"64 bytes from 14.22.48.18: icmp_seq=0 ttl=52 time=9.974 ms",
				"--- 14.22.48.18 ping statistics ---",
				"1 packets transmitted, 1 packets received, 0.0% packet loss",
			}, "\n"),
			want: []time.Duration{9974 * time.Microsecond},
		},
		{
			name: "windows english",
			out: strings.Join([]string{
				"Pinging 61.139.2.69 with 32 bytes of data:",
				"Reply from 61.139.2.69: bytes=32 time=338ms TTL=113",
				"Reply from 61.139.2.69: bytes=32 time<1ms TTL=113",
				"",
				"Approximate round trip times in milli-seconds:",
				"    Minimum = 0ms, Maximum = 338ms, Average = 169ms",
			}, "\r\n"),
			want: []time.Duration{338 * time.Millisecond, time.Millisecond},
		},
		{
			name: "windows chinese",
			out: strings.Join([]string{
				"正在 Ping 61.139.2.69 具有 32 字节的数据:",
				"来自 61.139.2.69 的回复: 字节=32 时间=338ms TTL=113",
				"请求超时。",
			}, "\r\n"),
			want: []time.Duration{338 * time.Millisecond},
		},
		{
			name: "destination unreachable only",
			out:  "From 10.0.0.1 icmp_seq=1 Destination Host Unreachable",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseICMPEchoTimes([]byte(tt.out))
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("parseICMPEchoTimes() = %v, want no replies", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseICMPEchoTimes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestICMPCommandSpecForLinuxFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("command flags are platform-specific; skipping on %s", runtime.GOOS)
	}
	v4 := icmpCommandSpecFor("192.0.2.1")
	if !reflect.DeepEqual(v4, icmpCommandSpec{name: "ping", args: []string{"-c", "10", "-w", "13", "192.0.2.1"}}) {
		t.Fatalf("v4 spec = %+v", v4)
	}
	v6 := icmpCommandSpecFor("2001:db8::1")
	if !reflect.DeepEqual(v6, icmpCommandSpec{name: "ping", args: []string{"-c", "10", "-w", "13", "-6", "2001:db8::1"}}) {
		t.Fatalf("v6 spec = %+v", v6)
	}
}

func TestSystemICMPEchoWithParsesReplies(t *testing.T) {
	output := "64 bytes from 14.22.48.18: icmp_seq=1 ttl=52 time=10.7 ms\n"
	evidence := systemICMPEchoWith(context.Background(), "14.22.48.18", "v4",
		func(string) (string, error) { return "/usr/bin/ping", nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte(output), nil },
	)
	if evidence.err != nil {
		t.Fatalf("err = %v, want nil", evidence.err)
	}
	if evidence.tool != "ping" || len(evidence.rtts) != 1 || evidence.rtts[0] != 10700*time.Microsecond {
		t.Fatalf("evidence = %+v, want one 10.7ms reply from ping", evidence)
	}
}

func TestSystemICMPEchoWithReportsMissingBinary(t *testing.T) {
	evidence := systemICMPEchoWith(context.Background(), "14.22.48.18", "v4",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run must not be called when lookPath fails")
			return nil, nil
		},
	)
	if evidence.err == nil || !strings.Contains(evidence.err.Error(), "not available") {
		t.Fatalf("err = %v, want ping-not-available", evidence.err)
	}
	if len(evidence.rtts) != 0 {
		t.Fatalf("rtts = %v, want none", evidence.rtts)
	}
}

func TestSystemICMPEchoWithKeepsCommandError(t *testing.T) {
	evidence := systemICMPEchoWith(context.Background(), "14.22.48.18", "v4",
		func(string) (string, error) { return "/usr/bin/ping", nil },
		func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("exit status 1") },
	)
	if evidence.err == nil || !strings.Contains(evidence.err.Error(), "exit status 1") {
		t.Fatalf("err = %v, want command error retained", evidence.err)
	}
}

func TestPingTargetFallsBackToICMPWhenTCPSilent(t *testing.T) {
	result := pingTargetWithProbes(context.Background(), PingTarget{
		ID:       "tcp-silent",
		Name:     "TCP Silent",
		Endpoint: "192.0.2.1",
		Port:     80,
	}, func(context.Context, string, string) (net.Conn, error) {
		return nil, context.DeadlineExceeded
	}, func(context.Context, string, string) icmpEchoEvidence {
		return icmpEchoEvidence{
			rtts: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond},
			tool: "ping",
		}
	})

	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok via icmp fallback", result.Status)
	}
	if result.ProbeProtocol != "icmp-echo" || result.ProbeTool != "ping" {
		t.Fatalf("probe evidence = %q/%q, want icmp-echo/ping", result.ProbeProtocol, result.ProbeTool)
	}
	if result.ConnectionState != "" {
		t.Fatalf("ConnectionState = %q, want empty (no TCP connection to describe)", result.ConnectionState)
	}
	if result.Sent != pingProbes || result.Received != 3 || result.PacketLoss != 70 {
		t.Fatalf("probe counts = sent %d received %d loss %.1f, want %d/3/70", result.Sent, result.Received, result.PacketLoss, pingProbes)
	}
	if result.AvgLatencyMs != 20 {
		t.Fatalf("AvgLatencyMs = %.2f, want 20", result.AvgLatencyMs)
	}
	if math.Abs(result.JitterMs-6.667) > 0.01 {
		t.Fatalf("JitterMs = %.3f, want ~6.667", result.JitterMs)
	}
	if !strings.Contains(result.Message, "icmp echo fallback") {
		t.Fatalf("Message = %q, want fallback explanation", result.Message)
	}
}

func TestPingTargetSkipsICMPFallbackWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probed := false
	result := pingTargetWithProbes(ctx, PingTarget{
		ID:       "canceled",
		Name:     "Canceled",
		Endpoint: "192.0.2.1",
		Port:     80,
	}, func(context.Context, string, string) (net.Conn, error) {
		return nil, context.Canceled
	}, func(context.Context, string, string) icmpEchoEvidence {
		probed = true
		return icmpEchoEvidence{rtts: []time.Duration{time.Millisecond}}
	})

	if probed {
		t.Fatal("icmp probe ran despite canceled context")
	}
	if result.Status != "error" || result.ConnectionState != PingConnectionStateNoResponse {
		t.Fatalf("result = %+v, want error/no_response without fallback", result)
	}
}

func TestPingTargetKeepsTCPErrorWhenICMPFallbackFails(t *testing.T) {
	result := pingTargetWithProbes(context.Background(), PingTarget{
		ID:       "icmp-dead",
		Name:     "ICMP Dead",
		Endpoint: "192.0.2.1",
		Port:     80,
	}, func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("dial tcp 192.0.2.1:80: %w", context.DeadlineExceeded)
	}, func(context.Context, string, string) icmpEchoEvidence {
		return icmpEchoEvidence{err: errors.New("ping command ping not available")}
	})

	if result.Status != "error" {
		t.Fatalf("Status = %q, want error", result.Status)
	}
	if result.ProbeProtocol != "tcp-connect" {
		t.Fatalf("ProbeProtocol = %q, want tcp-connect", result.ProbeProtocol)
	}
	if result.Received != 0 || result.PacketLoss != 100 {
		t.Fatalf("probe counts = received %d loss %.1f, want 0/100", result.Received, result.PacketLoss)
	}
	if !strings.Contains(result.Message, "dial tcp 192.0.2.1:80") {
		t.Fatalf("Message = %q, want original TCP dial error", result.Message)
	}
}

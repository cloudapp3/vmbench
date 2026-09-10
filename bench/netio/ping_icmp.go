package netio

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// icmpEchoCount mirrors the TCP probe count so fallback rows report the
	// same sent volume as pure TCP rows.
	icmpEchoCount = pingProbes
	// icmpEchoDeadline bounds the whole ping run on Linux/BusyBox (-w).
	icmpEchoDeadline = 13 * time.Second
	// icmpEchoDarwinTimeout bounds the whole ping run on Darwin (-t).
	icmpEchoDarwinTimeout = 15 * time.Second
	// icmpEchoOverallWait is the exec-context cap covering platforms whose
	// flags only bound per-reply waits (Windows -w).
	icmpEchoOverallWait   = 25 * time.Second
	icmpEchoWindowsWaitMs = 3000
)

// pingReplyTimeRE matches the per-reply RTT marker across ping flavors:
// "time=10.7 ms" (iputils/BusyBox/BSD) and "time=338ms" / "time<1ms" /
// localized "时间=338ms" (Windows).
var pingReplyTimeRE = regexp.MustCompile(`(?i)[=<]\s*(\d+(?:\.\d+)?)\s*ms`)

// icmpEchoEvidence holds ICMP echo replies collected by the system ping
// fallback for TCP-silent targets.
type icmpEchoEvidence struct {
	rtts []time.Duration
	tool string
	err  error
}

// icmpEchoProbeFunc resolves host for family and collects ICMP echo RTTs.
type icmpEchoProbeFunc func(context.Context, string, string) icmpEchoEvidence

// icmpEchoFallback runs the ICMP probe unless the run was cancelled or no
// probe is configured (tests inject nil to keep TCP-only semantics).
func icmpEchoFallback(ctx context.Context, target PingTarget, icmpProbe icmpEchoProbeFunc) icmpEchoEvidence {
	if icmpProbe == nil || ctx.Err() != nil {
		return icmpEchoEvidence{err: ctx.Err()}
	}
	return icmpProbe(ctx, target.Endpoint, target.IPFamily)
}

// systemICMPEchoProbe shells out to the system ping binary, mirroring the
// traceroute approach: no raw sockets, so unprivileged runs still work and a
// missing ping binary degrades to the previous TCP-only verdict.
func systemICMPEchoProbe(ctx context.Context, host, family string) icmpEchoEvidence {
	return systemICMPEchoWith(ctx, host, family, exec.LookPath, runTraceCommand)
}

func systemICMPEchoWith(ctx context.Context, host, family string, lookPath traceLookPathFunc, run traceRunCommandFunc) icmpEchoEvidence {
	ip, err := resolveTraceTargetForFamily(ctx, host, family)
	if err != nil {
		return icmpEchoEvidence{tool: "resolver", err: err}
	}
	spec := icmpCommandSpecFor(ip)
	runCtx, cancel := context.WithTimeout(ctx, icmpEchoOverallWait)
	defer cancel()
	path, pathErr := lookPath(spec.name)
	if pathErr != nil {
		return icmpEchoEvidence{err: fmt.Errorf("ping command %s not available: %w", spec.name, pathErr)}
	}
	output, runErr := run(runCtx, path, spec.args...)
	rtts := parseICMPEchoTimes(output)
	if len(rtts) == 0 {
		if runErr != nil {
			return icmpEchoEvidence{err: fmt.Errorf("ping %s: %v: %s", spec.name, runErr, truncateTraceOutput(output))}
		}
		return icmpEchoEvidence{err: fmt.Errorf("ping %s produced no icmp echo replies: %s", spec.name, truncateTraceOutput(output))}
	}
	return icmpEchoEvidence{rtts: rtts, tool: spec.name}
}

type icmpCommandSpec struct {
	name string
	args []string
}

// icmpCommandSpecFor builds the OS-specific ping invocation for a resolved
// target IP. The IP is passed literally so family selection cannot drift from
// the TCP probe's resolution.
func icmpCommandSpecFor(ip string) icmpCommandSpec {
	count := strconv.Itoa(icmpEchoCount)
	family := traceIPFamily(ip)
	if runtime.GOOS == "windows" {
		return icmpCommandSpec{name: "ping", args: []string{"-n", count, "-w", strconv.Itoa(icmpEchoWindowsWaitMs), ip}}
	}
	if runtime.GOOS == "darwin" {
		name := "ping"
		if family == "v6" {
			name = "ping6"
		}
		return icmpCommandSpec{name: name, args: []string{"-c", count, "-t", strconv.Itoa(int(icmpEchoDarwinTimeout.Seconds())), ip}}
	}
	args := []string{"-c", count, "-w", strconv.Itoa(int(icmpEchoDeadline.Seconds()))}
	if family == "v6" {
		args = append(args, "-6")
	}
	args = append(args, ip)
	return icmpCommandSpec{name: "ping", args: args}
}

// parseICMPEchoTimes extracts per-reply RTTs from ping output. Only lines
// carrying a TTL marker count as echo replies, which skips summary lines
// (rtt min/avg/max/mdev, Windows round-trip averages) across locales.
func parseICMPEchoTimes(output []byte) []time.Duration {
	rtts := make([]time.Duration, 0, icmpEchoCount)
	for line := range strings.SplitSeq(string(output), "\n") {
		if !strings.Contains(strings.ToLower(line), "ttl=") {
			continue
		}
		match := pingReplyTimeRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		ms, err := strconv.ParseFloat(match[1], 64)
		if err != nil || ms < 0 {
			continue
		}
		rtts = append(rtts, time.Duration(ms*float64(time.Millisecond)))
	}
	return rtts
}

package netio

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

var defaultMailPorts = []int{25, 465, 587, 2525, 110, 143, 993, 995}

// DefaultMailPorts returns the built-in outbound mail-related port set.
func DefaultMailPorts() []int {
	return append([]int(nil), defaultMailPorts...)
}

// ProbeMailPorts checks outbound TCP reachability for common mail ports.
func ProbeMailPorts(ctx context.Context, ports []int) []PortProbe {
	if len(ports) == 0 {
		ports = DefaultMailPorts()
	}
	results := make([]PortProbe, len(ports))
	var wg sync.WaitGroup
	for i, port := range ports {
		wg.Add(1)
		go func(idx, p int) {
			defer wg.Done()
			results[idx] = probeMailPort(ctx, p)
		}(i, port)
	}
	wg.Wait()
	return results
}

func probeMailPort(ctx context.Context, port int) PortProbe {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	start := time.Now()
	conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("portquiz.net:%d", port))
	latency := time.Since(start)
	probe := PortProbe{
		Port:      port,
		Title:     mailPortTitle(port),
		Supported: true,
		Status:    "blocked",
		Target:    "portquiz.net",
		Method:    "tcp_connect",
		LatencyMs: latency.Seconds() * 1000,
	}
	if err != nil {
		probe.Message = err.Error()
		return probe
	}
	_ = conn.Close()
	probe.Status = "open"
	probe.Message = "reachable"
	return probe
}

func mailPortTitle(port int) string {
	switch port {
	case 25:
		return "SMTP 25"
	case 465:
		return "SMTPS 465"
	case 587:
		return "Submission 587"
	case 2525:
		return "SMTP 2525"
	case 110:
		return "POP3 110"
	case 143:
		return "IMAP 143"
	case 993:
		return "IMAPS 993"
	case 995:
		return "POP3S 995"
	default:
		return fmt.Sprintf("Port %d", port)
	}
}

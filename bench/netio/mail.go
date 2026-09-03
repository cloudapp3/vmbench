package netio

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

const (
	mailPortTarget  = "portquiz.net"
	mailPortTimeout = 5 * time.Second

	MailPortStatusOpen    = "open"
	MailPortStatusRefused = "refused"
	MailPortStatusTimeout = "timeout"
	MailPortStatusError   = "error"
)

var defaultMailPorts = []int{25, 465, 587, 2525, 110, 143, 993, 995}

type mailDialFunc func(context.Context, string, string) (net.Conn, error)

// DefaultMailPorts returns the built-in outbound mail-related port set.
func DefaultMailPorts() []int {
	return append([]int(nil), defaultMailPorts...)
}

// ProbeMailPorts checks outbound TCP reachability for common mail ports.
func ProbeMailPorts(ctx context.Context, ports []int) []PortProbe {
	dialer := &net.Dialer{Timeout: mailPortTimeout}
	return probeMailPorts(ctx, ports, mailPortTimeout, dialer.DialContext)
}

func probeMailPorts(ctx context.Context, ports []int, timeout time.Duration, dial mailDialFunc) []PortProbe {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(ports) == 0 {
		ports = DefaultMailPorts()
	}
	if timeout <= 0 {
		timeout = mailPortTimeout
	}
	results := make([]PortProbe, len(ports))
	// portquiz.net can reject bursts of simultaneous connections. Probe in input
	// order so one vmbench run never opens multiple connections to it at once.
	for i, port := range ports {
		results[i] = probeMailPort(ctx, port, timeout, dial)
	}
	return results
}

func probeMailPort(ctx context.Context, port int, timeout time.Duration, dial mailDialFunc) PortProbe {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	var conn net.Conn
	var err error
	if dial == nil {
		err = errors.New("TCP probe unavailable")
	} else {
		conn, err = dial(dialCtx, "tcp", fmt.Sprintf("%s:%d", mailPortTarget, port))
	}
	latency := time.Since(start)
	probe := PortProbe{
		Port:      port,
		Title:     mailPortTitle(port),
		Supported: true,
		Status:    mailPortStatus(dialCtx, err),
		Target:    mailPortTarget,
		Method:    "tcp_connect",
		LatencyMs: latency.Seconds() * 1000,
	}
	if err != nil {
		probe.Message = err.Error()
		return probe
	}
	_ = conn.Close()
	probe.Message = "reachable"
	return probe
}

func mailPortStatus(ctx context.Context, err error) string {
	if err == nil {
		return MailPortStatusOpen
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return MailPortStatusError
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return MailPortStatusRefused
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return MailPortStatusTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return MailPortStatusTimeout
	}
	return MailPortStatusError
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

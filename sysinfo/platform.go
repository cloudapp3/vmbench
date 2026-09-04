package sysinfo

import (
	"context"
	"strings"
)

// PlatformDiagnostics stores host-level runtime and kernel tuning evidence
// collected without extra dependencies: uptime/load/timezone/swap (gopsutil)
// plus /proc+/sys reads on Linux (virtio balloon, KSM, TCP congestion
// control and buffers, nested-virtualization CPU flags, HugePages, boot
// disk). Missing values stay zero/empty; unsupported platforms return an
// empty report instead of errors.
type PlatformDiagnostics struct {
	UptimeSeconds        uint64  `json:"uptime_seconds,omitempty"`
	Load1                float64 `json:"load_1,omitempty"`
	Load5                float64 `json:"load_5,omitempty"`
	Load15               float64 `json:"load_15,omitempty"`
	Timezone             string  `json:"timezone,omitempty"`
	SwapTotalBytes       uint64  `json:"swap_total_bytes,omitempty"`
	SwapUsedBytes        uint64  `json:"swap_used_bytes,omitempty"`
	VirtioBalloon        string  `json:"virtio_balloon,omitempty"` // present | absent | unsupported
	KSM                  string  `json:"ksm,omitempty"`            // enabled | disabled | unsupported
	KSMPagesShared       int64   `json:"ksm_pages_shared,omitempty"`
	TCPCongestion        string  `json:"tcp_congestion,omitempty"` // cubic, bbr, ...
	TCPQDisc             string  `json:"tcp_qdisc,omitempty"`      // fq, fq_codel, ...
	TCPRmemMin           int64   `json:"tcp_rmem_min,omitempty"`   // bytes
	TCPRmemDefault       int64   `json:"tcp_rmem_default,omitempty"`
	TCPRmemMax           int64   `json:"tcp_rmem_max,omitempty"`
	TCPWmemMin           int64   `json:"tcp_wmem_min,omitempty"`
	TCPWmemDefault       int64   `json:"tcp_wmem_default,omitempty"`
	TCPWmemMax           int64   `json:"tcp_wmem_max,omitempty"`
	NestedVirtualization string  `json:"nested_virtualization,omitempty"` // vmx | svm | masked | unknown
	HugePagesTotal       int64   `json:"hugepages_total,omitempty"`
	HugePagesFree        int64   `json:"hugepages_free,omitempty"`
	HugePageSizeBytes    int64   `json:"hugepage_size_bytes,omitempty"`
	BootDisk             string  `json:"boot_disk,omitempty"`
}

// collectPlatformDiagnostics gathers host runtime evidence across platforms.
func collectPlatformDiagnostics(ctx context.Context) PlatformDiagnostics {
	if ctx == nil {
		ctx = context.Background()
	}
	diagnostics := collectHostBasics(ctx)
	diagnostics.merge(collectLinuxKernelTuning())
	return diagnostics
}

// merge fills empty fields from another partial report (Linux-only evidence).
func (d *PlatformDiagnostics) merge(other PlatformDiagnostics) {
	if d.UptimeSeconds == 0 {
		d.UptimeSeconds = other.UptimeSeconds
	}
	if d.Load1 == 0 {
		d.Load1, d.Load5, d.Load15 = other.Load1, other.Load5, other.Load15
	}
	if d.Timezone == "" {
		d.Timezone = other.Timezone
	}
	if d.SwapTotalBytes == 0 {
		d.SwapTotalBytes, d.SwapUsedBytes = other.SwapTotalBytes, other.SwapUsedBytes
	}
	if d.VirtioBalloon == "" {
		d.VirtioBalloon = other.VirtioBalloon
	}
	if d.KSM == "" {
		d.KSM, d.KSMPagesShared = other.KSM, other.KSMPagesShared
	}
	if d.TCPCongestion == "" {
		d.TCPCongestion, d.TCPQDisc = other.TCPCongestion, other.TCPQDisc
	}
	if d.TCPRmemMax == 0 {
		d.TCPRmemMin, d.TCPRmemDefault, d.TCPRmemMax = other.TCPRmemMin, other.TCPRmemDefault, other.TCPRmemMax
	}
	if d.TCPWmemMax == 0 {
		d.TCPWmemMin, d.TCPWmemDefault, d.TCPWmemMax = other.TCPWmemMin, other.TCPWmemDefault, other.TCPWmemMax
	}
	if d.NestedVirtualization == "" {
		d.NestedVirtualization = other.NestedVirtualization
	}
	if d.HugePagesTotal == 0 {
		d.HugePagesTotal, d.HugePagesFree, d.HugePageSizeBytes = other.HugePagesTotal, other.HugePagesFree, other.HugePageSizeBytes
	}
	if d.BootDisk == "" {
		d.BootDisk = other.BootDisk
	}
}

// normalize trims and canonicalizes display strings.
func (d *PlatformDiagnostics) normalize() {
	d.Timezone = strings.TrimSpace(d.Timezone)
	d.VirtioBalloon = strings.TrimSpace(d.VirtioBalloon)
	d.KSM = strings.TrimSpace(d.KSM)
	d.TCPCongestion = strings.TrimSpace(d.TCPCongestion)
	d.TCPQDisc = strings.TrimSpace(d.TCPQDisc)
	d.NestedVirtualization = strings.TrimSpace(d.NestedVirtualization)
	d.BootDisk = strings.TrimSpace(d.BootDisk)
}

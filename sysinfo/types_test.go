package sysinfo

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCollectHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, warnings := Collect(ctx)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Collect took %s with canceled context", elapsed)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "canceled") {
		t.Fatalf("warnings = %v, want cancellation warning", warnings)
	}
}

func TestVirtualizationInfoJSONFields(t *testing.T) {
	info := SystemInfo{Virtualization: VirtualizationInfo{System: "kvm", Role: "guest"}}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"virtualization":{"system":"kvm","role":"guest"}`) {
		t.Fatalf("JSON = %s, want virtualization system and role", got)
	}
}

func TestCollectVirtualizationInfoMergesLocalFallback(t *testing.T) {
	primary := func(context.Context) (string, string, error) {
		return "", "guest", nil
	}
	fallback := func(context.Context) VirtualizationInfo {
		return VirtualizationInfo{System: "kvm", Role: "guest"}
	}
	info, warnings := collectVirtualizationInfoWith(context.Background(), primary, fallback)
	if info.System != "kvm" || info.Role != "guest" {
		t.Fatalf("virtualization = %+v, want kvm guest", info)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

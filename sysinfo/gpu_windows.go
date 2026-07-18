//go:build windows

package sysinfo

import (
	"context"
	"encoding/json"
	"strings"
)

type winGPUProbe struct {
	Name          string `json:"Name"`
	AdapterRAM    uint64 `json:"AdapterRAM"`
	DriverVersion string `json:"DriverVersion"`
}

func collectGPUInfo(ctx context.Context) (*GPUInfo, []string) {
	text, err := runCommand(ctx, "powershell", "-NoProfile", "-Command", "Get-CimInstance Win32_VideoController | Select-Object -First 1 Name,AdapterRAM,DriverVersion | ConvertTo-Json -Compress")
	if err != nil || strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var probe winGPUProbe
	if err := json.Unmarshal([]byte(text), &probe); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(probe.Name) == "" {
		return nil, nil
	}
	return &GPUInfo{Model: probe.Name, VRAM: probe.AdapterRAM, Driver: probe.DriverVersion}, nil
}

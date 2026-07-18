package sysinfo

import (
	"strconv"
	"strings"
)

func parseVRAMBytes(text string) uint64 {
	text = strings.TrimSpace(strings.ToUpper(text))
	if text == "" {
		return 0
	}
	multiplier := uint64(1)
	switch {
	case strings.Contains(text, "GB"):
		multiplier = 1 << 30
	case strings.Contains(text, "MB"):
		multiplier = 1 << 20
	case strings.Contains(text, "KB"):
		multiplier = 1 << 10
	}
	value := firstNumber(text)
	if value <= 0 {
		return 0
	}
	return uint64(value) * multiplier
}

func pickNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func formatUint(value uint64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatUint(value, 10)
}

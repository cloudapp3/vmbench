package sysinfo

import (
	"regexp"
	"strconv"
	"strings"
)

var firstNumberPattern = regexp.MustCompile(`(\d+)`)

func firstNumber(text string) int {
	match := firstNumberPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return value
}

func parseMemoryType(text string) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	switch {
	case strings.Contains(text, "LPDDR5"):
		return "LPDDR5"
	case strings.Contains(text, "LPDDR4"):
		return "LPDDR4"
	case strings.Contains(text, "DDR5"):
		return "DDR5"
	case strings.Contains(text, "DDR4"):
		return "DDR4"
	case strings.Contains(text, "DDR3"):
		return "DDR3"
	default:
		return ""
	}
}

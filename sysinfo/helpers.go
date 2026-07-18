package sysinfo

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", err
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	out := strings.Split(text, "\n")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

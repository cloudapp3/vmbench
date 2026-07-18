package netio

import (
	"context"
	"runtime"
	"slices"
	"strconv"
	"testing"
)

func TestSystemTracerouteTargetUsesCatalogPort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tracert does not support a destination port")
	}

	const port = 5353
	lookPath := func(name string) (string, error) {
		return "/test/" + name, nil
	}
	var command string
	var args []string
	run := func(_ context.Context, path string, values ...string) ([]byte, error) {
		command = path
		args = append([]string(nil), values...)
		return []byte(" 1  192.0.2.1  1.0 ms\n"), nil
	}

	_, err := systemTracerouteWithPort(context.Background(), "192.0.2.10", port, lookPath, run)
	if err != nil {
		t.Fatalf("systemTracerouteWithPort() error = %v", err)
	}
	if command == "" || !slices.Contains(args, strconv.Itoa(port)) {
		t.Fatalf("command = %q args = %v, want catalog port %d", command, args, port)
	}
}

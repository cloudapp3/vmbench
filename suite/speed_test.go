package suite

import (
	"context"
	"math"
	"testing"
)

func TestMebibytesPerSecondToMegabitsPerSecond(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  float64
	}{
		{name: "zero", value: 0, want: 0},
		{name: "one MiB per second", value: 1, want: 8.388608},
		{name: "one hundred MiB per second", value: 100, want: 838.8608},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mebibytesPerSecondToMegabitsPerSecond(tt.value)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("conversion = %.12f, want %.12f", got, tt.want)
			}
		})
	}
}

func TestBuildIperfSpeedGroupRequiresHost(t *testing.T) {
	for _, hosts := range [][]string{nil, {"", "  "}} {
		group := buildIperfSpeedGroup(context.Background(), hosts)
		if group.Status != "error" {
			t.Fatalf("buildIperfSpeedGroup(%q).Status = %q, want error", hosts, group.Status)
		}
		if group.Available != 0 || group.Failed != 1 {
			t.Fatalf("buildIperfSpeedGroup(%q) counts = %d ok/%d failed, want 0/1", hosts, group.Available, group.Failed)
		}
		if len(group.Providers) != 1 || group.Providers[0].Status != "error" {
			t.Fatalf("buildIperfSpeedGroup(%q).Providers = %+v, want one error result", hosts, group.Providers)
		}
	}
}

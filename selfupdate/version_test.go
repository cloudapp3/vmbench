package selfupdate

import (
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.6.0", "0.6.0", 0},
		{"v0.6.0", "0.6.0", 0},
		{"0.6.0", "0.5.9", 1},
		{"0.5.9", "0.6.0", -1},
		{"0.6.0", "0.10.0", -1},
		{"0.10.0", "0.6.0", 1},
		{"0.6", "0.6.0", 0},
		{"0.6.1", "0.6", 1},
		{"0.7.0", "0.7.0-rc1", 1},
		{"0.7.0-rc1", "0.7.0", -1},
		{"dev", "0.1.0", -1},
		{"0.1.0", "dev", 1},
		{"dev", "dev", 0},
		{"", "dev", 0},
		{" 0.6.0 ", "0.6.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizeTag(t *testing.T) {
	if got, err := NormalizeTag("0.6.0"); err != nil || got != "v0.6.0" {
		t.Errorf("NormalizeTag(\"0.6.0\") = %q, %v; want v0.6.0, nil", got, err)
	}
	if got, err := NormalizeTag("v0.6.0"); err != nil || got != "v0.6.0" {
		t.Errorf("NormalizeTag(\"v0.6.0\") = %q, %v; want v0.6.0, nil", got, err)
	}
	if got, err := NormalizeTag(" 0.6.0 "); err != nil || got != "v0.6.0" {
		t.Errorf("NormalizeTag(\" 0.6.0 \") = %q, %v; want v0.6.0, nil", got, err)
	}
	if _, err := NormalizeTag(""); err == nil {
		t.Error("NormalizeTag(\"\") should fail")
	}
}

func TestIsDevBuild(t *testing.T) {
	for _, v := range []string{"", "dev", " dev "} {
		if !IsDevBuild(v) {
			t.Errorf("IsDevBuild(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0.6.0", "deploy-test"} {
		if IsDevBuild(v) {
			t.Errorf("IsDevBuild(%q) = true, want false", v)
		}
	}
}

func TestAssetName(t *testing.T) {
	cases := []struct {
		ver, goos, goarch, want string
	}{
		{"0.6.0", "linux", "amd64", "vmbench-0.6.0-linux-amd64.tar.gz"},
		{"0.6.0", "darwin", "arm64", "vmbench-0.6.0-darwin-arm64.tar.gz"},
		{"0.6.0", "windows", "amd64", "vmbench-0.6.0-windows-amd64.zip"},
	}
	for _, c := range cases {
		if got := AssetName(c.ver, c.goos, c.goarch); got != c.want {
			t.Errorf("AssetName(%q, %q, %q) = %q, want %q", c.ver, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestAssetFor(t *testing.T) {
	release := Release{
		Tag: "v0.6.0",
		Ver: "0.6.0",
		Assets: []Asset{
			{Name: "vmbench-0.6.0-linux-amd64.tar.gz", URL: "u1"},
			{Name: "checksums.txt", URL: "u2"},
		},
	}
	asset, err := AssetFor(release, "linux", "amd64")
	if err != nil {
		t.Fatalf("AssetFor: %v", err)
	}
	if asset.Name != "vmbench-0.6.0-linux-amd64.tar.gz" || asset.URL != "u1" {
		t.Errorf("AssetFor returned %+v", asset)
	}
	_, err = AssetFor(release, "windows", "arm64")
	if err == nil {
		t.Fatal("AssetFor for unbuilt platform should fail")
	}
	for _, want := range []string{"vmbench-0.6.0-windows-arm64.zip", "vmbench-0.6.0-linux-amd64.tar.gz"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("AssetFor error %q missing %q", err.Error(), want)
		}
	}
}

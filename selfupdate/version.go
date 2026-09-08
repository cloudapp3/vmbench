package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// binaryName is the base name of the published executable inside release
// archives and of the installed binary itself.
const binaryName = "vmbench"

// CompareVersions compares two dotted release versions, returning -1 when a
// is older than b, 0 when equal, and 1 when newer. Leading "v" prefixes are
// ignored, missing components default to 0, and a side with any non-numeric
// component (for example "dev" or "0.7.0-rc1") compares as older unless both
// sides are unparseable, in which case they compare equal.
func CompareVersions(a, b string) int {
	numbersA, parsedA := parseVersion(a)
	numbersB, parsedB := parseVersion(b)
	switch {
	case !parsedA && !parsedB:
		return 0
	case !parsedA:
		return -1
	case !parsedB:
		return 1
	}
	for i := 0; i < len(numbersA) || i < len(numbersB); i++ {
		x, y := 0, 0
		if i < len(numbersA) {
			x = numbersA[i]
		}
		if i < len(numbersB) {
			y = numbersB[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// NormalizeTag returns v with a leading "v", accepting both the "v0.6.0" and
// "0.6.0" spellings.
func NormalizeTag(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("release version is required")
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v, nil
}

// IsDevBuild reports whether the version identifies a non-release build that
// should always be considered older than any published release.
func IsDevBuild(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == "dev"
}

// AssetName returns the release archive file name for a version. Windows
// releases are zip archives; every other platform is tar.gz.
func AssetName(ver, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%s-%s-%s-%s%s", binaryName, ver, goos, goarch, ext)
}

// AssetFor finds the release archive matching goos/goarch.
func AssetFor(release Release, goos, goarch string) (Asset, error) {
	name := AssetName(strings.TrimPrefix(release.Ver, "v"), goos, goarch)
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	names := make([]string, 0, len(release.Assets))
	for _, asset := range release.Assets {
		names = append(names, asset.Name)
	}
	return Asset{}, fmt.Errorf("release %s has no asset %s (available: %s)", release.Tag, name, strings.Join(names, ", "))
}

func parseVersion(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		numbers = append(numbers, n)
	}
	return numbers, true
}

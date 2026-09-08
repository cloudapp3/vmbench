// Package selfupdate upgrades the running vmbench binary from GitHub
// Releases: discover the latest release for the current platform, download
// the archive, verify its SHA-256 against the release checksums.txt, and
// atomically replace the executable. Standard library only; the trust root
// is checksums.txt over TLS, the same model install.sh uses.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// DefaultAPIBase is the GitHub API base of the vmbench repository.
	DefaultAPIBase = "https://api.github.com/repos/cloudapp3/vmbench"
	// DefaultMaxArchiveBytes caps release archive downloads.
	DefaultMaxArchiveBytes = 256 << 20
)

// maxChecksumsBytes caps the checksums.txt download.
const maxChecksumsBytes = 1 << 20

// ErrTargetNotWritable reports that the destination binary could not be
// replaced, typically because it is owned by a package manager or root.
var ErrTargetNotWritable = errors.New("target binary is not writable")

// Options configures Check and Install. Zero fields take platform defaults.
type Options struct {
	// APIBase defaults to DefaultAPIBase and must be the
	// ".../repos/OWNER/REPO" prefix of a GitHub-compatible releases API.
	APIBase string
	// Tag pins a specific release ("v0.6.0" or "0.6.0"); empty means latest.
	Tag string
	// Current is the running build version (vmbench.Version).
	Current string
	// GOOS and GOARCH default to runtime values; overridable for tests and
	// cross-platform installs.
	GOOS, GOARCH string
	// Dest defaults to the running executable (symlinks resolved).
	Dest string
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// Token, when set, is sent as a GitHub bearer token.
	Token string
	// MaxArchiveBytes defaults to DefaultMaxArchiveBytes.
	MaxArchiveBytes int64
}

// Asset is one downloadable release file.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Release describes a GitHub release. Tag keeps its "v" prefix, Ver does not.
type Release struct {
	Tag    string
	Ver    string
	Assets []Asset
}

// Result reports a completed install.
type Result struct {
	OldVersion string
	NewVersion string
	Path       string
	AssetName  string
	Bytes      int64
}

// Check fetches release metadata for the latest release, or the pinned tag
// when options.Tag is set.
func Check(ctx context.Context, options Options) (Release, error) {
	apiBase := strings.TrimSuffix(strings.TrimSpace(options.APIBase), "/")
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	path := "/releases/latest"
	pinned := ""
	if strings.TrimSpace(options.Tag) != "" {
		tag, err := NormalizeTag(options.Tag)
		if err != nil {
			return Release{}, err
		}
		pinned = tag
		path = "/releases/tags/" + tag
	}
	data, err := fetch(ctx, options, apiBase+path, 4<<20)
	if err != nil {
		if pinned != "" {
			return Release{}, fmt.Errorf("query release %s: %w", pinned, err)
		}
		return Release{}, fmt.Errorf("query latest release: %w", err)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Release{}, fmt.Errorf("decode release metadata: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("release metadata has no tag")
	}
	if payload.Draft {
		return Release{}, fmt.Errorf("release %s is a draft", payload.TagName)
	}
	if pinned != "" && payload.TagName != pinned {
		return Release{}, fmt.Errorf("release metadata returned tag %s, want %s", payload.TagName, pinned)
	}
	release := Release{Tag: payload.TagName, Ver: strings.TrimPrefix(payload.TagName, "v")}
	for _, asset := range payload.Assets {
		release.Assets = append(release.Assets, Asset{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
	}
	return release, nil
}

// Install downloads, verifies, extracts, and atomically installs the release
// binary, replacing the destination file.
func Install(ctx context.Context, options Options, release Release) (Result, error) {
	goos, goarch := options.GOOS, options.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	maxBytes := options.MaxArchiveBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxArchiveBytes
	}
	dest := strings.TrimSpace(options.Dest)
	if dest == "" {
		executable, err := os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("locate running binary: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolved
		}
		dest = executable
	}

	asset, err := AssetFor(release, goos, goarch)
	if err != nil {
		return Result{}, err
	}
	wantSum, err := releaseChecksum(ctx, options, release, asset.Name)
	if err != nil {
		return Result{}, err
	}

	archivePath, gotBytes, err := downloadArchive(ctx, options, asset, wantSum, maxBytes)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.Remove(archivePath) }()

	mode := os.FileMode(0o755)
	if info, err := os.Stat(dest); err == nil {
		mode = info.Mode().Perm()
	}
	staged, err := extractBinary(archivePath, filepath.Dir(dest), mode, goos)
	if err != nil {
		// The staging temp file lives next to dest, so an unwritable install
		// directory surfaces here rather than in replaceBinary.
		return Result{}, wrapReplaceError(err)
	}
	defer func() { _ = os.Remove(staged) }()
	if err := replaceBinary(staged, dest, goos); err != nil {
		return Result{}, err
	}
	return Result{
		OldVersion: options.Current,
		NewVersion: release.Ver,
		Path:       dest,
		AssetName:  asset.Name,
		Bytes:      gotBytes,
	}, nil
}

// releaseChecksum fetches checksums.txt and returns the expected SHA-256 hex
// digest for the named asset.
func releaseChecksum(ctx context.Context, options Options, release Release, assetName string) (string, error) {
	var checksumAsset *Asset
	for i := range release.Assets {
		if release.Assets[i].Name == "checksums.txt" {
			checksumAsset = &release.Assets[i]
			break
		}
	}
	if checksumAsset == nil {
		return "", fmt.Errorf("release %s has no checksums.txt asset", release.Tag)
	}
	data, err := fetch(ctx, options, checksumAsset.URL, maxChecksumsBytes)
	if err != nil {
		return "", fmt.Errorf("download checksums.txt: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", assetName)
}

// downloadArchive streams the asset into a temp file while hashing it. The
// temp file is removed by the caller.
func downloadArchive(ctx context.Context, options Options, asset Asset, wantSum string, maxBytes int64) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", 0, err
	}
	applyHeaders(req, options)
	resp, err := options.client().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return "", 0, fmt.Errorf("download %s: HTTP %d", asset.Name, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "vmbench-update-*.archive")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", 0, fmt.Errorf("download %s: %w", asset.Name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	if written > maxBytes {
		_ = os.Remove(tmpName)
		return "", 0, fmt.Errorf("download %s: exceeds %d bytes", asset.Name, maxBytes)
	}
	gotSum := hex.EncodeToString(hash.Sum(nil))
	if gotSum != wantSum {
		_ = os.Remove(tmpName)
		return "", 0, fmt.Errorf("checksum mismatch for %s: got %s, want %s", asset.Name, gotSum, wantSum)
	}
	return tmpName, written, nil
}

// fetch issues a bounded GET, mirroring nodecatalog's fetch idiom.
func fetch(ctx context.Context, options Options, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req, options)
	resp, err := options.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

// applyHeaders sets the request identity and, when configured, the GitHub
// token headers install.sh also sends.
func applyHeaders(req *http.Request, options Options) {
	version := strings.TrimSpace(options.Current)
	if version == "" {
		version = "dev"
	}
	req.Header.Set("User-Agent", binaryName+"/"+version)
	if options.Token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+options.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func (o Options) client() *http.Client {
	if o.Client == nil {
		return http.DefaultClient
	}
	return o.Client
}

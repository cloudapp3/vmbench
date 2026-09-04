package netio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// IP quality evidence sources.
const (
	IPSourceBuiltin       = "builtin"
	IPSourceSecurityCheck = "securitycheck"
	scRawOutputLimit      = 16 << 10
	scRunTimeout          = 90 * time.Second
)

// scBinaryBaseCandidates is swappable for tests.
var scBinaryBaseCandidates = "securitycheck:securityCheck:sc"

// SecurityCheckResult stores output of the optional oneclickvirt/securityCheck
// binary (closed source, opt-in). The 18-database view is preserved as raw
// text evidence; only well-known score lines are extracted.
type SecurityCheckResult struct {
	Source   string        `json:"source"`
	Status   string        `json:"status"` // ok | unavailable | error
	Binary   string        `json:"binary,omitempty"`
	Message  string        `json:"message,omitempty"`
	Fields   []SCField     `json:"fields,omitempty"`
	Raw      string        `json:"raw,omitempty"`
	Duration time.Duration `json:"duration_ms,omitempty"`
}

// SCField is one extracted "name: value" line from securityCheck output.
type SCField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// securityCheckBinary resolves the opt-in external tool from PATH or the
// executable-adjacent binaries/ directory, mirroring the catalog adapter
// layout. netio cannot import catalog (import cycle), so the lookup is local.
func securityCheckBinary() (string, error) {
	for _, name := range strings.Split(scBinaryBaseCandidates, ":") {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
		if path, ok := localNetioTool(name); ok {
			return path, nil
		}
	}
	return "", fmt.Errorf("no securityCheck binary found in PATH or executable-adjacent binaries/ directory (tried %s); install it from github.com/oneclickvirt/securityCheck releases", scBinaryBaseCandidates)
}

func localNetioTool(name string) (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", false
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	candidates := []string{
		filepath.Join(filepath.Dir(executable), "binaries", fmt.Sprintf("%s_%s", name, arch)),
		filepath.Join(filepath.Dir(executable), fmt.Sprintf("%s_%s", name, arch)),
		filepath.Join(filepath.Dir(executable), "binaries", name),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

// ansiEscapeRe strips color/control sequences from securityCheck output.
var ansiEscapeRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

var scFieldRe = regexp.MustCompile(`^\s*([^\s:：][^:：]{0,40})\s*[:：]\s*(\S.*)$`)

// scInterestingField reports whether an extracted field is worth surfacing
// as structured evidence (score lines, IP usage type, abuse ratings).
func scInterestingField(name string) bool {
	lower := strings.ToLower(name)
	keywords := []string{
		"得分", "分数", "类型", "威胁", "滥用", "声誉", "欺诈", "信任", "级别", "黑名单", "记录", "代理",
		"score", "type", "threat", "abuse", "reputation", "fraud", "trust", "vpn", "proxy",
		"level", "blacklist", "records", "usage", "company", "cloud", "datacenter", "mobile",
		"tor", "crawler", "anonymous", "attacker", "traffic", "ratio", "browser", "device",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// runSecurityCheck executes the optional external securityCheck binary for
// the current host IP. Missing binaries surface as status=unavailable.
func runSecurityCheck(ctx context.Context) *SecurityCheckResult {
	result := &SecurityCheckResult{Source: IPSourceSecurityCheck}
	binary, err := securityCheckBinary()
	if err != nil {
		result.Status = "unavailable"
		result.Message = err.Error()
		return result
	}
	result.Binary = binary

	runCtx, cancel := context.WithTimeout(ctx, scRunTimeout)
	defer cancel()
	started := time.Now()
	// -c ipv4 keeps the external view aligned with the v4-first probe;
	// -l en normalizes field names for extraction; -e yes prints IP info.
	output, err := exec.CommandContext(runCtx, binary, "-c", "ipv4", "-e", "yes", "-l", "en").CombinedOutput()
	result.Duration = time.Since(started)
	if runCtx.Err() != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("securityCheck timed out after %s", scRunTimeout)
		return result
	}
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("securityCheck exited with error: %v", err)
		result.Raw = trimSCRaw(output)
		return result
	}
	result.Status = "ok"
	result.Raw = trimSCRaw(output)
	for _, line := range strings.Split(result.Raw, "\n") {
		match := scFieldRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if len(match) != 3 {
			continue
		}
		name := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])
		if name == "" || value == "" {
			continue
		}
		if scInterestingField(name) {
			result.Fields = append(result.Fields, SCField{Name: name, Value: value})
		}
	}
	if len(result.Fields) == 0 {
		result.Message = "no recognizable score fields; raw output preserved"
	}
	return result
}

func trimSCRaw(output []byte) string {
	cleaned := ansiEscapeRe.ReplaceAllString(string(output), "")
	if len(cleaned) > scRawOutputLimit {
		cleaned = cleaned[:scRawOutputLimit] + "\n... (truncated)"
	}
	return strings.TrimSpace(cleaned)
}

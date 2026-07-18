package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/sysinfo"
)

const (
	protocolVersion = "2025-06-18"
	serverName      = "vmbench"
	maxIterations   = 9
	maxTimeout      = 15 * time.Minute
)

// Server exposes vmbench as a minimal MCP server over JSON-RPC.
type Server struct {
	out io.Writer
	err io.Writer

	mu      sync.Mutex
	running bool
}

// NewServer creates a vmbench MCP server.
func NewServer(stdout, stderr io.Writer) *Server {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Server{out: stdout, err: stderr}
}

// ServeStdio serves MCP JSON-RPC messages from r and writes responses to w.
func ServeStdio(ctx context.Context, r io.Reader, w io.Writer, stderr io.Writer) error {
	return NewServer(w, stderr).Serve(ctx, r)
}

// Serve reads newline-delimited JSON-RPC requests until EOF or ctx cancel.
func (s *Server) Serve(ctx context.Context, r io.Reader) error {
	if s.out == nil {
		return errors.New("mcp stdout writer is nil")
	}
	if r == nil {
		return errors.New("mcp stdin reader is nil")
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.handleLine(ctx, []byte(line))
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolSpec struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResponse(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
		return
	}
	if len(req.ID) == 0 {
		// JSON-RPC notification. MCP initialized notifications require no response.
		return
	}
	if req.JSONRPC != "2.0" {
		s.writeError(req.ID, -32600, "invalid JSON-RPC version")
		return
	}

	result, rpcErr := s.dispatch(ctx, req)
	if rpcErr != nil {
		s.writeResponse(response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
		return
	}
	s.writeResponse(response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initializeResult(), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolSpecs()}, nil
	case "tools/call":
		var params toolCallParams
		if err := decodeArgs(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		res, err := s.callTool(ctx, params.Name, params.Arguments)
		if err != nil {
			return nil, &rpcError{Code: -32603, Message: err.Error()}
		}
		return res, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    serverName,
			"version": vmbench.Version,
		},
	}
}

func (s *Server) callTool(ctx context.Context, name string, raw json.RawMessage) (toolResult, error) {
	switch strings.TrimSpace(name) {
	case "vmbench_capabilities":
		return okToolResult("vmbench capabilities", capabilitiesPayload()), nil
	case "vmbench_sysinfo":
		return s.toolSysinfo(ctx, raw)
	case "vmbench_run":
		return s.toolRun(ctx, raw)
	case "vmbench_suite":
		return s.toolSuite(ctx, raw)
	default:
		return errorToolResult("unknown tool: " + name), nil
	}
}

func (s *Server) toolSysinfo(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args struct{}
	if err := decodeToolArgs(raw, &args); err != nil {
		return errorToolResult(err.Error()), nil
	}
	info, warnings := sysinfo.Collect(ctx)
	payload := map[string]any{
		"system":   info,
		"warnings": warnings,
	}
	return okToolResult(formatSysinfoSummary(info, warnings), payload), nil
}

type runArgs struct {
	Iterations       json.RawMessage `json:"iterations,omitempty"`
	Filter           string          `json:"filter,omitempty"`
	Mode             string          `json:"mode,omitempty"`
	Scope            string          `json:"scope,omitempty"`
	DiskPath         string          `json:"disk_path,omitempty"`
	TimeoutMS        json.RawMessage `json:"timeout_ms,omitempty"`
	HardwareTools    []string        `json:"hardware_tools,omitempty"`
	IperfHosts       []string        `json:"iperf_hosts,omitempty"`
	CatalogSource    string          `json:"catalog_source,omitempty"`
	CatalogRevision  string          `json:"catalog_revision,omitempty"`
	CatalogCachePath string          `json:"catalog_cache_path,omitempty"`
}

func (s *Server) toolRun(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args runArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return errorToolResult(err.Error()), nil
	}
	norm, warnings := normalizeRunArgs(args)
	if len(warnings) > 0 {
		return errorToolResult(strings.Join(warnings, "; ")), nil
	}
	if ok := s.tryAcquireRun(); !ok {
		return errorToolResult("another vmbench benchmark is already running"), nil
	}
	defer s.releaseRun()

	runCtx, cancel := context.WithTimeout(ctx, norm.Timeout)
	defer cancel()
	report := vmbench.RunCore(runCtx, vmbench.Options{
		DiskPath:         norm.DiskPath,
		Timeout:          norm.Timeout,
		Iterations:       norm.Iterations,
		Filter:           norm.Filter,
		Mode:             norm.Mode,
		Engine:           "external",
		Scope:            norm.Scope,
		IperfHosts:       norm.IperfHosts,
		HardwareTools:    norm.HardwareTools,
		CatalogSource:    norm.CatalogSource,
		CatalogRevision:  norm.CatalogRevision,
		CatalogCachePath: norm.CatalogCachePath,
		ResolvedCatalog:  norm.ResolvedCatalog,
		CatalogWarning:   norm.CatalogWarning,
	})
	payload := map[string]any{"report": report}
	result := okToolResult(formatRunSummary(report), payload)
	result.IsError = gbreport.HasFailures(report)
	return result, nil
}

type normalizedRunArgs struct {
	Iterations       int
	Filter           string
	Mode             string
	Scope            string
	DiskPath         string
	Timeout          time.Duration
	HardwareTools    []string
	IperfHosts       []string
	CatalogSource    string
	CatalogRevision  string
	CatalogCachePath string
	ResolvedCatalog  *nodecatalog.Manifest
	CatalogWarning   string
}

func normalizeRunArgs(args runArgs) (normalizedRunArgs, []string) {
	warnings := make([]string, 0, 4)
	iterations, iterationError := normalizeIterations(args.Iterations)
	appendValidationError(&warnings, iterationError)
	timeout, timeoutError := normalizeTimeoutMillis(args.TimeoutMS, 5*time.Minute)
	appendValidationError(&warnings, timeoutError)
	norm, err := vmbench.NormalizeOptions(vmbench.Options{
		Iterations:       iterations,
		Filter:           strings.TrimSpace(args.Filter),
		Mode:             strings.TrimSpace(args.Mode),
		Engine:           "external",
		Scope:            strings.TrimSpace(args.Scope),
		DiskPath:         strings.TrimSpace(args.DiskPath),
		Timeout:          timeout,
		HardwareTools:    cleanList(args.HardwareTools),
		IperfHosts:       cleanList(args.IperfHosts),
		CatalogSource:    strings.TrimSpace(args.CatalogSource),
		CatalogRevision:  strings.TrimSpace(args.CatalogRevision),
		CatalogCachePath: strings.TrimSpace(args.CatalogCachePath),
	})
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	return normalizedRunArgs{
		Iterations:       norm.Iterations,
		Filter:           norm.Filter,
		Mode:             norm.Mode,
		Scope:            norm.Scope,
		DiskPath:         norm.DiskPath,
		Timeout:          norm.Timeout,
		HardwareTools:    norm.HardwareTools,
		IperfHosts:       norm.IperfHosts,
		CatalogSource:    norm.CatalogSource,
		CatalogRevision:  norm.CatalogRevision,
		CatalogCachePath: norm.CatalogCachePath,
		ResolvedCatalog:  norm.ResolvedCatalog,
		CatalogWarning:   norm.CatalogWarning,
	}, warnings
}

type suiteArgs struct {
	Iterations       json.RawMessage `json:"iterations,omitempty"`
	Filter           string          `json:"filter,omitempty"`
	DiskPath         string          `json:"disk_path,omitempty"`
	TimeoutMS        json.RawMessage `json:"timeout_ms,omitempty"`
	Preset           string          `json:"preset,omitempty"`
	Only             []string        `json:"only,omitempty"`
	Skip             []string        `json:"skip,omitempty"`
	RoutePresets     []string        `json:"route_presets,omitempty"`
	SpeedProviders   []string        `json:"speed_providers,omitempty"`
	HardwareTools    []string        `json:"hardware_tools,omitempty"`
	IperfHosts       []string        `json:"iperf_hosts,omitempty"`
	IPVersion        string          `json:"ip_version,omitempty"`
	CatalogSource    string          `json:"catalog_source,omitempty"`
	CatalogRevision  string          `json:"catalog_revision,omitempty"`
	CatalogCachePath string          `json:"catalog_cache_path,omitempty"`
}

func (s *Server) toolSuite(ctx context.Context, raw json.RawMessage) (toolResult, error) {
	var args suiteArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return errorToolResult(err.Error()), nil
	}
	opts, warnings := normalizeSuiteArgs(args)
	if len(warnings) > 0 {
		return errorToolResult(strings.Join(warnings, "; ")), nil
	}
	if ok := s.tryAcquireRun(); !ok {
		return errorToolResult("another vmbench benchmark is already running"), nil
	}
	defer s.releaseRun()

	report := suite.Run(ctx, opts)
	payload := map[string]any{"report": report}
	result := okToolResult(formatSuiteSummary(report), payload)
	result.IsError = report.HasFailures()
	return result, nil
}

func normalizeSuiteArgs(args suiteArgs) (suite.Options, []string) {
	warnings := make([]string, 0, 6)
	iterations, iterationError := normalizeIterations(args.Iterations)
	appendValidationError(&warnings, iterationError)
	timeout, timeoutError := normalizeTimeoutMillis(args.TimeoutMS, 5*time.Minute)
	appendValidationError(&warnings, timeoutError)

	preset := strings.ToLower(strings.TrimSpace(args.Preset))
	sections := suite.SectionSelector{Hardware: true}
	if preset != "" {
		spec, ok := suite.LookupPreset(preset)
		if !ok {
			warnings = append(warnings, "unknown preset: "+preset+"; available: "+strings.Join(suite.PresetIDs(), ", "))
		} else {
			sections = spec.Sections
		}
	}
	if len(args.Only) > 0 {
		sections = selectorFromNames(args.Only, true, &warnings)
	}
	if len(args.Skip) > 0 {
		sections = applySectionNames(sections, args.Skip, false, &warnings)
	}
	if !sections.AnyEnabled() {
		warnings = append(warnings, "at least one suite section must remain enabled")
	}

	norm, err := suite.NormalizeOptions(suite.Options{
		Iterations:       iterations,
		Filter:           strings.TrimSpace(args.Filter),
		DiskPath:         strings.TrimSpace(args.DiskPath),
		Timeout:          timeout,
		Preset:           preset,
		RoutePresets:     cleanList(args.RoutePresets),
		SpeedProviders:   cleanList(args.SpeedProviders),
		HardwareTools:    cleanList(args.HardwareTools),
		Sections:         sections,
		IperfHosts:       cleanList(args.IperfHosts),
		IPVersion:        strings.TrimSpace(args.IPVersion),
		CatalogSource:    strings.TrimSpace(args.CatalogSource),
		CatalogRevision:  strings.TrimSpace(args.CatalogRevision),
		CatalogCachePath: strings.TrimSpace(args.CatalogCachePath),
	})
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	return norm, warnings
}

func selectorFromNames(names []string, enable bool, warnings *[]string) suite.SectionSelector {
	return applySectionNames(suite.SectionSelector{}, names, enable, warnings)
}

func applySectionNames(base suite.SectionSelector, names []string, enable bool, warnings *[]string) suite.SectionSelector {
	sections, err := suite.ApplySectionNames(base, names, enable)
	if err != nil {
		*warnings = append(*warnings, err.Error())
		return base
	}
	return sections
}

func normalizeSectionName(raw string) string {
	return suite.NormalizeSectionName(raw)
}

func (s *Server) tryAcquireRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *Server) releaseRun() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *Server) writeError(id json.RawMessage, code int, msg string) {
	s.writeResponse(response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func (s *Server) writeResponse(resp response) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(s.err, "mcp: marshal response: %v\n", err)
		return
	}
	_, _ = s.out.Write(append(data, '\n'))
}

func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func decodeToolArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func okToolResult(summary string, structured any) toolResult {
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: summary}},
		StructuredContent: structured,
	}
}

func errorToolResult(message string) toolResult {
	return toolResult{
		Content: []toolContent{{Type: "text", Text: strings.TrimSpace(message)}},
		IsError: true,
	}
}

func capabilitiesPayload() map[string]any {
	defs := catalog.DefaultDefinitions(true)
	workloads := make([]map[string]string, 0, len(defs))
	for _, def := range defs {
		workloads = append(workloads, map[string]string{
			"name":        strings.TrimSpace(def.Name),
			"category":    strings.TrimSpace(def.Category),
			"description": strings.TrimSpace(def.Description),
		})
	}
	catalogInfo := map[string]any{"source": nodecatalog.SourceEmbedded}
	if manifest, err := nodecatalog.Embedded(); err == nil {
		catalogInfo["revision"] = manifest.Revision
		catalogInfo["node_count"] = len(manifest.Nodes)
		catalogInfo["schema_version"] = manifest.SchemaVersion
	}
	return map[string]any{
		"version":            vmbench.Version,
		"protocol_version":   protocolVersion,
		"go":                 runtime.Version(),
		"os":                 runtime.GOOS,
		"arch":               runtime.GOARCH,
		"suite_sections":     suite.SectionIDs(),
		"suite_presets":      suite.Presets(),
		"route_presets":      suite.RoutePresets(),
		"speed_providers":    suite.SpeedProviders(),
		"hardware_tools":     catalog.HardwareTools(),
		"default_hardware":   catalog.DefaultHardwareTools(),
		"default_suite_only": []string{"hardware"},
		"workloads":          workloads,
		"node_catalog":       catalogInfo,
		"policy": map[string]string{
			"scoring": "vmbench MCP returns raw metrics and structured diagnostics only; no benchmark total score, grade, or category score.",
			"network": "network suite sections run only when explicitly requested by preset or only sections.",
		},
	}
}

func toolSpecs() []toolSpec {
	return []toolSpec{
		{
			Name:        "vmbench_capabilities",
			Title:       "VMBench capabilities",
			Description: "List vmbench version, suite sections, presets, hardware tools, speed providers, and workloads.",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "vmbench_sysinfo",
			Title:       "VMBench system info",
			Description: "Collect host CPU, memory, OS, disk, GPU, and network interface information.",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "vmbench_run",
			Title:       "VMBench raw benchmark run",
			Description: "Run vmbench workloads and return raw metrics. Defaults to hardware scope, one iteration, and no synthetic scoring.",
			InputSchema: objectSchema(map[string]any{
				"iterations":         map[string]any{"type": "integer", "minimum": 1, "maximum": maxIterations, "description": "Iterations per workload. Default 1 for MCP."},
				"filter":             map[string]any{"type": "string", "description": "Regex matched against workload name or category."},
				"mode":               map[string]any{"type": "string", "enum": []string{"single", "multi", "all"}, "description": "Compatibility mode. Legacy multi/all values run the external catalog once; tools define concurrency."},
				"scope":              map[string]any{"type": "string", "enum": []string{"hardware", "network", "all"}, "description": "hardware by default; network/all explicitly enable traffic-generating workloads."},
				"disk_path":          map[string]any{"type": "string", "description": "Temp directory for disk workloads."},
				"timeout_ms":         map[string]any{"type": "integer", "minimum": 1, "maximum": maxTimeout.Milliseconds(), "description": "Overall tool timeout in milliseconds."},
				"hardware_tools":     enumArraySchema(catalog.HardwareToolIDs(), "External hardware tools; use capabilities for metadata."),
				"iperf_hosts":        stringArraySchema("iperf3 hosts; only useful with scope=all or speed sections."),
				"catalog_source":     map[string]any{"type": "string", "description": "Node catalog source: embedded, auto, or a local JSON path."},
				"catalog_revision":   map[string]any{"type": "string", "description": "Require an exact node catalog revision before network workloads start."},
				"catalog_cache_path": map[string]any{"type": "string", "description": "Optional cache path used with catalog_source=auto."},
			}, nil),
		},
		{
			Name:        "vmbench_suite",
			Title:       "VMBench VPS suite",
			Description: "Run VPS suite sections. Defaults to hardware only so network diagnostics are opt-in unless a preset/only list requests them.",
			InputSchema: objectSchema(map[string]any{
				"iterations":         map[string]any{"type": "integer", "minimum": 1, "maximum": maxIterations, "description": "Iterations for hardware workloads. Default 1 for MCP."},
				"filter":             map[string]any{"type": "string", "description": "Regex matched against hardware workload name or category."},
				"disk_path":          map[string]any{"type": "string", "description": "Temp directory for disk workloads."},
				"timeout_ms":         map[string]any{"type": "integer", "minimum": 1, "maximum": maxTimeout.Milliseconds(), "description": "Per-section timeout in milliseconds; hardware applies it per workload."},
				"preset":             map[string]any{"type": "string", "enum": suite.PresetIDs(), "description": "Scenario preset. Enables its sections."},
				"only":               enumArraySchema(suite.SectionIDs(), "Run only these suite sections."),
				"skip":               enumArraySchema(suite.SectionIDs(), "Skip these suite sections."),
				"route_presets":      enumArraySchema(routePresetIDs(), "Route/ping preset IDs."),
				"speed_providers":    enumArraySchema(suite.SpeedProviderIDs(), "Speed providers."),
				"hardware_tools":     enumArraySchema(catalog.HardwareToolIDs(), "External hardware tools."),
				"iperf_hosts":        stringArraySchema("iperf3 hosts; adds iperf3 speed provider when speed is enabled."),
				"ip_version":         map[string]any{"type": "string", "enum": []string{"v4", "v6", "dual"}, "description": "Network IP version."},
				"catalog_source":     map[string]any{"type": "string", "description": "Node catalog source: embedded, auto, or a local JSON path."},
				"catalog_revision":   map[string]any{"type": "string", "description": "Require an exact node catalog revision before Suite sections start."},
				"catalog_cache_path": map[string]any{"type": "string", "description": "Optional cache path used with catalog_source=auto."},
			}, nil),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func enumArraySchema(values []string, description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
			"enum": values,
		},
	}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

func normalizeIterations(raw json.RawMessage) (int, string) {
	if len(raw) == 0 {
		return 1, ""
	}
	if strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return 1, "iterations must be an integer"
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 1, "iterations must be an integer"
	}
	if value < 1 || value > maxIterations {
		return 1, fmt.Sprintf("iterations must be between 1 and %d", maxIterations)
	}
	return value, ""
}

func normalizeTimeoutMillis(raw json.RawMessage, fallback time.Duration) (time.Duration, string) {
	if len(raw) == 0 {
		return fallback, ""
	}
	if strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return fallback, "timeout_ms must be an integer"
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback, "timeout_ms must be an integer"
	}
	if value < 1 || value > maxTimeout.Milliseconds() {
		return fallback, fmt.Sprintf("timeout_ms must be between 1 and %d", maxTimeout.Milliseconds())
	}
	return time.Duration(value) * time.Millisecond, ""
}

func validateFilter(filter string) string {
	if filter == "" {
		return ""
	}
	if _, err := regexp.Compile(filter); err != nil {
		return "invalid filter regex: " + err.Error()
	}
	return ""
}

func appendValidationError(warnings *[]string, message string) {
	if strings.TrimSpace(message) != "" {
		*warnings = append(*warnings, message)
	}
}

func firstInvalidValue(raw []string, normalize func([]string) []string) string {
	for _, value := range cleanList(raw) {
		if len(normalize([]string{value})) == 0 {
			return value
		}
	}
	return ""
}

func cleanList(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func routePresetIDs() []string {
	items := suite.RoutePresets()
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func formatSysinfoSummary(info sysinfo.SystemInfo, warnings []string) string {
	parts := []string{
		"system info collected",
		strings.TrimSpace(info.OS.Name),
		strings.TrimSpace(info.CPU.Model),
	}
	if len(warnings) > 0 {
		parts = append(parts, fmt.Sprintf("warnings=%d", len(warnings)))
	}
	return joinNonEmpty(parts, " | ")
}

func formatRunSummary(doc gbreport.Document) string {
	workloads := append([]gbreport.WorkloadEntry{}, doc.Results.Workloads...)
	workloads = append(workloads, doc.Extensions.Workloads...)
	failed := 0
	for _, item := range workloads {
		if item.Result == nil || strings.TrimSpace(item.Result.Error) != "" {
			failed++
		}
	}
	status := "ok"
	if gbreport.HasFailures(doc) {
		status = "failed"
	}
	return fmt.Sprintf("vmbench run completed: status=%s workloads=%d failed=%d warnings=%d", status, len(workloads), failed, len(doc.Warnings))
}

func formatSuiteSummary(report suite.SuiteReport) string {
	sections := report.Sections()
	enabled := 0
	failed := 0
	for _, item := range sections {
		if !item.Enabled {
			continue
		}
		enabled++
		if !strings.EqualFold(item.Status, "ok") {
			failed++
		}
	}
	return fmt.Sprintf("vmbench suite completed: status=%s sections=%d failed=%d message=%s", report.Status, enabled, failed, report.Message)
}

func joinNonEmpty(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, sep)
}

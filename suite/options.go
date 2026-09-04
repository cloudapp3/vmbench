package suite

import (
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

type SectionSelector struct {
	Hardware     bool `json:"hardware"`
	NetworkInfo  bool `json:"network_info"`
	Route        bool `json:"route"`
	Ping         bool `json:"ping"`
	Speed        bool `json:"speed"`
	IPQuality    bool `json:"ip_quality"`
	Reachability bool `json:"reachability"`
	Mail         bool `json:"mail"`
	Media        bool `json:"media"`
}

type PresetSpec struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Sections     SectionSelector `json:"sections"`
	IPVersion    string          `json:"ip_version,omitempty"`
	RoutePresets []string        `json:"route_presets,omitempty"`
	MediaSet     string          `json:"media_set,omitempty"`
}

type Options struct {
	Iterations       int                   `json:"iterations"`
	Filter           string                `json:"filter,omitempty"`
	DiskPath         string                `json:"disk_path,omitempty"`
	Timeout          time.Duration         `json:"timeout,omitempty"`
	Preset           string                `json:"preset,omitempty"`
	RoutePresets     []string              `json:"route_presets,omitempty"`
	SpeedProviders   []string              `json:"speed_providers,omitempty"`
	HardwareTools    []string              `json:"hardware_tools,omitempty"`
	Sections         SectionSelector       `json:"sections"`
	IperfHosts       []string              `json:"iperf_hosts,omitempty"`
	IPVersion        string                `json:"ip_version,omitempty"`
	MediaSet         string                `json:"media_set,omitempty"`
	IPSources        []string              `json:"ip_sources,omitempty"`
	CatalogSource    string                `json:"catalog_source,omitempty"`
	CatalogRevision  string                `json:"catalog_revision,omitempty"`
	CatalogCachePath string                `json:"catalog_cache_path,omitempty"`
	ResolvedCatalog  *nodecatalog.Manifest `json:"-"`
	CatalogWarning   string                `json:"-"`
	NodeIDs          []string              `json:"-"`
	OnEvent          EventHandler          `json:"-"`
}

type EventKind string

const (
	EventSectionStart EventKind = "section.start"
	EventSectionDone  EventKind = "section.done"
	EventSectionFail  EventKind = "section.fail"
	EventSectionSkip  EventKind = "section.skip"
	EventSuiteDone    EventKind = "suite.done"
)

type Event struct {
	Kind    EventKind
	Section SectionID
	Status  string
	Message string
	Time    time.Time
}

type EventHandler func(Event)

type RoutePresetSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Flag        string `json:"flag"`
}

type SpeedProviderSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Requires    string `json:"requires,omitempty"`
}

var defaultRoutePresetOrder = []string{"gz", "bj", "sh", "cd", "cernet", "cstnet"}

var routePresetSpecs = map[string]RoutePresetSpec{
	"gz":     {ID: "gz", Name: "广州三网", Description: "广州电信 / 联通 / 移动预置点", Flag: "--gz"},
	"bj":     {ID: "bj", Name: "北京三网", Description: "北京电信 / 联通 / 移动预置点", Flag: "--bj"},
	"sh":     {ID: "sh", Name: "上海三网", Description: "上海电信 / 联通 / 移动预置点", Flag: "--sh"},
	"cd":     {ID: "cd", Name: "成都线路", Description: "成都电信 / 联通 / 移动 / 教育网预置点", Flag: "--cd"},
	"cernet": {ID: "cernet", Name: "教育网", Description: "CERNET 可验证预置点", Flag: "--cernet"},
	"cstnet": {ID: "cstnet", Name: "科技网", Description: "CSTNET 可验证预置点", Flag: "--cstnet"},
}

var presetOrder = []string{"quick", "website", "proxy", "mail"}

const (
	SpeedProviderCloudflare   = "cloudflare"
	SpeedProviderSpeedtestNet = "speedtest_net"
	SpeedProviderSpeedtestCN  = "speedtest_cn"
	SpeedProviderIperf3       = "iperf3"
	SpeedProviderChinaISP     = "china_isp"
	SpeedProviderSpeedtestISP = "speedtest_isp"
)

var defaultSpeedProviderOrder = []string{SpeedProviderCloudflare, SpeedProviderSpeedtestNet, SpeedProviderSpeedtestCN, SpeedProviderIperf3, SpeedProviderChinaISP, SpeedProviderSpeedtestISP}

var speedProviderSpecs = map[string]SpeedProviderSpec{
	SpeedProviderCloudflare: {
		ID:          SpeedProviderCloudflare,
		Name:        "Cloudflare",
		Description: "HTTP multi-download and upload via speed.cloudflare.com.",
	},
	SpeedProviderSpeedtestNet: {
		ID:          SpeedProviderSpeedtestNet,
		Name:        "Speedtest.net",
		Description: "Ookla Speedtest CLI JSON result.",
		Requires:    "speedtest CLI",
	},
	SpeedProviderSpeedtestCN: {
		ID:          SpeedProviderSpeedtestCN,
		Name:        "Speedtest.cn",
		Description: "Compatible speedtest.cn CLI JSON result.",
		Requires:    "speedtest-cn compatible CLI",
	},
	SpeedProviderIperf3: {
		ID:          SpeedProviderIperf3,
		Name:        "iperf3",
		Description: "TCP bandwidth to user-provided --iperf-host targets.",
		Requires:    "iperf3 CLI and --iperf-host",
	},
	SpeedProviderChinaISP: {
		ID:          SpeedProviderChinaISP,
		Name:        "China ISP (speedtest.cn)",
		Description: "Direct HTTP download per China carrier (telecom/unicom/mobile) from versioned catalog isp_download nodes.",
	},
	SpeedProviderSpeedtestISP: {
		ID:          SpeedProviderSpeedtestISP,
		Name:        "China ISP (speedtest.net)",
		Description: "Ookla speedtest CLI pinned to per-carrier China server IDs.",
		Requires:    "speedtest CLI",
	},
}

// IP quality evidence sources.
const (
	IPSourceBuiltin       = netio.IPSourceBuiltin
	IPSourceSecurityCheck = netio.IPSourceSecurityCheck
)

var defaultIPSourceOrder = []string{IPSourceBuiltin, IPSourceSecurityCheck}

// IPSourceSpec describes one IP quality evidence source.
type IPSourceSpec struct {
	ID          string
	Name        string
	Description string
	Requires    string
}

var ipSourceSpecs = map[string]IPSourceSpec{
	IPSourceBuiltin: {
		ID:          IPSourceBuiltin,
		Name:        "Builtin",
		Description: "ip-api.com metadata, ipapi.is ownership cross-check, DNSBL, mail ports.",
	},
	IPSourceSecurityCheck: {
		ID:          IPSourceSecurityCheck,
		Name:        "securityCheck",
		Description: "18-database IP quality view from the external securityCheck binary.",
		Requires:    "securityCheck binary (github.com/oneclickvirt/securityCheck)",
	},
}

// IPSourceIDs lists the selectable IP quality sources.
func IPSourceIDs() []string { return append([]string(nil), defaultIPSourceOrder...) }

// StandardizeIPSources filters and orders raw source names.
func StandardizeIPSources(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		seen[strings.ToLower(strings.TrimSpace(item))] = struct{}{}
	}
	out := make([]string, 0, len(defaultIPSourceOrder))
	for _, item := range defaultIPSourceOrder {
		if _, ok := seen[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

var presetSpecs = map[string]PresetSpec{
	"quick": {
		ID:          "quick",
		Name:        "Quick VPS",
		Description: "YABS-like quick run: hardware, network identity, speed, and IP quality.",
		Sections:    SectionSelector{Hardware: true, NetworkInfo: true, Speed: true, IPQuality: true},
		IPVersion:   "v4",
	},
	"website": {
		ID:          "website",
		Name:        "Website Hosting",
		Description: "Website/server scenario: hardware, network identity, China ping/route, speed, reachability, IP quality, and mail ports.",
		Sections:    SectionSelector{Hardware: true, NetworkInfo: true, Route: true, Ping: true, Speed: true, IPQuality: true, Reachability: true, Mail: true},
		IPVersion:   "v4",
	},
	"proxy": {
		ID:          "proxy",
		Name:        "Proxy/Unlock",
		Description: "Proxy scenario: network identity, China ping/route, speed, reachability, IP quality, and streaming unlock.",
		Sections:    SectionSelector{NetworkInfo: true, Route: true, Ping: true, Speed: true, IPQuality: true, Reachability: true, Media: true},
		IPVersion:   "v4",
	},
	"mail": {
		ID:          "mail",
		Name:        "Mail Server",
		Description: "Mail scenario: network identity, IP quality, mail ports, and route diagnostics.",
		Sections:    SectionSelector{NetworkInfo: true, Route: true, IPQuality: true, Mail: true},
		IPVersion:   "v4",
	},
}

func DefaultSections() SectionSelector {
	return SectionSelector{Hardware: true, NetworkInfo: true, Route: true, Ping: true, Speed: true, IPQuality: true, Reachability: true, Mail: true, Media: true}
}

func (s SectionSelector) AnyEnabled() bool {
	return s.Hardware || s.NetworkInfo || s.Route || s.Ping || s.Speed || s.IPQuality || s.Reachability || s.Mail || s.Media
}

func (s SectionSelector) Names() []string {
	names := make([]string, 0, 9)
	if s.Hardware {
		names = append(names, string(SectionHardware))
	}
	if s.NetworkInfo {
		names = append(names, string(SectionNetworkInfo))
	}
	if s.Route {
		names = append(names, string(SectionRoute))
	}
	if s.Ping {
		names = append(names, string(SectionPing))
	}
	if s.Speed {
		names = append(names, string(SectionSpeed))
	}
	if s.IPQuality {
		names = append(names, string(SectionIPQuality))
	}
	if s.Reachability {
		names = append(names, string(SectionReachability))
	}
	if s.Mail {
		names = append(names, string(SectionMail))
	}
	if s.Media {
		names = append(names, string(SectionMedia))
	}
	return names
}

func (s SectionSelector) String() string {
	return strings.Join(s.Names(), ",")
}

func PrepareOptions(opts Options) Options {
	norm := opts
	norm.Filter = strings.TrimSpace(norm.Filter)
	norm.DiskPath = strings.TrimSpace(norm.DiskPath)
	norm.IperfHosts = normalizeStringList(norm.IperfHosts)
	norm.Preset = normalizePresetID(norm.Preset)
	if norm.Preset != "" {
		if spec, ok := LookupPreset(norm.Preset); ok {
			if !norm.Sections.AnyEnabled() {
				norm.Sections = spec.Sections
			}
			if strings.TrimSpace(norm.IPVersion) == "" {
				norm.IPVersion = spec.IPVersion
			}
			if len(norm.RoutePresets) == 0 && len(spec.RoutePresets) > 0 {
				norm.RoutePresets = append([]string(nil), spec.RoutePresets...)
			}
		} else {
			norm.Preset = ""
		}
	}
	if norm.Iterations <= 0 {
		norm.Iterations = 3
	}
	if norm.Timeout <= 0 {
		norm.Timeout = 5 * time.Minute
	}
	norm.IPVersion = normalizeIPVersion(norm.IPVersion)
	norm.MediaSet = normalizeMediaSet(norm.MediaSet)
	if norm.Sections.Media && strings.TrimSpace(norm.MediaSet) == "" {
		norm.MediaSet = DefaultMediaSet()
	}
	if !norm.Sections.Media {
		norm.MediaSet = ""
	}
	norm.IPSources = StandardizeIPSources(norm.IPSources)
	if norm.Sections.IPQuality && len(norm.IPSources) == 0 {
		norm.IPSources = []string{IPSourceBuiltin}
	}
	if !norm.Sections.IPQuality {
		norm.IPSources = nil
	}
	if !norm.Sections.AnyEnabled() {
		norm.Sections = DefaultSections()
	}
	norm.RoutePresets = StandardizeRoutePresets(norm.RoutePresets)
	if norm.Sections.Route && len(norm.RoutePresets) == 0 {
		norm.RoutePresets = DefaultRoutePresets()
	}
	norm.SpeedProviders = StandardizeSpeedProviders(norm.SpeedProviders)
	if norm.Sections.Speed && len(norm.SpeedProviders) == 0 {
		norm.SpeedProviders = DefaultSpeedProviders()
	}
	if !norm.Sections.Speed {
		norm.SpeedProviders = nil
		norm.IperfHosts = nil
	} else if len(norm.IperfHosts) > 0 && !containsString(norm.SpeedProviders, SpeedProviderIperf3) {
		norm.SpeedProviders = append(norm.SpeedProviders, SpeedProviderIperf3)
	}
	norm.HardwareTools = catalog.StandardizeHardwareTools(norm.HardwareTools)
	if norm.Sections.Hardware && len(norm.HardwareTools) == 0 && len(opts.HardwareTools) == 0 {
		norm.HardwareTools = catalog.DefaultHardwareTools()
	}
	if !norm.Sections.Hardware {
		norm.HardwareTools = nil
	}
	return norm
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizePresetID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeIPVersion(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v6", "ipv6", "6":
		return "v6"
	case "dual", "both", "all":
		return "dual"
	default:
		return "v4"
	}
}

// DefaultMediaSet is the full-platform unlock set.
func DefaultMediaSet() string { return "all" }

// mediaSetIDs lists the UnlockTests region selections exposed to users.
var mediaSetIDs = []string{"all", "globe", "tw", "hk", "jp", "kr", "na", "sa", "eu", "afr", "sea", "oce", "ai"}

// MediaSets returns the selectable media set IDs.
func MediaSets() []string { return append([]string(nil), mediaSetIDs...) }

func normalizeMediaSet(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// StandardizeMediaSet validates one media set token; comma-separated
// combinations are accepted as-is because UnlockTests resolves them.
func StandardizeMediaSet(value string) (string, error) {
	normalized := normalizeMediaSet(value)
	if normalized == "" {
		return DefaultMediaSet(), nil
	}
	if err := netio.ValidateMediaSet(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func SectionsFromList(raw string, base SectionSelector, enable bool) SectionSelector {
	sections := base
	for _, name := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "hardware", "hw":
			sections.Hardware = enable
		case "network_info", "network-info", "netinfo", "identity", "network_identity", "network-identity":
			sections.NetworkInfo = enable
		case "route", "trace", "traceroute", "backtrace":
			sections.Route = enable
		case "ping", "latency":
			sections.Ping = enable
		case "speed", "speedtest", "network":
			sections.Speed = enable
		case "ip", "ip_quality", "ip-quality", "quality":
			sections.IPQuality = enable
		case "reachability", "reach", "web", "website", "tg", "telegram":
			sections.Reachability = enable
		case "mail", "email", "ports", "port":
			sections.Mail = enable
		case "media", "unlock", "streaming":
			sections.Media = enable
		}
	}
	return sections
}

func StandardizeRoutePresets(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if _, ok := routePresetSpecs[value]; !ok {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
	}
	for _, item := range defaultRoutePresetOrder {
		if _, ok := seen[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

func StandardizeSpeedProviders(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value := normalizeSpeedProviderID(item)
		if value == "" {
			continue
		}
		if _, ok := speedProviderSpecs[value]; !ok {
			continue
		}
		seen[value] = struct{}{}
	}
	for _, item := range defaultSpeedProviderOrder {
		if _, ok := seen[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

func normalizeSpeedProviderID(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "cf", "cloudflare", "speed.cloudflare.com":
		return SpeedProviderCloudflare
	case "ookla", "speedtest", "speedtest.net", "speedtest_net", "net":
		return SpeedProviderSpeedtestNet
	case "speedtest.cn", "speedtest_cn", "speedtestcn", "cn":
		return SpeedProviderSpeedtestCN
	case "iperf", "iperf3":
		return SpeedProviderIperf3
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func LookupPreset(id string) (PresetSpec, bool) {
	spec, ok := presetSpecs[normalizePresetID(id)]
	if !ok {
		return PresetSpec{}, false
	}
	spec.RoutePresets = append([]string(nil), spec.RoutePresets...)
	return spec, true
}

func Presets() []PresetSpec {
	out := make([]PresetSpec, 0, len(presetOrder))
	for _, id := range presetOrder {
		if spec, ok := LookupPreset(id); ok {
			out = append(out, spec)
		}
	}
	return out
}

func PresetIDs() []string {
	return append([]string(nil), presetOrder...)
}

func RoutePresets() []RoutePresetSpec {
	out := make([]RoutePresetSpec, 0, len(defaultRoutePresetOrder))
	for _, id := range defaultRoutePresetOrder {
		out = append(out, routePresetSpecs[id])
	}
	return out
}

func DefaultRoutePresets() []string {
	return append([]string(nil), defaultRoutePresetOrder...)
}

func SpeedProviders() []SpeedProviderSpec {
	out := make([]SpeedProviderSpec, 0, len(defaultSpeedProviderOrder))
	for _, id := range defaultSpeedProviderOrder {
		out = append(out, speedProviderSpecs[id])
	}
	return out
}

func SpeedProviderIDs() []string {
	return append([]string(nil), defaultSpeedProviderOrder...)
}

// IPSources returns the ordered IP quality source specifications.
func IPSources() []IPSourceSpec {
	out := make([]IPSourceSpec, 0, len(defaultIPSourceOrder))
	for _, id := range defaultIPSourceOrder {
		out = append(out, ipSourceSpecs[id])
	}
	return out
}

func DefaultSpeedProviders() []string {
	return []string{SpeedProviderCloudflare}
}

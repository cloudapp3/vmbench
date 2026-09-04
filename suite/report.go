package suite

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/sysinfo"
)

type Config struct {
	Iterations      int             `json:"iterations"`
	Filter          string          `json:"filter,omitempty"`
	DiskPath        string          `json:"disk_path,omitempty"`
	TimeoutMS       int64           `json:"timeout_ms,omitempty"`
	Preset          string          `json:"preset,omitempty"`
	RoutePresets    []string        `json:"route_presets,omitempty"`
	SpeedProviders  []string        `json:"speed_providers,omitempty"`
	HardwareTools   []string        `json:"hardware_tools,omitempty"`
	Sections        SectionSelector `json:"sections"`
	IperfHosts      []string        `json:"iperf_hosts,omitempty"`
	IPVersion       string          `json:"ip_version,omitempty"`
	MediaSet        string          `json:"media_set,omitempty"`
	IPSources       []string        `json:"ip_sources,omitempty"`
	CatalogSource   string          `json:"catalog_source,omitempty"`
	CatalogRevision string          `json:"catalog_revision,omitempty"`
	NodeIDs         []string        `json:"node_ids,omitempty"`
}

// AppInfo identifies the vmbench binary that produced a suite report.
type AppInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

type SuiteReport struct {
	SchemaVersion int                `json:"schema_version"`
	ReportKind    string             `json:"report_kind"`
	ReportID      string             `json:"report_id"`
	App           AppInfo            `json:"app"`
	System        sysinfo.SystemInfo `json:"system"`
	StartedAt     time.Time          `json:"started_at"`
	FinishedAt    time.Time          `json:"finished_at"`
	DurationMS    int64              `json:"duration_ms"`

	// Version and the Unix time fields are retained for v1 consumers.
	Version      int                 `json:"version"`
	Config       Config              `json:"config"`
	StartedTime  int64               `json:"started_time,omitempty"`
	UpdatedTime  int64               `json:"updated_time,omitempty"`
	FinishedTime int64               `json:"finished_time,omitempty"`
	Status       string              `json:"status,omitempty"`
	Message      string              `json:"message,omitempty"`
	Hardware     HardwareSection     `json:"hardware"`
	NetworkInfo  NetworkInfoSection  `json:"network_info"`
	Route        RouteSection        `json:"route"`
	Ping         PingSection         `json:"ping"`
	Speed        SpeedSection        `json:"speed"`
	IPQuality    IPQualitySection    `json:"ip_quality"`
	Reachability ReachabilitySection `json:"reachability"`
	Mail         MailSection         `json:"mail"`
	Media        MediaSection        `json:"media"`
	Warnings     []string            `json:"warnings,omitempty"`
}

type SectionSummary struct {
	ID      SectionID `json:"id"`
	Enabled bool      `json:"enabled"`
	Status  string    `json:"status,omitempty"`
	Message string    `json:"message,omitempty"`
}

const (
	reportVersion        = 1
	currentSchemaVersion = 2
	reportKind           = "suite"
)

var fallbackReportSequence atomic.Uint64

func NewSuiteReport(opts Options) SuiteReport {
	norm := PrepareOptions(opts)
	return SuiteReport{
		SchemaVersion: currentSchemaVersion,
		ReportKind:    reportKind,
		ReportID:      newReportID(),
		App: AppInfo{
			Version:   vmbench.Version,
			Commit:    vmbench.Commit,
			BuildTime: vmbench.BuildTime,
		},
		Version: reportVersion,
		Config: Config{
			Iterations:      norm.Iterations,
			Filter:          norm.Filter,
			DiskPath:        norm.DiskPath,
			TimeoutMS:       norm.Timeout.Milliseconds(),
			Preset:          norm.Preset,
			RoutePresets:    append([]string(nil), norm.RoutePresets...),
			SpeedProviders:  append([]string(nil), norm.SpeedProviders...),
			HardwareTools:   append([]string(nil), norm.HardwareTools...),
			Sections:        norm.Sections,
			IperfHosts:      append([]string(nil), norm.IperfHosts...),
			IPVersion:       norm.IPVersion,
			MediaSet:        norm.MediaSet,
			IPSources:       append([]string(nil), norm.IPSources...),
			CatalogSource:   norm.CatalogSource,
			CatalogRevision: norm.CatalogRevision,
			NodeIDs:         append([]string(nil), norm.NodeIDs...),
		},
		Hardware:     HardwareSection{SectionState: SectionState{Enabled: norm.Sections.Hardware}},
		NetworkInfo:  NetworkInfoSection{SectionState: SectionState{Enabled: norm.Sections.NetworkInfo}, IPVersion: norm.IPVersion},
		Route:        RouteSection{SectionState: SectionState{Enabled: norm.Sections.Route}, RoutePresets: append([]string(nil), norm.RoutePresets...), IPVersion: norm.IPVersion},
		Ping:         PingSection{SectionState: SectionState{Enabled: norm.Sections.Ping}, IPVersion: norm.IPVersion},
		Speed:        SpeedSection{SectionState: SectionState{Enabled: norm.Sections.Speed}},
		IPQuality:    IPQualitySection{SectionState: SectionState{Enabled: norm.Sections.IPQuality}},
		Reachability: ReachabilitySection{SectionState: SectionState{Enabled: norm.Sections.Reachability}},
		Mail:         MailSection{SectionState: SectionState{Enabled: norm.Sections.Mail}},
		Media:        MediaSection{SectionState: SectionState{Enabled: norm.Sections.Media}},
	}
}

func newReportID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		value[6] = (value[6] & 0x0f) | 0x40
		value[8] = (value[8] & 0x3f) | 0x80
		encoded := hex.EncodeToString(value[:])
		return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32])
	}
	return fmt.Sprintf("suite-%d-%d-%d", time.Now().UTC().UnixNano(), os.Getpid(), fallbackReportSequence.Add(1))
}

func (r SuiteReport) Sections() []SectionSummary {
	return []SectionSummary{
		{ID: SectionHardware, Enabled: r.Hardware.Enabled, Status: r.Hardware.Status, Message: r.Hardware.Message},
		{ID: SectionNetworkInfo, Enabled: r.NetworkInfo.Enabled, Status: r.NetworkInfo.Status, Message: r.NetworkInfo.Message},
		{ID: SectionRoute, Enabled: r.Route.Enabled, Status: r.Route.Status, Message: r.Route.Message},
		{ID: SectionPing, Enabled: r.Ping.Enabled, Status: r.Ping.Status, Message: r.Ping.Message},
		{ID: SectionSpeed, Enabled: r.Speed.Enabled, Status: r.Speed.Status, Message: r.Speed.Message},
		{ID: SectionIPQuality, Enabled: r.IPQuality.Enabled, Status: r.IPQuality.Status, Message: r.IPQuality.Message},
		{ID: SectionReachability, Enabled: r.Reachability.Enabled, Status: r.Reachability.Status, Message: r.Reachability.Message},
		{ID: SectionMail, Enabled: r.Mail.Enabled, Status: r.Mail.Status, Message: r.Mail.Message},
		{ID: SectionMedia, Enabled: r.Media.Enabled, Status: r.Media.Status, Message: r.Media.Message},
	}
}

func (r SuiteReport) EnabledSectionCount() int {
	count := 0
	for _, item := range r.Sections() {
		if item.Enabled {
			count++
		}
	}
	return count
}

func (r SuiteReport) HasFailures() bool {
	enabled := 0
	for _, item := range r.Sections() {
		if !item.Enabled {
			continue
		}
		enabled++
		if !sectionStatusOK(item.Status) {
			return true
		}
	}
	return enabled == 0
}

func statusFromCounts(okCount, failCount int) string {
	switch {
	case okCount == 0 && failCount == 0:
		return "skipped"
	case okCount == 0:
		return "error"
	case failCount == 0:
		return "ok"
	default:
		return "partial"
	}
}

package suite

import (
	"strings"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
)

type SectionID string

const (
	SectionHardware     SectionID = "hardware"
	SectionNetworkInfo  SectionID = "network_info"
	SectionRoute        SectionID = "route"
	SectionPing         SectionID = "ping"
	SectionSpeed        SectionID = "speed"
	SectionIPQuality    SectionID = "ip_quality"
	SectionReachability SectionID = "reachability"
	SectionMail         SectionID = "mail"
	SectionMedia        SectionID = "media"
)

type SectionState struct {
	Enabled     bool   `json:"enabled"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	StartedTime int64  `json:"started_time,omitempty"`
	FinishTime  int64  `json:"finish_time,omitempty"`
}

type HardwareSection struct {
	SectionState
	Report *vmbench.Report `json:"report,omitempty"`
}

type LocalGlobalAddress = netio.LocalGlobalAddress
type PublicIPIdentity = netio.PublicIPIdentity
type NetworkIdentityProviderResult = netio.NetworkIdentityProviderResult
type NATHeuristic = netio.NATHeuristic
type NetworkIdentityResult = netio.NetworkIdentityResult

type NetworkInfoSection struct {
	SectionState
	IPVersion string                 `json:"ip_version,omitempty"`
	Result    *NetworkIdentityResult `json:"result,omitempty"`
}

type RouteRun = netio.TraceProbeResult
type PingResult = netio.PingProbeResult

type RouteSection struct {
	SectionState
	RoutePresets []string   `json:"route_presets,omitempty"`
	IPVersion    string     `json:"ip_version,omitempty"`
	Results      []RouteRun `json:"results,omitempty"`
}

type PingSection struct {
	SectionState
	IPVersion string       `json:"ip_version,omitempty"`
	Results   []PingResult `json:"results,omitempty"`
}

type SpeedSummary struct {
	Provider          string  `json:"provider,omitempty"`
	ProviderLabel     string  `json:"provider_label,omitempty"`
	Aggregation       string  `json:"aggregation,omitempty"`
	Node              string  `json:"node,omitempty"`
	Region            string  `json:"region,omitempty"`
	DownloadMbps      float64 `json:"download_mbps,omitempty"`
	UploadMbps        float64 `json:"upload_mbps,omitempty"`
	LatencyMs         float64 `json:"latency_ms,omitempty"`
	Available         int     `json:"available,omitempty"`
	Failed            int     `json:"failed,omitempty"`
	SelectedProviders int     `json:"selected_providers,omitempty"`
}

type SpeedProviderResult struct {
	ID            string  `json:"id"`
	Provider      string  `json:"provider"`
	ProviderLabel string  `json:"provider_label,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Status        string  `json:"status"`
	NodeID        string  `json:"node_id,omitempty"`
	Node          string  `json:"node,omitempty"`
	Endpoint      string  `json:"endpoint,omitempty"`
	Region        string  `json:"region,omitempty"`
	DownloadMbps  float64 `json:"download_mbps,omitempty"`
	UploadMbps    float64 `json:"upload_mbps,omitempty"`
	LatencyMs     float64 `json:"latency_ms,omitempty"`
	ElapsedMs     float64 `json:"elapsed_ms,omitempty"`
	Message       string  `json:"message,omitempty"`
}

type SpeedResult struct {
	Summary   *SpeedSummary         `json:"summary,omitempty"`
	Providers []SpeedProviderResult `json:"providers,omitempty"`
	Groups    []SpeedProviderGroup  `json:"groups,omitempty"`
}

type SpeedProviderGroup struct {
	ID            string                `json:"id"`
	Provider      string                `json:"provider"`
	ProviderLabel string                `json:"provider_label,omitempty"`
	Status        string                `json:"status,omitempty"`
	Message       string                `json:"message,omitempty"`
	Available     int                   `json:"available,omitempty"`
	Failed        int                   `json:"failed,omitempty"`
	Summary       *SpeedSummary         `json:"summary,omitempty"`
	Providers     []SpeedProviderResult `json:"providers,omitempty"`
}

func (g SpeedProviderGroup) SummaryValue(kind string) float64 {
	if g.Summary == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "download", "dl":
		return g.Summary.DownloadMbps
	case "upload", "ul":
		return g.Summary.UploadMbps
	case "latency", "ping", "delay":
		return g.Summary.LatencyMs
	default:
		return 0
	}
}

type SpeedSection struct {
	SectionState
	Result *SpeedResult `json:"result,omitempty"`
}

type IPBasicInfo = netio.IPBasicInfo
type IPRiskSummary = netio.IPRiskSummary
type PortProbe = netio.PortProbe
type IPScore = netio.IPScore
type IPQualityResult = netio.IPQualityResult

type IPQualitySection struct {
	SectionState
	Result *IPQualityResult `json:"result,omitempty"`
}

type ReachabilityResult = netio.ReachabilityProbeResult

type ReachabilitySection struct {
	SectionState
	Results []ReachabilityResult `json:"results,omitempty"`
}

type MailSection struct {
	SectionState
	Results []PortProbe `json:"results,omitempty"`
}

type MediaServiceResult = netio.MediaServiceResult
type MediaSummary = netio.MediaSummary
type MediaResult = netio.MediaResult

type MediaSection struct {
	SectionState
	Result *MediaResult `json:"result,omitempty"`
}

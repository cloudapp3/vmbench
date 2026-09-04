package suite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

// OptionsError reports invalid suite configuration before any section starts.
type OptionsError struct {
	Problems []string
}

func (e *OptionsError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return ""
	}
	return strings.Join(e.Problems, "; ")
}

// SectionIDs returns canonical suite section names in execution order.
func SectionIDs() []string {
	return []string{
		string(SectionHardware),
		string(SectionNetworkInfo),
		string(SectionRoute),
		string(SectionPing),
		string(SectionSpeed),
		string(SectionIPQuality),
		string(SectionReachability),
		string(SectionMail),
		string(SectionMedia),
	}
}

// NormalizeSectionName resolves public CLI/MCP aliases to a canonical name.
func NormalizeSectionName(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return ""
	case "hardware", "hw":
		return string(SectionHardware)
	case "network_info", "network-info", "netinfo", "identity", "network_identity", "network-identity":
		return string(SectionNetworkInfo)
	case "route", "trace", "traceroute", "backtrace":
		return string(SectionRoute)
	case "ping", "latency":
		return string(SectionPing)
	case "speed", "speedtest", "network":
		return string(SectionSpeed)
	case "ip", "ip_quality", "ip-quality", "quality":
		return string(SectionIPQuality)
	case "reachability", "reach", "web", "website", "tg", "telegram":
		return string(SectionReachability)
	case "mail", "email", "ports", "port":
		return string(SectionMail)
	case "media", "unlock", "streaming":
		return string(SectionMedia)
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// ApplySectionNames toggles canonical or aliased section names. Unknown names
// are rejected instead of being silently discarded.
func ApplySectionNames(base SectionSelector, values []string, enable bool) (SectionSelector, error) {
	sections := base
	problems := make([]string, 0)
	for _, raw := range values {
		name := NormalizeSectionName(raw)
		switch name {
		case "":
			continue
		case string(SectionHardware):
			sections.Hardware = enable
		case string(SectionNetworkInfo):
			sections.NetworkInfo = enable
		case string(SectionRoute):
			sections.Route = enable
		case string(SectionPing):
			sections.Ping = enable
		case string(SectionSpeed):
			sections.Speed = enable
		case string(SectionIPQuality):
			sections.IPQuality = enable
		case string(SectionReachability):
			sections.Reachability = enable
		case string(SectionMail):
			sections.Mail = enable
		case string(SectionMedia):
			sections.Media = enable
		default:
			problems = append(problems, fmt.Sprintf("unknown section %q; available: %s", strings.TrimSpace(raw), strings.Join(SectionIDs(), ", ")))
		}
	}
	if len(problems) > 0 {
		return SectionSelector{}, &OptionsError{Problems: problems}
	}
	return sections, nil
}

// ValidateOptions applies the shared Suite configuration contract used by
// CLI, TUI, and MCP. Documented zero-value defaults remain valid.
func ValidateOptions(opts Options) error {
	problems := make([]string, 0, 6)
	if opts.Iterations < 0 || opts.Iterations > 9 {
		problems = append(problems, "iterations must be between 1 and 9")
	}
	if opts.Timeout < 0 {
		problems = append(problems, "timeout must not be negative")
	}
	if filter := strings.TrimSpace(opts.Filter); filter != "" {
		if _, err := regexp.Compile(filter); err != nil {
			problems = append(problems, "invalid filter regex: "+err.Error())
		}
	}
	if preset := normalizePresetID(opts.Preset); preset != "" {
		if _, ok := LookupPreset(preset); !ok {
			problems = append(problems, "unknown preset: "+preset+"; available: "+strings.Join(PresetIDs(), ", "))
		}
	}
	switch strings.ToLower(strings.TrimSpace(opts.IPVersion)) {
	case "", "v4", "ipv4", "4", "v6", "ipv6", "6", "dual", "both", "all":
	default:
		problems = append(problems, "ip_version must be one of: v4, v6, dual")
	}
	if invalid := firstInvalidValue(opts.RoutePresets, StandardizeRoutePresets); invalid != "" {
		problems = append(problems, fmt.Sprintf("unknown route preset %q", invalid))
	}
	if opts.Sections.Media && strings.TrimSpace(opts.MediaSet) != "" {
		if _, err := StandardizeMediaSet(opts.MediaSet); err != nil {
			problems = append(problems, fmt.Sprintf("unknown media set %q (valid: %s or comma combinations)",
				opts.MediaSet, strings.Join(MediaSets(), ", ")))
		}
	}
	if invalid := firstInvalidValue(opts.IPSources, StandardizeIPSources); invalid != "" {
		problems = append(problems, fmt.Sprintf("unknown IP quality source %q (valid: %s)",
			invalid, strings.Join(IPSourceIDs(), ", ")))
	}
	if invalid := firstInvalidValue(opts.SpeedProviders, StandardizeSpeedProviders); invalid != "" {
		problems = append(problems, fmt.Sprintf("unknown speed provider %q", invalid))
	}
	if invalid := firstInvalidValue(opts.HardwareTools, catalog.StandardizeHardwareTools); invalid != "" {
		problems = append(problems, fmt.Sprintf("unknown hardware tool %q", invalid))
	}
	if len(problems) > 0 {
		return &OptionsError{Problems: problems}
	}
	return nil
}

// NormalizeOptions validates and resolves Suite defaults without running any
// probe. The normalized value is suitable for report provenance.
func NormalizeOptions(opts Options) (Options, error) {
	if err := ValidateOptions(opts); err != nil {
		return Options{}, err
	}
	norm := PrepareOptions(opts)
	if !suiteUsesCatalog(norm.Sections) {
		clearCatalogOptions(&norm)
		return norm, nil
	}
	if err := resolveCatalogOptions(&norm); err != nil {
		return Options{}, err
	}
	norm.NodeIDs = selectedCatalogNodeIDs(norm)
	return norm, nil
}

func suiteUsesCatalog(sections SectionSelector) bool {
	return sections.Route || sections.Ping || sections.Speed
}

func resolveCatalogOptions(opts *Options) error {
	if opts.ResolvedCatalog != nil {
		manifest := opts.ResolvedCatalog.Clone()
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("node catalog: %w", err)
		}
		pin := strings.TrimSpace(opts.CatalogRevision)
		if pin != "" && manifest.Revision != pin {
			return fmt.Errorf("node catalog revision %q does not match pinned revision %q", manifest.Revision, pin)
		}
		opts.ResolvedCatalog = &manifest
		opts.CatalogRevision = manifest.Revision
		if strings.TrimSpace(opts.CatalogSource) == "" {
			opts.CatalogSource = nodecatalog.SourceEmbedded
		}
		return nil
	}
	loaded, err := nodecatalog.Load(nodecatalog.LoadOptions{
		Source:    opts.CatalogSource,
		Revision:  opts.CatalogRevision,
		CachePath: opts.CatalogCachePath,
	})
	if err != nil {
		return fmt.Errorf("node catalog: %w", err)
	}
	manifest := loaded.Manifest.Clone()
	opts.ResolvedCatalog = &manifest
	opts.CatalogSource = loaded.Source
	opts.CatalogRevision = manifest.Revision
	opts.CatalogWarning = loaded.Warning
	return nil
}

func clearCatalogOptions(opts *Options) {
	opts.CatalogSource = ""
	opts.CatalogRevision = ""
	opts.CatalogCachePath = ""
	opts.ResolvedCatalog = nil
	opts.CatalogWarning = ""
	opts.NodeIDs = nil
}

func catalogManifestForOptions(opts Options) (nodecatalog.Manifest, bool) {
	if opts.ResolvedCatalog != nil {
		return opts.ResolvedCatalog.Clone(), true
	}
	if strings.TrimSpace(opts.CatalogSource) != "" || strings.TrimSpace(opts.CatalogRevision) != "" || strings.TrimSpace(opts.CatalogCachePath) != "" {
		return nodecatalog.Manifest{}, false
	}
	manifest, err := nodecatalog.Embedded()
	return manifest, err == nil
}

func selectedCatalogNodeIDs(opts Options) []string {
	if opts.ResolvedCatalog == nil {
		return nil
	}
	selected := make(map[string]struct{})
	if opts.Sections.Route {
		for _, target := range routeTargetsForManifest(*opts.ResolvedCatalog, opts.RoutePresets, opts.IPVersion) {
			selected[target.ID] = struct{}{}
		}
	}
	if opts.Sections.Ping {
		for _, target := range pingTargetsForManifest(*opts.ResolvedCatalog, opts.RoutePresets, opts.IPVersion) {
			selected[target.ID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(selected))
	for _, node := range opts.ResolvedCatalog.Nodes {
		if _, ok := selected[node.ID]; ok {
			ids = append(ids, node.ID)
		}
	}
	return ids
}

func firstInvalidValue(values []string, normalize func([]string) []string) string {
	for _, item := range normalizeStringList(values) {
		if len(normalize([]string{item})) == 0 {
			return item
		}
	}
	return ""
}

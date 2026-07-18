package netio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	NetworkIdentityProviderOK      = "ok"
	NetworkIdentityProviderPartial = "partial"
	NetworkIdentityProviderError   = "error"
	NetworkIdentityProviderSkipped = "skipped"

	NATStatusDirect     = "direct"
	NATStatusTranslated = "translated"
	NATStatusUnknown    = "unknown"
)

// LocalGlobalAddress is a non-loopback global-unicast address assigned to a
// local interface. RFC1918 and unique-local addresses are retained and marked
// private because they are useful evidence for the NAT heuristic.
type LocalGlobalAddress struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	IPVersion string `json:"ip_version"`
	Private   bool   `json:"private"`
}

// PublicIPIdentity stores one observed public address and its coarse metadata.
type PublicIPIdentity struct {
	IP          string `json:"ip"`
	IPVersion   string `json:"ip_version"`
	ASN         int64  `json:"asn,omitempty"`
	Org         string `json:"org,omitempty"`
	ISP         string `json:"isp,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

// NetworkIdentityProviderResult records one independent evidence provider.
// Errors stay attached to the provider that produced them.
type NetworkIdentityProviderResult struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	IPVersion string `json:"ip_version,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// NATHeuristic compares an observed public address with local global-unicast
// addresses. It is diagnostic evidence, not authoritative NAT detection.
type NATHeuristic struct {
	IPVersion string `json:"ip_version"`
	Status    string `json:"status"`
	Method    string `json:"method"`
	PublicIP  string `json:"public_ip,omitempty"`
	LocalIP   string `json:"local_ip,omitempty"`
	Reason    string `json:"reason"`
}

// NetworkIdentityResult stores local addresses, observed public identities,
// per-provider status, and an explicit NAT heuristic. It has no aggregate score.
type NetworkIdentityResult struct {
	LocalGlobalAddresses []LocalGlobalAddress            `json:"local_global_addresses,omitempty"`
	PublicIPv4           *PublicIPIdentity               `json:"public_ipv4,omitempty"`
	PublicIPv6           *PublicIPIdentity               `json:"public_ipv6,omitempty"`
	NAT                  []NATHeuristic                  `json:"nat,omitempty"`
	Providers            []NetworkIdentityProviderResult `json:"providers"`
}

type publicIPMetadata struct {
	ASN         int64
	Org         string
	ISP         string
	Country     string
	CountryCode string
}

type networkIdentityDependencies struct {
	localAddresses func() ([]LocalGlobalAddress, []error)
	publicIP       func(context.Context, string) (string, error)
	metadata       func(context.Context, string) (publicIPMetadata, error)
}

type identityFamilyResult struct {
	identity  *PublicIPIdentity
	providers []NetworkIdentityProviderResult
}

// ProbeNetworkIdentity gathers network identity evidence for v4, v6, or dual.
// A non-nil result is returned with provider errors whenever partial evidence is
// available.
func ProbeNetworkIdentity(ctx context.Context, ipVersion string) (*NetworkIdentityResult, error) {
	return probeNetworkIdentity(ctx, ipVersion, networkIdentityDependencies{
		localAddresses: collectLocalGlobalAddresses,
		publicIP:       queryPublicIP,
		metadata:       queryPublicIPMetadata,
	})
}

func probeNetworkIdentity(ctx context.Context, ipVersion string, deps networkIdentityDependencies) (*NetworkIdentityResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	families, err := identityFamilies(ipVersion)
	if err != nil {
		return &NetworkIdentityResult{}, err
	}
	if deps.localAddresses == nil || deps.publicIP == nil || deps.metadata == nil {
		return &NetworkIdentityResult{}, errors.New("network identity: incomplete probe dependencies")
	}

	result := &NetworkIdentityResult{}
	localAddresses, localErrors := deps.localAddresses()
	result.LocalGlobalAddresses = localAddresses
	localStatus := NetworkIdentityProviderOK
	localError := ""
	if len(localErrors) > 0 {
		messages := make([]string, 0, len(localErrors))
		for _, item := range localErrors {
			if item != nil {
				messages = append(messages, item.Error())
			}
		}
		localError = strings.Join(messages, "; ")
		if len(localAddresses) > 0 {
			localStatus = NetworkIdentityProviderPartial
		} else {
			localStatus = NetworkIdentityProviderError
		}
	}
	result.Providers = append(result.Providers, NetworkIdentityProviderResult{
		ID:     "local_interfaces",
		Kind:   "local_addresses",
		Status: localStatus,
		Error:  localError,
	})

	familyResults := make([]identityFamilyResult, len(families))
	var done = make(chan struct{}, len(families))
	for idx, family := range families {
		go func(index int, version string) {
			familyResults[index] = probeIdentityFamily(ctx, version, deps)
			done <- struct{}{}
		}(idx, family)
	}
	for range families {
		<-done
	}

	publicCount := 0
	for idx, family := range families {
		familyResult := familyResults[idx]
		result.Providers = append(result.Providers, familyResult.providers...)
		if familyResult.identity != nil {
			publicCount++
			switch family {
			case "v4":
				result.PublicIPv4 = familyResult.identity
			case "v6":
				result.PublicIPv6 = familyResult.identity
			}
		}
		result.NAT = append(result.NAT, buildNATHeuristic(family, familyResult.identity, localAddresses, localStatus))
	}

	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("network identity: %w", err)
	}
	if publicCount == 0 {
		return result, errors.New("network identity: no requested public IP could be observed")
	}
	return result, nil
}

func probeIdentityFamily(ctx context.Context, family string, deps networkIdentityDependencies) identityFamilyResult {
	publicProvider := NetworkIdentityProviderResult{
		ID:        "ipify_" + family,
		Kind:      "public_ip",
		IPVersion: family,
		Status:    NetworkIdentityProviderError,
	}
	metadataProvider := NetworkIdentityProviderResult{
		ID:        "ipwhois_" + family,
		Kind:      "metadata",
		IPVersion: family,
		Status:    NetworkIdentityProviderSkipped,
	}

	publicIP, err := deps.publicIP(ctx, family)
	if err != nil {
		publicProvider.Error = err.Error()
		metadataProvider.Error = "public IP unavailable"
		return identityFamilyResult{providers: []NetworkIdentityProviderResult{publicProvider, metadataProvider}}
	}
	parsed := net.ParseIP(strings.TrimSpace(publicIP))
	if parsed == nil || (family == "v4" && parsed.To4() == nil) || (family == "v6" && parsed.To4() != nil) {
		publicProvider.Error = fmt.Sprintf("provider returned invalid %s address %q", family, publicIP)
		metadataProvider.Error = "public IP unavailable"
		return identityFamilyResult{providers: []NetworkIdentityProviderResult{publicProvider, metadataProvider}}
	}
	publicIP = parsed.String()
	publicProvider.Status = NetworkIdentityProviderOK
	identity := &PublicIPIdentity{IP: publicIP, IPVersion: family}

	metadata, err := deps.metadata(ctx, publicIP)
	if err != nil {
		metadataProvider.Status = NetworkIdentityProviderError
		metadataProvider.Error = err.Error()
		return identityFamilyResult{identity: identity, providers: []NetworkIdentityProviderResult{publicProvider, metadataProvider}}
	}
	metadataProvider.Status = NetworkIdentityProviderOK
	identity.ASN = metadata.ASN
	identity.Org = strings.TrimSpace(metadata.Org)
	identity.ISP = strings.TrimSpace(metadata.ISP)
	identity.Country = strings.TrimSpace(metadata.Country)
	identity.CountryCode = strings.ToUpper(strings.TrimSpace(metadata.CountryCode))
	return identityFamilyResult{identity: identity, providers: []NetworkIdentityProviderResult{publicProvider, metadataProvider}}
}

func identityFamilies(value string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "v4", "ipv4", "4":
		return []string{"v4"}, nil
	case "v6", "ipv6", "6":
		return []string{"v6"}, nil
	case "dual", "both":
		return []string{"v4", "v6"}, nil
	default:
		return nil, fmt.Errorf("network identity: unsupported IP version %q", value)
	}
}

func collectLocalGlobalAddresses() ([]LocalGlobalAddress, []error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, []error{err}
	}

	seen := make(map[string]struct{})
	out := make([]LocalGlobalAddress, 0)
	errs := make([]error, 0)
	for _, item := range interfaces {
		if item.Flags&net.FlagLoopback != 0 || item.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := item.Addrs()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", item.Name, err))
			continue
		}
		for _, address := range addresses {
			ipText := address.String()
			if host, _, err := net.ParseCIDR(ipText); err == nil {
				ipText = host.String()
			} else if host, _, err := net.SplitHostPort(ipText); err == nil {
				ipText = strings.Trim(host, "[]")
			}
			ip := net.ParseIP(strings.TrimSpace(ipText))
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}
			family := "v6"
			if ip.To4() != nil {
				family = "v4"
			}
			normalized := ip.String()
			key := item.Name + "\x00" + normalized
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, LocalGlobalAddress{
				Interface: item.Name,
				Address:   normalized,
				IPVersion: family,
				Private:   ip.IsPrivate(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IPVersion != out[j].IPVersion {
			return out[i].IPVersion < out[j].IPVersion
		}
		if out[i].Interface != out[j].Interface {
			return out[i].Interface < out[j].Interface
		}
		return out[i].Address < out[j].Address
	})
	return out, errs
}

func queryPublicIP(ctx context.Context, family string) (string, error) {
	endpoint := "https://api4.ipify.org"
	if family == "v6" {
		endpoint = "https://api6.ipify.org"
	}
	return queryPublicIPFromURL(ctx, http.DefaultClient, endpoint, family)
}

func queryPublicIPFromURL(ctx context.Context, client *http.Client, endpoint, family string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(body))
	ip := net.ParseIP(value)
	if ip == nil || (family == "v4" && ip.To4() == nil) || (family == "v6" && ip.To4() != nil) {
		return "", fmt.Errorf("invalid %s address %q", family, value)
	}
	return ip.String(), nil
}

type ipWhoIsResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Connection  struct {
		ASN int64  `json:"asn"`
		Org string `json:"org"`
		ISP string `json:"isp"`
	} `json:"connection"`
}

func queryPublicIPMetadata(ctx context.Context, ip string) (publicIPMetadata, error) {
	endpoint := "https://ipwho.is/" + url.PathEscape(ip)
	return queryPublicIPMetadataFromURL(ctx, http.DefaultClient, endpoint, ip)
}

func queryPublicIPMetadataFromURL(ctx context.Context, client *http.Client, endpoint, expectedIP string) (publicIPMetadata, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return publicIPMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return publicIPMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return publicIPMetadata{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var data ipWhoIsResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&data); err != nil {
		return publicIPMetadata{}, err
	}
	if !data.Success {
		return publicIPMetadata{}, fmt.Errorf("provider failure: %s", strings.TrimSpace(data.Message))
	}
	returnedIP := net.ParseIP(strings.TrimSpace(data.IP))
	wantedIP := net.ParseIP(strings.TrimSpace(expectedIP))
	if returnedIP == nil || wantedIP == nil || !returnedIP.Equal(wantedIP) {
		return publicIPMetadata{}, fmt.Errorf("provider returned mismatched address %q", data.IP)
	}
	if data.Connection.ASN <= 0 || strings.TrimSpace(data.Connection.Org) == "" || strings.TrimSpace(data.CountryCode) == "" {
		return publicIPMetadata{}, errors.New("provider returned incomplete ASN/org/country metadata")
	}
	return publicIPMetadata{
		ASN:         data.Connection.ASN,
		Org:         data.Connection.Org,
		ISP:         data.Connection.ISP,
		Country:     data.Country,
		CountryCode: data.CountryCode,
	}, nil
}

func buildNATHeuristic(family string, identity *PublicIPIdentity, local []LocalGlobalAddress, localStatus string) NATHeuristic {
	const method = "public_ip_vs_local_global_unicast"
	result := NATHeuristic{
		IPVersion: family,
		Status:    NATStatusUnknown,
		Method:    method,
		Reason:    "public IP unavailable",
	}
	if identity == nil {
		return result
	}
	result.PublicIP = identity.IP
	candidates := make([]LocalGlobalAddress, 0)
	for _, item := range local {
		if item.IPVersion != family {
			continue
		}
		candidates = append(candidates, item)
		if net.ParseIP(item.Address).Equal(net.ParseIP(identity.IP)) {
			result.Status = NATStatusDirect
			result.LocalIP = item.Address
			result.Reason = "observed public IP is assigned to a local interface"
			return result
		}
	}
	if localStatus != NetworkIdentityProviderOK {
		result.Reason = "local interface evidence is incomplete"
		return result
	}
	if len(candidates) == 0 {
		result.Reason = "no local global-unicast address was detected for this IP version"
		return result
	}
	result.Status = NATStatusTranslated
	result.LocalIP = candidates[0].Address
	result.Reason = "observed public IP is not assigned to a local interface"
	return result
}

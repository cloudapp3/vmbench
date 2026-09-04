package netio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// IPAPIISInfo stores the ipapi.is ownership cross-check evidence.
//
// Since 2026-09-01 the anonymous tier of ipapi.is no longer returns risk
// flags (is_datacenter / is_proxy / is_abuser / ... require an API key), so
// this source contributes ownership corroboration (company, ASN, bogon
// status) instead of risk scoring.
type IPAPIISInfo struct {
	Source    string `json:"source"`
	Supported bool   `json:"supported"`
	IP        string `json:"ip,omitempty"`
	Company   string `json:"company,omitempty"`
	ASN       string `json:"asn,omitempty"`
	Location  string `json:"location,omitempty"`
	IsBogon   bool   `json:"is_bogon,omitempty"`
	Message   string `json:"message,omitempty"`
}

// CrossCheck compares the ipapi.is ownership evidence with the primary
// metadata and reports whether the two sources agree.
func (info *IPAPIISInfo) CrossCheck(basic *IPBasicInfo) string {
	if info == nil || !info.Supported || basic == nil {
		return ""
	}
	notes := make([]string, 0, 2)
	if info.Company != "" && basic.Org != "" &&
		!strings.Contains(strings.ToLower(info.Company), firstWordLower(basic.Org)) &&
		!strings.Contains(strings.ToLower(basic.Org), firstWordLower(info.Company)) {
		notes = append(notes, fmt.Sprintf("company mismatch: %q vs %q", info.Company, basic.Org))
	}
	if basic.ASN != 0 && info.ASN != "" {
		if m := asnRe.FindStringSubmatch(info.ASN); len(m) > 1 {
			var reported int64
			fmt.Sscan(m[1], &reported)
			if reported != 0 && reported != basic.ASN {
				notes = append(notes, fmt.Sprintf("asn mismatch: AS%d vs AS%d", reported, basic.ASN))
			}
		}
	}
	if len(notes) == 0 {
		return "ownership agrees with primary metadata"
	}
	return strings.Join(notes, "; ")
}

func firstWordLower(value string) string {
	fields := strings.Fields(strings.ToLower(value))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// ipapiisBaseURL is swappable for tests.
var ipapiisBaseURL = "https://api.ipapi.is/"

// queryIPAPIIS fetches ownership evidence for one IPv4 address. Failures are
// reported through Supported=false and never fail the whole probe.
func queryIPAPIIS(ctx context.Context, ip string) *IPAPIISInfo {
	info := &IPAPIISInfo{Source: "ipapi.is"}
	if strings.TrimSpace(ip) == "" {
		info.Message = "no public IP to query"
		return info
	}
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	endpoint := strings.TrimRight(ipapiisBaseURL, "/") + "/?q=" + ip
	req, err := http.NewRequestWithContext(queryCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		info.Message = err.Error()
		return info
	}
	req.Header.Set("User-Agent", ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		info.Message = err.Error()
		return info
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		info.Message = err.Error()
		return info
	}
	if resp.StatusCode != http.StatusOK {
		info.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return info
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		info.Message = err.Error()
		return info
	}
	info.Supported = true
	info.IP = jsonStringValue(data, "ip")
	info.Company = jsonStringValue(data, "company", "company_name")
	info.ASN = jsonStringValue(data, "asn", "asn_org")
	info.IsBogon = jsonBoolValue(data, "is_bogon")
	city := jsonStringValue(data, "city")
	country := jsonStringValue(data, "country")
	switch {
	case city != "" && country != "":
		info.Location = city + ", " + country
	case country != "":
		info.Location = country
	}
	return info
}

func jsonStringValue(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			case float64:
				return fmt.Sprintf("%.0f", typed)
			}
		}
	}
	return ""
}

func jsonBoolValue(data map[string]any, key string) bool {
	if value, ok := data[key]; ok {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	return false
}

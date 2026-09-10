package redact

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

// identityFixture mirrors the report layout the harvester walks.
type identityFixture struct {
	NetworkInfo struct {
		Result *struct {
			PublicIPv4 *struct {
				IP string `json:"ip"`
			} `json:"public_ipv4"`
			PublicIPv6 *struct {
				IP string `json:"ip"`
			} `json:"public_ipv6"`
			NAT []struct {
				PublicIP string `json:"public_ip,omitempty"`
			} `json:"nat"`
			LocalGlobalAddresses []struct {
				Address string `json:"address"`
			} `json:"local_global_addresses"`
		} `json:"result"`
	} `json:"network_info"`
	IPQuality struct {
		Result *struct {
			BasicInfo *struct {
				IP string `json:"ip"`
			} `json:"basic_info"`
		} `json:"result"`
	} `json:"ip_quality"`
}

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", Default, false},
		{"ips", ModeIPs, false},
		{"IPS", ModeIPs, false},
		{" none ", ModeNone, false},
		{"strict", "", true},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	if !ModeIPs.Enabled() || !Mode("").Enabled() || ModeNone.Enabled() {
		t.Error("Enabled: only none is disabled; the zero value means default (ips)")
	}
}

func TestApplyMasksAllFormsConsistently(t *testing.T) {
	locals := []net.IP{net.ParseIP("96.9.228.37"), net.ParseIP("2600:1f2:3:4::5")}
	raw := `{
	  "text": "IP:96.9.228.37 via http://96.9.228.37:8080 and ::ffff:96.9.228.37",
	  "dnsbl": "lookup 37.228.9.96.zen.spamhaus.org failed",
	  "prefix": "96.9.228.0/24",
	  "announced": "96.9.228.0/23",
	  "host": "96.9.228.37/32",
	  "range": "96.9.228.0 - 96.9.228.255",
	  "v6": "2600:1f2:3:4::5 and 2600:1f2:3:4::/64",
	  "remote": "8.8.8.8 in 8.8.8.0/24 and 2001:4860:4860::8888"
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	out, stats, err := Apply(doc, locals)
	if err != nil {
		t.Fatal(err)
	}
	text := mustJSON(t, out)

	for _, want := range []string{
		`"text":"IP:203.0.113.1 via http://203.0.113.1:8080 and ::ffff:203.0.113.1"`, // bare + URL + v4-in-v6
		`"dnsbl":"lookup 1.113.0.203.zen.spamhaus.org failed"`,                       // reverse DNSBL label
		`"prefix":"203.0.113.0/24"`,                                                  // /24 keeps length
		`"announced":"203.0.113.0/24"`,                                               // /23 collapses to /24
		`"host":"203.0.113.1/32"`,                                                    // /32 keeps the host
		`"range":"203.0.113.0/24"`,                                                   // range → prefix form
		`"v6":"2001:db8::1 and 2001:db8::/64"`,                                       // bare v6 + /64 prefix
		`"remote":"8.8.8.8 in 8.8.8.0/24 and 2001:4860:4860::8888"`,                  // remote untouched
	} {
		if !strings.Contains(text, want) {
			t.Errorf("redacted JSON missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "96.9.228") || strings.Contains(text, "2600:1f2:3:4::5") {
		t.Errorf("original address leaked:\n%s", text)
	}
	if strings.Contains(text, "37.228.9.96.zen") {
		t.Errorf("reverse label leaked:\n%s", text)
	}
	if stats.Replacements == 0 || len(stats.MappedIPv4) != 1 || len(stats.MappedIPv6) != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestApplyBoundaryGuards(t *testing.T) {
	doc := map[string]any{
		"text":   "a 1.2.3.9 b 1.2.3.99 c 11.2.3.9 d 1.2.3.9.5 e",
		"target": "1.2.3.9",
	}
	out, _, err := Apply(doc, []net.IP{net.ParseIP("1.2.3.9")})
	if err != nil {
		t.Fatal(err)
	}
	text := mustJSON(t, out)
	for _, want := range []string{"a 203.0.113.1 b", "c 11.2.3.9 d", "1.2.3.9.5"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "1.2.3.9 b") || strings.Contains(text, `"target":"1.2.3.9"`) {
		t.Errorf("unguarded replacement happened: %s", text)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Regression (real-world capture): CIDR evidence surfaces inside provider
// messages where a colon follows the mask. The token must stay eligible —
// only a longer number can extend a mask, never punctuation.
func TestApplyCIDRTokenFollowedByColon(t *testing.T) {
	doc := map[string]any{
		"detail": "96.9.228.0/24: invalid PNG format; 96.9.228.0/23: invalid PNG format",
		"v6":     "2001:df1:801:a022::/64: bad scope",
	}
	out, _, err := Apply(doc, []net.IP{net.ParseIP("96.9.228.95"), net.ParseIP("2001:df1:801:a022::29:18")})
	if err != nil {
		t.Fatal(err)
	}
	text := mustJSON(t, out)
	for _, want := range []string{
		`"detail":"203.0.113.0/24: invalid PNG format; 203.0.113.0/24: invalid PNG format"`,
		`"v6":"2001:db8::/64: bad scope"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
}

// Regression (real-world capture): SLAAC assigns several global addresses per
// interface and only one is the observed egress address. Every interface
// global address belongs to the machine and must be harvested; private
// entries stay untouched.
func TestApplyHarvestsInterfaceGlobalAddresses(t *testing.T) {
	raw := `{"network_info": {"result": {
	  "public_ipv6": {"ip": "2001:df1:801:a022::29:18"},
	  "local_global_addresses": [
	    {"address": "2001:df1:801:a022::29:10", "private": false},
	    {"address": "2001:df1:801:a022::29:18", "private": false},
	    {"address": "10.0.0.5", "private": true}
	  ]
	}}}`
	var fixture identityFixture
	if err := json.Unmarshal([]byte(raw), &fixture); err != nil {
		t.Fatal(err)
	}
	out, _, err := Apply(fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := mustJSON(t, out)
	if strings.Contains(text, "2001:df1") {
		t.Errorf("interface global address leaked: %s", text)
	}
	for _, want := range []string{"2001:db8::", "10.0.0.5"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
}

func TestApplyHarvestsIdentityFields(t *testing.T) {
	raw := `{"network_info": {"result": {
	  "public_ipv4": {"ip": "96.9.228.37"},
	  "public_ipv6": {"ip": "2600:1f2:3:4::5"},
	  "nat": [{"public_ip": "96.9.228.37"}]
	}}}`
	var fixture identityFixture
	if err := json.Unmarshal([]byte(raw), &fixture); err != nil {
		t.Fatal(err)
	}
	// Apply must discover both addresses from the identity fields without
	// explicit locals.
	out, stats, err := Apply(fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.MappedIPv4) != 1 || stats.MappedIPv4[0] != "96.9.228.37" {
		t.Fatalf("MappedIPv4 = %v", stats.MappedIPv4)
	}
	if len(stats.MappedIPv6) != 1 || stats.MappedIPv6[0] != "2600:1f2:3:4::5" {
		t.Fatalf("MappedIPv6 = %v", stats.MappedIPv6)
	}
	if out.NetworkInfo.Result.PublicIPv4.IP != "203.0.113.1" {
		t.Errorf("public_ipv4.ip = %q", out.NetworkInfo.Result.PublicIPv4.IP)
	}
	if out.NetworkInfo.Result.PublicIPv6.IP != "2001:db8::1" {
		t.Errorf("public_ipv6.ip = %q", out.NetworkInfo.Result.PublicIPv6.IP)
	}
}

func TestApplyOrderingAndMultipleAddresses(t *testing.T) {
	doc := map[string]any{"a": "9.9.9.9", "b": "1.1.1.1", "c": "8.8.8.8"}
	out, stats, err := Apply(doc, []net.IP{net.ParseIP("9.9.9.9"), net.ParseIP("1.1.1.1"), net.ParseIP("8.8.8.8")})
	if err != nil {
		t.Fatal(err)
	}
	text := mustJSON(t, out)
	// Lexicographic v4 order: 1.1.1.1 → .1, 8.8.8.8 → .2, 9.9.9.9 → .3.
	for _, want := range []string{`"b":"203.0.113.1"`, `"c":"203.0.113.2"`, `"a":"203.0.113.3"`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if len(stats.MappedIPv4) != 3 {
		t.Errorf("MappedIPv4 = %v", stats.MappedIPv4)
	}
}

func TestApplyNoRedactableAddressIsNoOp(t *testing.T) {
	raw := map[string]any{
		"private":  "192.168.1.10",
		"loopback": "127.0.0.1",
		"docnet":   "198.51.100.20",
		"v6":       "fd00::1",
	}
	out, stats, err := Apply(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Replacements != 0 || len(stats.MappedIPv4) != 0 {
		t.Errorf("stats = %+v, want no-op", stats)
	}
	got := mustJSON(t, out)
	want := mustJSON(t, raw)
	if got != want {
		t.Errorf("no-op changed document:\n%s\n%s", got, want)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	doc := map[string]any{
		"network_info": map[string]any{
			"result": map[string]any{
				"public_ipv4": map[string]any{"ip": "96.9.228.37"},
			},
		},
		"prefix": "96.9.228.0/24",
		"dnsbl":  "37.228.9.96.zen.spamhaus.org",
	}
	once, _, err := Apply(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	twice, stats, err := Apply(once, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Replacements != 0 {
		t.Errorf("second pass replaced %d sites, want idempotent", stats.Replacements)
	}
	if mustJSON(t, once) != mustJSON(t, twice) {
		t.Errorf("not idempotent:\n%s\n%s", mustJSON(t, once), mustJSON(t, twice))
	}
	if strings.Contains(mustJSON(t, once), "96.9.228") {
		t.Errorf("prefix octets leaked: %s", mustJSON(t, once))
	}
}

func TestApplyPreservesNumbersAndStructure(t *testing.T) {
	type inner struct {
		MedianMS float64   `json:"median_ms"`
		Samples  []float64 `json:"samples_ms"`
		Detail   string    `json:"detail,omitempty"`
	}
	type doc struct {
		Name    string         `json:"name"`
		Metrics inner          `json:"metrics"`
		Tags    map[string]int `json:"tags"`
	}
	report := doc{
		Name:    "net",
		Metrics: inner{MedianMS: 12.5, Samples: []float64{1, 2.25, 3}, Detail: "egress 96.9.228.37"},
		Tags:    map[string]int{"one": 1},
	}
	out, _, err := Apply(report, []net.IP{net.ParseIP("96.9.228.37")})
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "net" || out.Metrics.MedianMS != 12.5 || len(out.Metrics.Samples) != 3 || out.Metrics.Samples[1] != 2.25 || out.Tags["one"] != 1 {
		t.Errorf("round trip lost data: %+v", out)
	}
	if out.Metrics.Detail != "egress 203.0.113.1" {
		t.Errorf("detail = %q", out.Metrics.Detail)
	}
}

func TestReplaceBoundedAdjacentOccurrences(t *testing.T) {
	body := []byte("1.2.3.4 1.2.3.4,1.2.3.4")
	out, count := replaceBounded(body, "1.2.3.4", "X")
	if count != 3 || string(out) != "X X,X" {
		t.Errorf("replaceBounded = %q, %d", out, count)
	}
}

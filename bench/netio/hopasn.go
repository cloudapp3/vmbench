package netio

import (
	"embed"
	"net/netip"
	"strings"
	"sync"
)

// asnPrefixFS embeds the China carrier IPv6 prefix snapshots copied from
// oneclickvirt/backtrace v0.0.21 (Apache-2.0); see asnprefixes/README.md.
//
//go:embed asnprefixes/as*.txt
var asnPrefixFS embed.FS

// hopASNv4Rules are the China carrier IPv4 backbone ranges. They follow the
// public backbone allocations (202.97/16 is ChinaNet 163, 59.43/16 is CN2,
// and so on); 223.118.32.0/21 is the CMIN2 exception inside AS58453 space.
var hopASNv4Rules = []struct {
	prefix netip.Prefix
	asn    string
}{
	{mustPrefix("59.43.0.0/16"), "AS4809"},     // 电信 CN2 GT/GIA
	{mustPrefix("202.97.0.0/16"), "AS4134"},    // 电信 163 骨干网
	{mustPrefix("218.105.0.0/16"), "AS9929"},   // 联通 9929 优质国际线路
	{mustPrefix("210.51.0.0/16"), "AS9929"},    // 联通 9929 优质国际线路
	{mustPrefix("202.77.0.0/16"), "AS10099"},   // 联通 CUG 国际网络
	{mustPrefix("43.252.0.0/16"), "AS10099"},   // 联通 CUG 国际网络
	{mustPrefix("61.14.0.0/16"), "AS10099"},    // 联通 CUG 国际网络
	{mustPrefix("219.158.0.0/16"), "AS4837"},   // 联通 4837 普通国际线路
	{mustPrefix("223.118.32.0/21"), "AS58807"}, // 移动 CMIN2 精品网特例
	{mustPrefix("223.118.0.0/16"), "AS58453"},  // 移动 CMI
	{mustPrefix("223.119.0.0/16"), "AS58453"},  // 移动 CMI
	{mustPrefix("223.120.0.0/16"), "AS58453"},  // 移动 CMI
	{mustPrefix("223.121.0.0/16"), "AS58453"},  // 移动 CMI
	{mustPrefix("221.183.0.0/16"), "AS9808"},   // 移动 CMNET
	{mustPrefix("111.24.0.0/16"), "AS9808"},    // 移动 CMNET
	{mustPrefix("69.194.0.0/16"), "AS23764"},   // 电信 CTGNET
	{mustPrefix("203.22.0.0/16"), "AS23764"},   // 电信 CTGNET
}

type asnPrefixIndex struct {
	once   sync.Once
	prefix []netip.Prefix
	asn    []string
}

var hopASNv6Index asnPrefixIndex

// load builds the IPv6 prefix index once. Unparseable lines are skipped so a
// refreshed snapshot can never break annotation at runtime.
func (idx *asnPrefixIndex) load() {
	idx.once.Do(func() {
		entries, err := asnPrefixFS.ReadDir("asnprefixes")
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "as") || !strings.HasSuffix(name, ".txt") {
				continue
			}
			asn := strings.ToUpper(strings.TrimSuffix(name, ".txt"))
			data, err := asnPrefixFS.ReadFile("asnprefixes/" + name)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				prefix, err := netip.ParsePrefix(line)
				if err != nil || prefix.Addr().Is4() || prefix.Addr().Is4In6() {
					continue
				}
				idx.prefix = append(idx.prefix, prefix)
				idx.asn = append(idx.asn, asn)
			}
		}
	})
}

// HopASN annotates one traceroute hop IP with a China carrier backbone ASN.
// It returns an empty string for non-carrier, private, or unknown addresses;
// this is display evidence, not a general-purpose IP-to-ASN database.
func HopASN(ipStr string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return ""
	}
	if address.Is4() {
		for _, rule := range hopASNv4Rules {
			if rule.prefix.Contains(address) {
				return rule.asn
			}
		}
		return ""
	}
	hopASNv6Index.load()
	for i, prefix := range hopASNv6Index.prefix {
		if prefix.Contains(address.Unmap()) {
			return hopASNv6Index.asn[i]
		}
	}
	return ""
}

func mustPrefix(value string) netip.Prefix {
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		panic("hopasn: invalid static prefix " + value)
	}
	return prefix
}

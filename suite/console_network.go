package suite

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
)

func writeNetworkInfoConsole(w io.Writer, section NetworkInfoSection) error {
	if _, err := writeSectionBanner(w, SectionNetworkInfo); err != nil {
		return err
	}
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if section.Result == nil {
		return nil
	}

	if len(section.Result.LocalGlobalAddresses) > 0 {
		headers := []string{i18n.T("report.suite.col.localInterface"), i18n.T("report.suite.col.ipVersion"), i18n.T("report.suite.col.address"), i18n.T("report.suite.col.private")}
		rows := make([][]string, 0, len(section.Result.LocalGlobalAddresses))
		for _, item := range section.Result.LocalGlobalAddresses {
			rows = append(rows, []string{item.Interface, item.IPVersion, item.Address, fmt.Sprintf("%t", item.Private)})
		}
		if err := writeGrid(w, headers, rows); err != nil {
			return err
		}
	}

	public := make([]*PublicIPIdentity, 0, 2)
	if section.Result.PublicIPv4 != nil {
		public = append(public, section.Result.PublicIPv4)
	}
	if section.Result.PublicIPv6 != nil {
		public = append(public, section.Result.PublicIPv6)
	}
	if len(public) > 0 {
		headers := []string{i18n.T("report.suite.col.publicIP"), i18n.T("report.suite.col.version"), i18n.T("report.suite.col.asn"), i18n.T("report.suite.col.country"), i18n.T("report.suite.col.organization")}
		rows := make([][]string, 0, len(public))
		for _, item := range public {
			asn := "-"
			if item.ASN > 0 {
				asn = "AS" + strconv.FormatInt(item.ASN, 10)
			}
			rows = append(rows, []string{
				item.IP,
				item.IPVersion,
				asn,
				defaultText(item.CountryCode, item.Country),
				defaultText(item.Org, item.ISP),
			})
		}
		if err := writeGrid(w, headers, rows); err != nil {
			return err
		}
	}

	if len(section.Result.NAT) > 0 {
		headers := []string{i18n.T("report.suite.col.nat"), i18n.T("report.suite.col.status"), i18n.T("report.suite.col.publicIP"), i18n.T("report.suite.col.localIP"), i18n.T("report.suite.col.reason")}
		rows := make([][]string, 0, len(section.Result.NAT))
		for _, item := range section.Result.NAT {
			rows = append(rows, []string{
				item.IPVersion,
				item.Status,
				defaultText(item.PublicIP, "-"),
				defaultText(item.LocalIP, "-"),
				defaultText(item.Reason, "-"),
			})
		}
		if err := writeGrid(w, headers, rows); err != nil {
			return err
		}
	}

	if stun := section.Result.STUNNAT; stun != nil {
		if _, err := fmt.Fprintf(w, "%s %s (%s %s", i18n.T("report.sh.stunPrefix"), defaultText(stun.NATType, i18n.T("report.sh.inconclusive")), i18n.T("report.suite.stateStatus"), stun.Status); err != nil {
			return err
		}
		if stun.MappingBehavior != "" {
			if _, err := fmt.Fprintf(w, ", %s %s", i18n.T("report.sh.mapping"), stun.MappingBehavior); err != nil {
				return err
			}
		}
		if stun.FilteringBehavior != "" {
			if _, err := fmt.Fprintf(w, ", %s %s", i18n.T("report.sh.filtering"), stun.FilteringBehavior); err != nil {
				return err
			}
		}
		if stun.Hairpin != "" {
			if _, err := fmt.Fprintf(w, ", %s %s", i18n.T("report.sh.hairpin"), stun.Hairpin); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, ")"); err != nil {
			return err
		}
		if message := strings.TrimSpace(stun.Message); message != "" {
			if _, err := fmt.Fprintf(w, "%s %s\n", i18n.T("report.sh.stunDetail"), message); err != nil {
				return err
			}
		}
	}

	if bgp := section.Result.IPBGP; bgp != nil {
		if _, err := fmt.Fprintf(w, "%s %s %s (%s %s", i18n.T("report.sh.bgpPrefix"), defaultText(bgp.ASN, "-"), defaultText(bgp.NetworkName, ""), i18n.T("report.suite.stateStatus"), bgp.Status); err != nil {
			return err
		}
		if len(bgp.Prefixes) > 0 {
			if _, err := fmt.Fprintf(w, ", %s %s", i18n.T("report.sh.prefix"), strings.Join(bgp.Prefixes, " ")); err != nil {
				return err
			}
		}
		if bgp.RIR != "" {
			if _, err := fmt.Fprintf(w, ", %s", bgp.RIR); err != nil {
				return err
			}
		}
		if bgp.RegistrationDate != "" {
			if _, err := fmt.Fprintf(w, ", %s %s", i18n.T("report.sh.registered"), bgp.RegistrationDate); err != nil {
				return err
			}
		}
		if bgp.Tier1Upstreams > 0 {
			if _, err := fmt.Fprintf(w, ", %s %d", i18n.T("report.sh.tier1Upstreams"), bgp.Tier1Upstreams); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, ")"); err != nil {
			return err
		}
		if message := strings.TrimSpace(bgp.Message); message != "" {
			if _, err := fmt.Fprintf(w, "%s %s\n", i18n.T("report.sh.bgpDetail"), message); err != nil {
				return err
			}
		}
		if len(bgp.Relationships) > 0 {
			headers := []string{i18n.T("report.suite.col.relationship"), i18n.T("report.suite.col.asn"), i18n.T("report.suite.col.name"), i18n.T("report.suite.col.source")}
			rows := make([][]string, 0, len(bgp.Relationships))
			for _, rel := range bgp.Relationships {
				name := rel.Name
				if rel.Tier1 {
					name = name + " [Tier1]"
				}
				rows = append(rows, []string{defaultText(rel.Kind, "-"), defaultText(rel.ASN, "-"), defaultText(name, "-"), defaultText(rel.Source, "-")})
			}
			if err := writeGrid(w, headers, rows); err != nil {
				return err
			}
		}
	}

	if neighbors := section.Result.CIDRNeighbors; neighbors != nil {
		if _, err := fmt.Fprintf(w, "%s: ", i18n.T("report.sh.activeNeighborsPrefix")); err != nil {
			return err
		}
		parts := make([]string, 0, 2)
		if neighbors.SubnetActive > 0 {
			parts = append(parts, i18n.Tf("report.sh.neighborsSubnet", map[string]any{"Active": fmt.Sprintf("%d", neighbors.SubnetActive), "Total": fmt.Sprintf("%d", neighbors.SubnetTotal), "Prefix": neighbors.SubnetPrefix}))
		}
		if neighbors.PrefixActive > 0 && neighbors.AnnouncedPrefix != neighbors.SubnetPrefix {
			parts = append(parts, i18n.Tf("report.sh.neighborsAnnounced", map[string]any{"Active": fmt.Sprintf("%d", neighbors.PrefixActive), "Total": fmt.Sprintf("%d", neighbors.PrefixTotal), "Prefix": neighbors.AnnouncedPrefix}))
		}
		if len(parts) > 0 {
			if _, err := fmt.Fprintf(w, "%s (%s %s)\n", strings.Join(parts, " · "), i18n.T("report.suite.stateStatus"), neighbors.Status); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "%s (%s %s)\n", i18n.T("report.sh.unavailable"), i18n.T("report.suite.stateStatus"), neighbors.Status); err != nil {
				return err
			}
			if message := strings.TrimSpace(neighbors.Message); message != "" {
				if _, err := fmt.Fprintf(w, "%s %s\n", i18n.T("report.sh.neighborsDetail"), message); err != nil {
					return err
				}
			}
		}
	}

	if subnet := section.Result.IPv6Subnet; subnet != nil {
		if subnet.Status == "ok" {
			if _, err := fmt.Fprintf(w, "%s /%d (%s)\n", i18n.T("report.sh.ipv6SubnetPrefix"), subnet.PrefixLength, subnet.Address); err != nil {
				return err
			}
		} else if subnet.Status != "unsupported" {
			if _, err := fmt.Fprintf(w, "%s %s (%s)\n", i18n.T("report.sh.ipv6SubnetPrefix"), subnet.Status, defaultText(subnet.Message, subnet.Address)); err != nil {
				return err
			}
		}
	}

	if len(section.Result.Providers) > 0 {
		headers := []string{i18n.T("report.suite.col.provider"), i18n.T("report.suite.col.kind"), i18n.T("report.suite.col.version"), i18n.T("report.suite.col.status"), i18n.T("report.suite.col.error")}
		rows := make([][]string, 0, len(section.Result.Providers))
		for _, item := range section.Result.Providers {
			rows = append(rows, []string{
				item.ID,
				item.Kind,
				defaultText(item.IPVersion, "-"),
				item.Status,
				defaultText(item.Error, "-"),
			})
		}
		if err := writeGrid(w, headers, rows); err != nil {
			return err
		}
	}
	return nil
}

func writeReachabilityConsole(w io.Writer, section ReachabilitySection) error {
	if _, err := writeSectionBanner(w, SectionReachability); err != nil {
		return err
	}
	if err := writeSectionState(w, section.SectionState); err != nil {
		return err
	}
	if len(section.Results) == 0 {
		return nil
	}
	headers := []string{i18n.T("report.suite.col.target"), i18n.T("report.suite.col.category"), i18n.T("report.suite.col.protocol"), i18n.T("report.suite.col.endpoint"), i18n.T("report.suite.col.status"), i18n.T("report.suite.col.latency"), i18n.T("report.suite.col.http"), i18n.T("report.suite.col.error")}
	rows := make([][]string, 0, len(section.Results))
	for _, item := range section.Results {
		httpStatus := "-"
		if item.HTTPStatus > 0 {
			httpStatus = strconv.Itoa(item.HTTPStatus)
		}
		rows = append(rows, []string{
			item.ID,
			item.Category,
			item.Protocol,
			item.Endpoint,
			item.Status,
			formatMaybeFloat(item.LatencyMs, "ms"),
			httpStatus,
			defaultText(item.Error, "-"),
		})
	}
	return writeGrid(w, headers, rows)
}

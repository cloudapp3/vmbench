package suite

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

func writeNetworkInfoConsole(w io.Writer, section NetworkInfoSection) error {
	if _, err := fmt.Fprintln(w, "\n[Network Info]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(section.Status, "unknown")); err != nil {
		return err
	}
	if message := strings.TrimSpace(section.Message); message != "" {
		if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
			return err
		}
	}
	if section.Result == nil {
		return nil
	}

	if len(section.Result.LocalGlobalAddresses) > 0 {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "Local interface\tIP version\tAddress\tPrivate")
		for _, item := range section.Result.LocalGlobalAddresses {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\n", item.Interface, item.IPVersion, item.Address, item.Private)
		}
		if err := tw.Flush(); err != nil {
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
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "Public IP\tVersion\tASN\tCountry\tOrganization")
		for _, item := range public {
			asn := "-"
			if item.ASN > 0 {
				asn = "AS" + strconv.FormatInt(item.ASN, 10)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.IP,
				item.IPVersion,
				asn,
				defaultText(item.CountryCode, item.Country),
				defaultText(item.Org, item.ISP),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}

	if len(section.Result.NAT) > 0 {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAT\tStatus\tPublic IP\tLocal IP\tReason")
		for _, item := range section.Result.NAT {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.IPVersion,
				item.Status,
				defaultText(item.PublicIP, "-"),
				defaultText(item.LocalIP, "-"),
				defaultText(item.Reason, "-"),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}

	if stun := section.Result.STUNNAT; stun != nil {
		if _, err := fmt.Fprintf(w, "stun nat: %s (status %s", defaultText(stun.NATType, "Inconclusive"), stun.Status); err != nil {
			return err
		}
		if stun.MappingBehavior != "" {
			if _, err := fmt.Fprintf(w, ", mapping %s", stun.MappingBehavior); err != nil {
				return err
			}
		}
		if stun.FilteringBehavior != "" {
			if _, err := fmt.Fprintf(w, ", filtering %s", stun.FilteringBehavior); err != nil {
				return err
			}
		}
		if stun.Hairpin != "" {
			if _, err := fmt.Fprintf(w, ", hairpin %s", stun.Hairpin); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, ")"); err != nil {
			return err
		}
		if message := strings.TrimSpace(stun.Message); message != "" {
			if _, err := fmt.Fprintf(w, "stun detail: %s\n", message); err != nil {
				return err
			}
		}
	}

	if bgp := section.Result.IPBGP; bgp != nil {
		if _, err := fmt.Fprintf(w, "ip bgp: %s %s (status %s", defaultText(bgp.ASN, "-"), defaultText(bgp.NetworkName, ""), bgp.Status); err != nil {
			return err
		}
		if len(bgp.Prefixes) > 0 {
			if _, err := fmt.Fprintf(w, ", prefix %s", strings.Join(bgp.Prefixes, " ")); err != nil {
				return err
			}
		}
		if bgp.RIR != "" {
			if _, err := fmt.Fprintf(w, ", %s", bgp.RIR); err != nil {
				return err
			}
		}
		if bgp.RegistrationDate != "" {
			if _, err := fmt.Fprintf(w, ", registered %s", bgp.RegistrationDate); err != nil {
				return err
			}
		}
		if bgp.Tier1Upstreams > 0 {
			if _, err := fmt.Fprintf(w, ", tier1 upstreams %d", bgp.Tier1Upstreams); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, ")"); err != nil {
			return err
		}
		if message := strings.TrimSpace(bgp.Message); message != "" {
			if _, err := fmt.Fprintf(w, "bgp detail: %s\n", message); err != nil {
				return err
			}
		}
		if len(bgp.Relationships) > 0 {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "Relationship\tASN\tName\tSource")
			for _, rel := range bgp.Relationships {
				name := rel.Name
				if rel.Tier1 {
					name = name + " [Tier1]"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", defaultText(rel.Kind, "-"), defaultText(rel.ASN, "-"), defaultText(name, "-"), defaultText(rel.Source, "-"))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}

	if neighbors := section.Result.CIDRNeighbors; neighbors != nil {
		if _, err := fmt.Fprintf(w, "active neighbors: "); err != nil {
			return err
		}
		parts := make([]string, 0, 2)
		if neighbors.SubnetActive > 0 {
			parts = append(parts, fmt.Sprintf("%d/%d in %s (subnet)", neighbors.SubnetActive, neighbors.SubnetTotal, neighbors.SubnetPrefix))
		}
		if neighbors.PrefixActive > 0 && neighbors.AnnouncedPrefix != neighbors.SubnetPrefix {
			parts = append(parts, fmt.Sprintf("%d/%d in %s (announced)", neighbors.PrefixActive, neighbors.PrefixTotal, neighbors.AnnouncedPrefix))
		}
		if len(parts) > 0 {
			if _, err := fmt.Fprintf(w, "%s (status %s)\n", strings.Join(parts, " · "), neighbors.Status); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "unavailable (status %s)\n", neighbors.Status); err != nil {
				return err
			}
			if message := strings.TrimSpace(neighbors.Message); message != "" {
				if _, err := fmt.Fprintf(w, "neighbors detail: %s\n", message); err != nil {
					return err
				}
			}
		}
	}

	if subnet := section.Result.IPv6Subnet; subnet != nil {
		if subnet.Status == "ok" {
			if _, err := fmt.Fprintf(w, "ipv6 subnet: /%d (%s)\n", subnet.PrefixLength, subnet.Address); err != nil {
				return err
			}
		} else if subnet.Status != "unsupported" {
			if _, err := fmt.Fprintf(w, "ipv6 subnet: %s (%s)\n", subnet.Status, defaultText(subnet.Message, subnet.Address)); err != nil {
				return err
			}
		}
	}

	if len(section.Result.Providers) > 0 {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "Provider\tKind\tVersion\tStatus\tError")
		for _, item := range section.Result.Providers {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.ID,
				item.Kind,
				defaultText(item.IPVersion, "-"),
				item.Status,
				defaultText(item.Error, "-"),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func writeReachabilityConsole(w io.Writer, section ReachabilitySection) error {
	if _, err := fmt.Fprintln(w, "\n[Reachability]"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(section.Status, "unknown")); err != nil {
		return err
	}
	if message := strings.TrimSpace(section.Message); message != "" {
		if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
			return err
		}
	}
	if len(section.Results) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "Target\tCategory\tProtocol\tEndpoint\tStatus\tLatency\tHTTP\tError")
	for _, item := range section.Results {
		httpStatus := "-"
		if item.HTTPStatus > 0 {
			httpStatus = strconv.Itoa(item.HTTPStatus)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.ID,
			item.Category,
			item.Protocol,
			item.Endpoint,
			item.Status,
			formatMaybeFloat(item.LatencyMs, "ms"),
			httpStatus,
			defaultText(item.Error, "-"),
		)
	}
	return tw.Flush()
}

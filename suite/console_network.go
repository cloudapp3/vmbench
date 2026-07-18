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

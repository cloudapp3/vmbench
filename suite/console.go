package suite

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	gbreport "github.com/cloudapp3/vmbench/report"
)

func WriteConsole(w io.Writer, report SuiteReport) error {
	if w == nil {
		w = os.Stdout
	}
	if _, err := fmt.Fprintln(w, "VMBench Suite"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Status: %s\n", defaultText(report.Status, "unknown")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Message: %s\n", defaultText(report.Message, "-")); err != nil {
		return err
	}
	if preset := strings.TrimSpace(report.Config.Preset); preset != "" {
		if _, err := fmt.Fprintf(w, "Preset: %s\n", preset); err != nil {
			return err
		}
	}
	if sections := report.Config.Sections.String(); sections != "" {
		if _, err := fmt.Fprintf(w, "Sections: %s\n", sections); err != nil {
			return err
		}
	}

	if report.Hardware.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Hardware]"); err != nil {
			return err
		}
		if report.Hardware.Report != nil {
			if err := gbreport.WriteConsole(w, *report.Hardware.Report); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Hardware.Status, "unknown")); err != nil {
				return err
			}
			if message := strings.TrimSpace(report.Hardware.Message); message != "" {
				if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
					return err
				}
			}
		}
	}

	if report.NetworkInfo.Enabled {
		if err := writeNetworkInfoConsole(w, report.NetworkInfo); err != nil {
			return err
		}
	}

	if report.Route.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Route]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Route.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.Route.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if len(report.Route.Results) > 0 {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "Target\tCity\tCarrier\tResolved\tProbe\tHops\tReached\tLine\tStatus")
			for _, item := range report.Route.Results {
				status := item.EffectiveStatus()
				if message := strings.TrimSpace(item.Error); message != "" {
					status += ": " + message
				}
				probe := defaultText(item.ProbeProtocol, "unknown") + "/" + defaultText(item.ProbeTool, "unknown")
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
					item.Target.Name,
					item.Target.City,
					item.Target.Carrier,
					defaultText(item.ResolvedTarget, "unknown"),
					probe,
					len(item.Hops),
					traceDestinationReachedText(item.DestinationReached),
					traceClassificationText(item.Classification),
					status,
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}

	if report.Ping.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Ping]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Ping.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.Ping.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if len(report.Ping.Results) > 0 {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "Target\tCity\tCarrier\tIP\tProbe\tConnection\tAvg\tJitter\tLoss\tStatus")
			for _, item := range report.Ping.Results {
				status := defaultText(item.Status, "unknown")
				if item.Status != "ok" && item.Message != "" {
					status = item.Message
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%.0f%%\t%s\n",
					defaultText(item.Name, "-"),
					defaultText(item.City, "-"),
					defaultText(item.Carrier, "-"),
					defaultText(item.IPFamily, "-"),
					defaultText(item.ProbeProtocol, "unknown")+"/"+defaultText(item.ProbeTool, "unknown"),
					defaultText(item.ConnectionState, "unknown"),
					formatMaybeFloat(item.AvgLatencyMs, "ms"),
					formatMaybeFloat(item.JitterMs, "ms"),
					item.PacketLoss,
					status,
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}

	if report.Speed.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Speed]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Speed.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.Speed.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if report.Speed.Result != nil {
			if len(report.Speed.Result.Groups) > 0 {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "Group\tStatus\tOk\tFail\tDL\tUL\tLatency\tMessage")
				for _, group := range report.Speed.Result.Groups {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n",
						defaultText(group.ProviderLabel, group.Provider),
						defaultText(group.Status, "unknown"),
						group.Available,
						group.Failed,
						formatMaybeFloat(group.SummaryValue("download"), "Mbps"),
						formatMaybeFloat(group.SummaryValue("upload"), "Mbps"),
						formatMaybeFloat(group.SummaryValue("latency"), "ms"),
						defaultText(group.Message, "-"),
					)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				for _, group := range report.Speed.Result.Groups {
					if len(group.Providers) == 0 {
						continue
					}
					if _, err := fmt.Fprintf(w, "\n  [%s]\n", defaultText(group.ProviderLabel, group.Provider)); err != nil {
						return err
					}
					tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
					_, _ = fmt.Fprintln(tw, "Provider\tKind\tNode\tDL\tUL\tLatency\tStatus\tMessage")
					for _, item := range group.Providers {
						_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
							defaultText(item.ProviderLabel, item.Provider),
							defaultText(item.Kind, "-"),
							defaultText(item.Node, "-"),
							formatMaybeFloat(item.DownloadMbps, "Mbps"),
							formatMaybeFloat(item.UploadMbps, "Mbps"),
							formatMaybeFloat(item.LatencyMs, "ms"),
							defaultText(item.Status, "unknown"),
							defaultText(item.Message, "-"),
						)
					}
					if err := tw.Flush(); err != nil {
						return err
					}
				}
			} else if len(report.Speed.Result.Providers) > 0 {
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "Provider\tKind\tNode\tDL\tUL\tLatency\tStatus\tMessage")
				for _, item := range report.Speed.Result.Providers {
					_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						defaultText(item.ProviderLabel, item.Provider),
						defaultText(item.Kind, "-"),
						defaultText(item.Node, "-"),
						formatMaybeFloat(item.DownloadMbps, "Mbps"),
						formatMaybeFloat(item.UploadMbps, "Mbps"),
						formatMaybeFloat(item.LatencyMs, "ms"),
						defaultText(item.Status, "unknown"),
						defaultText(item.Message, "-"),
					)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
			}
		}
	}

	if report.IPQuality.Enabled {
		if _, err := fmt.Fprintln(w, "\n[IP Quality]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.IPQuality.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.IPQuality.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if result := report.IPQuality.Result; result != nil {
			if info := result.BasicInfo; info != nil {
				if _, err := fmt.Fprintf(w, "ip: %s | country: %s | asn: %d | org: %s\n", defaultText(info.IP, "-"), defaultText(info.CountryCode, defaultText(info.Country, "-")), info.ASN, defaultText(info.Org, defaultText(info.ISP, "-"))); err != nil {
					return err
				}
			}
			if score := result.Score; score != nil {
				if _, err := fmt.Fprintf(w, "score: %d/%d | level: %s\n", score.Total, score.MaxTotal, defaultText(score.Level, "unknown")); err != nil {
					return err
				}
			}
			if cross := result.IPAPIIS; cross != nil && cross.Supported {
				if _, err := fmt.Fprintf(w, "ipapi.is: %s | %s | %s\n", defaultText(cross.Company, "-"), defaultText(cross.ASN, "-"), defaultText(cross.Location, "-")); err != nil {
					return err
				}
			}
			if len(result.Sources) > 0 {
				parts := make([]string, 0, len(result.Sources))
				for _, source := range result.Sources {
					note := source.Source + "=" + source.Status
					if source.Message != "" {
						note += " (" + source.Message + ")"
					}
					parts = append(parts, note)
				}
				if _, err := fmt.Fprintf(w, "sources: %s\n", strings.Join(parts, ", ")); err != nil {
					return err
				}
			}
			if sc := result.SecurityCheck; sc != nil {
				if _, err := fmt.Fprintf(w, "securitycheck: %s%s\n", sc.Status, scMessageSuffix(sc)); err != nil {
					return err
				}
				if len(sc.Fields) > 0 {
					tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
					_, _ = fmt.Fprintln(tw, "Field\tValue")
					for _, field := range sc.Fields {
						_, _ = fmt.Fprintf(tw, "%s\t%s\n", field.Name, field.Value)
					}
					if err := tw.Flush(); err != nil {
						return err
					}
				}
			}
		}
	}

	if report.Reachability.Enabled {
		if err := writeReachabilityConsole(w, report.Reachability); err != nil {
			return err
		}
	}

	if report.Mail.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Mail Ports]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Mail.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.Mail.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if len(report.Mail.Results) > 0 {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "Port\tStatus\tLatency\tMethod\tMessage")
			for _, item := range report.Mail.Results {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					defaultText(item.Title, fmt.Sprintf("%d", item.Port)),
					defaultText(item.Status, "unknown"),
					formatMaybeFloat(item.LatencyMs, "ms"),
					defaultText(item.Method, "-"),
					defaultText(item.Message, "-"),
				)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}

	if report.Media.Enabled {
		if _, err := fmt.Fprintln(w, "\n[Media]"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "status: %s\n", defaultText(report.Media.Status, "unknown")); err != nil {
			return err
		}
		if message := strings.TrimSpace(report.Media.Message); message != "" {
			if _, err := fmt.Fprintf(w, "message: %s\n", message); err != nil {
				return err
			}
		}
		if report.Media.Result != nil && len(report.Media.Result.Items) > 0 {
			if set := strings.TrimSpace(report.Media.Result.Set); set != "" {
				if _, err := fmt.Fprintf(w, "set: %s\n", set); err != nil {
					return err
				}
			}
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "Item\tIP\tRegion\tStatus\tMessage")
			for _, item := range report.Media.Result.Items {
				status := item.Status
				if item.RawStatus == "Restricted" {
					status = "restricted"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", defaultText(item.Title, item.ID), defaultText(item.IPVersion, "-"), defaultText(item.Region, "-"), defaultText(status, "unknown"), defaultText(item.Message, "-"))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
	}

	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultText(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func formatMaybeFloat(value float64, suffix string) string {
	if value <= 0 {
		return "-"
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, suffix)
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

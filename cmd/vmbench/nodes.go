package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

const maxSignatureDownloadBytes = 16 << 10

func runNodes(args []string) int {
	if len(args) == 0 {
		printNodesUsage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "list":
		return runNodesList(args[1:])
	case "verify":
		return runNodesVerify(args[1:])
	case "update":
		return runNodesUpdate(args[1:])
	case "health":
		return runNodesHealth(args[1:])
	case "help", "-h", "--help":
		printNodesUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown nodes command: %s\n\n", args[0])
		printNodesUsage(os.Stderr)
		return 2
	}
}

func printNodesUsage(w io.Writer) {
	fmt.Fprintln(w, strings.Join([]string{
		"Usage: vmbench nodes <command> [flags]",
		"",
		i18n.T("cli.usage.nodesCommands"),
		"  list      " + i18n.T("cli.usage.nodesList"),
		"  verify    " + i18n.T("cli.usage.nodesVerify"),
		"  update    " + i18n.T("cli.usage.nodesUpdate"),
		"  health    " + i18n.T("cli.usage.nodesHealth"),
		"",
		i18n.T("cli.usage.nodesCatalog"),
		"  --node-catalog embedded|auto|PATH",
		"  --node-revision REVISION",
	}, "\n"))
}

type nodeLoadFlags struct {
	source    string
	revision  string
	cachePath string
}

func addNodeLoadFlags(fs *flag.FlagSet, values *nodeLoadFlags) {
	fs.StringVar(&values.source, "node-catalog", nodecatalog.SourceEmbedded, i18n.T("cli.flag.nodeCatalogNodes"))
	fs.StringVar(&values.revision, "node-revision", "", i18n.T("cli.flag.nodeRevision"))
	fs.StringVar(&values.cachePath, "node-cache", "", i18n.T("cli.flag.nodeCacheNodes"))
}

func (values nodeLoadFlags) load() (nodecatalog.Loaded, error) {
	return nodecatalog.Load(nodecatalog.LoadOptions{
		Source:    values.source,
		Revision:  values.revision,
		CachePath: values.cachePath,
	})
}

func runNodesList(args []string) int {
	fs := flag.NewFlagSet("nodes list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var loadFlags nodeLoadFlags
	var asJSON bool
	var kind, family, region, city, carrier string
	addNodeLoadFlags(fs, &loadFlags)
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.StringVar(&kind, "kind", "", i18n.T("cli.flag.nodesKind"))
	fs.StringVar(&family, "ip-family", "", i18n.T("cli.flag.nodesFamily"))
	fs.StringVar(&region, "region", "", i18n.T("cli.flag.nodesRegion"))
	fs.StringVar(&city, "city", "", i18n.T("cli.flag.nodesCity"))
	fs.StringVar(&carrier, "carrier", "", i18n.T("cli.flag.nodesCarrier"))
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes list [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.nodesNoPositional"))
		return 2
	}
	if err := validateNodeFilter(kind, family); err != nil {
		printErr(err)
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		printErr(err)
		return 1
	}
	nodes := loaded.Manifest.Select(nodecatalog.Filter{Kind: kind, IPFamily: family, Region: region, City: city, Carrier: carrier})
	if asJSON {
		return writeNodeJSON(os.Stdout, struct {
			SchemaVersion int                `json:"schema_version"`
			Revision      string             `json:"revision"`
			GeneratedAt   time.Time          `json:"generated_at"`
			ExpiresAt     time.Time          `json:"expires_at"`
			Source        string             `json:"source"`
			Path          string             `json:"path,omitempty"`
			Warning       string             `json:"warning,omitempty"`
			Nodes         []nodecatalog.Node `json:"nodes"`
		}{
			SchemaVersion: loaded.Manifest.SchemaVersion,
			Revision:      loaded.Manifest.Revision,
			GeneratedAt:   loaded.Manifest.GeneratedAt,
			ExpiresAt:     loaded.Manifest.ExpiresAt,
			Source:        loaded.Source,
			Path:          loaded.Path,
			Warning:       loaded.Warning,
			Nodes:         nodes,
		})
	}
	writeCatalogNotice(os.Stderr, loaded)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join([]string{i18n.T("cli.nodes.colID"), i18n.T("cli.nodes.colKind"), i18n.T("cli.nodes.colCity"), i18n.T("cli.nodes.colCarrier"), i18n.T("cli.nodes.colASN"), i18n.T("cli.nodes.colIP"), i18n.T("cli.nodes.colProtocol"), i18n.T("cli.nodes.colEndpoint"), i18n.T("cli.nodes.colTraffic")}, "\t"))
	for _, node := range nodes {
		traffic := "-"
		if node.TrafficBytes > 0 {
			traffic = formatBytes(uint64(node.TrafficBytes))
		}
		asn := "-"
		if node.ASN > 0 {
			asn = fmt.Sprintf("AS%d", node.ASN)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			node.ID, node.Kind, firstNonEmpty(node.City, "-"), firstNonEmpty(node.Carrier, "-"), asn,
			node.IPFamily, node.Protocol, nodecatalog.EndpointForDisplay(node), traffic)
	}
	if err := tw.Flush(); err != nil {
		return 1
	}
	return 0
}

func runNodesVerify(args []string) int {
	fs := flag.NewFlagSet("nodes verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var loadFlags nodeLoadFlags
	var signatureRef, publicKeyFile, publicKeyValue string
	var asJSON bool
	var timeout time.Duration
	addNodeLoadFlags(fs, &loadFlags)
	fs.StringVar(&signatureRef, "signature", "", i18n.T("cli.flag.signature"))
	fs.StringVar(&publicKeyFile, "public-key", "", i18n.T("cli.flag.publicKey"))
	fs.StringVar(&publicKeyValue, "public-key-value", "", i18n.T("cli.flag.publicKeyValue"))
	fs.DurationVar(&timeout, "timeout", 15*time.Second, i18n.T("cli.flag.signatureTimeout"))
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes verify [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || timeout <= 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.invalidNodesVerify"))
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		printErr(err)
		return 1
	}
	signed := strings.TrimSpace(signatureRef) != "" || strings.TrimSpace(publicKeyFile) != "" || strings.TrimSpace(publicKeyValue) != ""
	if signed && strings.TrimSpace(signatureRef) == "" {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.signatureRequired"))
		return 2
	}
	var key ed25519.PublicKey
	if signed {
		key, err = explicitPublicKey(publicKeyFile, publicKeyValue)
		if err != nil {
			printErr(err)
			return 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		signature, signatureErr := loadSignatureReference(ctx, signatureRef, http.DefaultClient)
		cancel()
		if signatureErr != nil {
			printErr(signatureErr)
			return 1
		}
		if err := nodecatalog.Verify(loaded.Raw, signature, key); err != nil {
			printErr(err)
			return 1
		}
	}
	if asJSON {
		return writeNodeJSON(os.Stdout, struct {
			Valid             bool   `json:"valid"`
			SignatureVerified bool   `json:"signature_verified"`
			Revision          string `json:"revision"`
			NodeCount         int    `json:"node_count"`
			Source            string `json:"source"`
			Warning           string `json:"warning,omitempty"`
		}{true, signed, loaded.Manifest.Revision, len(loaded.Manifest.Nodes), loaded.Source, loaded.Warning})
	}
	verification := "schema/revision"
	if signed {
		verification += "/signature"
	}
	fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.nodes.catalogValid", map[string]any{"Revision": loaded.Manifest.Revision, "Count": len(loaded.Manifest.Nodes), "Verified": verification}))
	writeCatalogNotice(os.Stderr, loaded)
	return 0
}

func runNodesUpdate(args []string) int {
	fs := flag.NewFlagSet("nodes update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var manifestURL, signatureRef, publicKeyFile, publicKeyValue, destination string
	var timeout time.Duration
	var asJSON bool
	fs.StringVar(&manifestURL, "url", "", i18n.T("cli.flag.updateURL"))
	fs.StringVar(&signatureRef, "signature", "", i18n.T("cli.flag.signature"))
	fs.StringVar(&publicKeyFile, "public-key", "", i18n.T("cli.flag.publicKey"))
	fs.StringVar(&publicKeyValue, "public-key-value", "", i18n.T("cli.flag.publicKeyValue"))
	fs.StringVar(&destination, "cache", "", i18n.T("cli.flag.updateCache"))
	fs.DurationVar(&timeout, "timeout", 30*time.Second, i18n.T("cli.flag.updateTimeout"))
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: vmbench nodes update --url URL --signature PATH|URL (--public-key PATH|--public-key-value KEY) [flags]")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(manifestURL) == "" || strings.TrimSpace(signatureRef) == "" || timeout <= 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.updateRequires"))
		return 2
	}
	key, err := explicitPublicKey(publicKeyFile, publicKeyValue)
	if err != nil {
		printErr(err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	options := nodecatalog.UpdateOptions{
		ManifestURL: manifestURL,
		PublicKey:   key,
		Destination: destination,
		Client:      http.DefaultClient,
	}
	if isHTTPReference(signatureRef) {
		options.SignatureURL = signatureRef
	} else {
		options.Signature, err = nodecatalog.ReadSignature(signatureRef)
		if err != nil {
			printErr(err)
			return 1
		}
	}
	loaded, err := nodecatalog.Update(ctx, options)
	if err != nil {
		printErr(err)
		return 1
	}
	if asJSON {
		return writeNodeJSON(os.Stdout, struct {
			Updated   bool   `json:"updated"`
			Revision  string `json:"revision"`
			NodeCount int    `json:"node_count"`
			Path      string `json:"path"`
			Warning   string `json:"warning,omitempty"`
		}{true, loaded.Manifest.Revision, len(loaded.Manifest.Nodes), loaded.Path, loaded.Warning})
	}
	fmt.Fprintf(os.Stdout, "%s\n", i18n.Tf("cli.nodes.catalogUpdated", map[string]any{"Revision": loaded.Manifest.Revision, "Count": len(loaded.Manifest.Nodes), "Path": loaded.Path}))
	writeCatalogNotice(os.Stderr, loaded)
	return 0
}

func runNodesHealth(args []string) int {
	fs := flag.NewFlagSet("nodes health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerLangFlag(fs)
	var loadFlags nodeLoadFlags
	var asJSON bool
	var kind, family, region, city, carrier string
	var timeout time.Duration
	var concurrency int
	addNodeLoadFlags(fs, &loadFlags)
	fs.BoolVar(&asJSON, "json", false, i18n.T("cli.flag.jsonOutput"))
	fs.StringVar(&kind, "kind", "", i18n.T("cli.flag.nodesKindShort"))
	fs.StringVar(&family, "ip-family", "", i18n.T("cli.flag.nodesFamilyShort"))
	fs.StringVar(&region, "region", "", i18n.T("cli.flag.nodesRegion"))
	fs.StringVar(&city, "city", "", i18n.T("cli.flag.nodesCity"))
	fs.StringVar(&carrier, "carrier", "", i18n.T("cli.flag.nodesCarrier"))
	fs.DurationVar(&timeout, "timeout", 5*time.Second, i18n.T("cli.flag.healthTimeout"))
	fs.IntVar(&concurrency, "concurrency", 8, i18n.T("cli.flag.healthConcurrency"))
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes health [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || timeout <= 0 || concurrency < 1 || concurrency > 32 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.healthRange"))
		return 2
	}
	if err := validateNodeFilter(kind, family); err != nil {
		printErr(err)
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		printErr(err)
		return 1
	}
	filter := nodecatalog.Filter{Kind: kind, IPFamily: family, Region: region, City: city, Carrier: carrier}
	if len(loaded.Manifest.Select(filter)) == 0 {
		fmt.Fprintln(os.Stderr, i18n.T("cli.error.noNodesMatch"))
		return 2
	}
	results := nodecatalog.CheckHealth(context.Background(), loaded.Manifest, nodecatalog.HealthOptions{
		Timeout: timeout, Concurrency: concurrency, Filter: filter,
	})
	failed := 0
	for _, result := range results {
		if result.Status != "ok" {
			failed++
		}
	}
	if asJSON {
		if code := writeNodeJSON(os.Stdout, struct {
			Revision string                     `json:"revision"`
			Source   string                     `json:"source"`
			Healthy  int                        `json:"healthy"`
			Failed   int                        `json:"failed"`
			Results  []nodecatalog.HealthResult `json:"results"`
		}{loaded.Manifest.Revision, loaded.Source, len(results) - failed, failed, results}); code != 0 {
			return code
		}
	} else {
		writeCatalogNotice(os.Stderr, loaded)
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join([]string{i18n.T("cli.nodes.colNode"), i18n.T("cli.nodes.colStatus"), i18n.T("cli.nodes.colMethod"), i18n.T("cli.nodes.colLatency"), i18n.T("cli.nodes.colDetail")}, "\t"))
		for _, result := range results {
			detail := result.Endpoint
			if result.Error != "" {
				detail = result.Error
			} else if result.HTTPStatus > 0 {
				detail = fmt.Sprintf("HTTP %d", result.HTTPStatus)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", result.NodeID, result.Status, result.Method, result.Latency.Round(time.Millisecond), detail)
		}
		if err := tw.Flush(); err != nil {
			return 1
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func explicitPublicKey(file, value string) (ed25519.PublicKey, error) {
	file = strings.TrimSpace(file)
	value = strings.TrimSpace(value)
	if file != "" && value != "" {
		return nil, fmt.Errorf("use only one of --public-key or --public-key-value")
	}
	if file != "" {
		return nodecatalog.ReadPublicKey(file)
	}
	if value != "" {
		return nodecatalog.ParsePublicKey([]byte(value))
	}
	return nil, fmt.Errorf("an explicit --public-key or --public-key-value is required")
}

func loadSignatureReference(ctx context.Context, reference string, client *http.Client) ([]byte, error) {
	if !isHTTPReference(reference) {
		return nodecatalog.ReadSignature(reference)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reference, nil)
	if err != nil {
		return nil, fmt.Errorf("load signature: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("load signature: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("load signature: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("load signature: %w", err)
	}
	if len(data) > maxSignatureDownloadBytes {
		return nil, fmt.Errorf("load signature: response exceeds %d bytes", maxSignatureDownloadBytes)
	}
	return data, nil
}

func isHTTPReference(reference string) bool {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validateNodeFilter(kind, family string) error {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "all", nodecatalog.KindDownload, nodecatalog.KindUpload, nodecatalog.KindRoute, nodecatalog.KindPing, nodecatalog.KindRoutePing:
	default:
		return fmt.Errorf("unknown node kind %q", kind)
	}
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "", "v4", "v6", "dual", "any":
	default:
		return fmt.Errorf("unknown IP family %q", family)
	}
	return nil
}

func writeCatalogNotice(w io.Writer, loaded nodecatalog.Loaded) {
	fmt.Fprintf(w, "%s\n", i18n.Tf("cli.nodes.catalogNotice", map[string]any{"Revision": loaded.Manifest.Revision, "Source": loaded.Source}))
	if loaded.Warning != "" {
		fmt.Fprintf(w, "%s\n", i18n.Tf("cli.nodes.catalogWarning", map[string]any{"Warning": loaded.Warning}))
	}
}

func writeNodeJSON(w io.Writer, value any) int {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
		return 1
	}
	return 0
}

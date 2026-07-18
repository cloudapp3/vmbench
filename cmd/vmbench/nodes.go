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
		"Commands:",
		"  list      list nodes from a selected catalog",
		"  verify    validate schema/revision and optionally a detached signature",
		"  update    download and cache a mandatory Ed25519-signed catalog",
		"  health    run lightweight HEAD/DNS/TCP availability checks",
		"",
		"Catalog selection:",
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
	fs.StringVar(&values.source, "node-catalog", nodecatalog.SourceEmbedded, "catalog source: embedded, auto, or JSON path")
	fs.StringVar(&values.revision, "node-revision", "", "require an exact catalog revision")
	fs.StringVar(&values.cachePath, "node-cache", "", "override auto catalog cache path")
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
	var loadFlags nodeLoadFlags
	var asJSON bool
	var kind, family, region, city, carrier string
	addNodeLoadFlags(fs, &loadFlags)
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.StringVar(&kind, "kind", "", "filter kind: download, upload, route, ping, or route_ping")
	fs.StringVar(&family, "ip-family", "", "filter IP family: v4, v6, dual, or any")
	fs.StringVar(&region, "region", "", "filter region")
	fs.StringVar(&city, "city", "", "filter city")
	fs.StringVar(&carrier, "carrier", "", "filter carrier")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes list [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "error: nodes list does not accept positional arguments")
		return 2
	}
	if err := validateNodeFilter(kind, family); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	fmt.Fprintln(tw, "ID\tKIND\tCITY\tCARRIER\tASN\tIP\tPROTOCOL\tENDPOINT\tTRAFFIC")
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
	var loadFlags nodeLoadFlags
	var signatureRef, publicKeyFile, publicKeyValue string
	var asJSON bool
	var timeout time.Duration
	addNodeLoadFlags(fs, &loadFlags)
	fs.StringVar(&signatureRef, "signature", "", "detached signature path or URL")
	fs.StringVar(&publicKeyFile, "public-key", "", "Ed25519 public key file")
	fs.StringVar(&publicKeyValue, "public-key-value", "", "inline base64, hex, or PEM Ed25519 public key")
	fs.DurationVar(&timeout, "timeout", 15*time.Second, "signature download timeout")
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes verify [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || timeout <= 0 {
		fmt.Fprintln(os.Stderr, "error: invalid nodes verify arguments")
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	signed := strings.TrimSpace(signatureRef) != "" || strings.TrimSpace(publicKeyFile) != "" || strings.TrimSpace(publicKeyValue) != ""
	if signed && strings.TrimSpace(signatureRef) == "" {
		fmt.Fprintln(os.Stderr, "error: --signature is required when a public key is provided")
		return 2
	}
	var key ed25519.PublicKey
	if signed {
		key, err = explicitPublicKey(publicKeyFile, publicKeyValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		signature, signatureErr := loadSignatureReference(ctx, signatureRef, http.DefaultClient)
		cancel()
		if signatureErr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", signatureErr)
			return 1
		}
		if err := nodecatalog.Verify(loaded.Raw, signature, key); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	fmt.Fprintf(os.Stdout, "node catalog %s is valid (%d nodes, %s verified)\n", loaded.Manifest.Revision, len(loaded.Manifest.Nodes), verification)
	writeCatalogNotice(os.Stderr, loaded)
	return 0
}

func runNodesUpdate(args []string) int {
	fs := flag.NewFlagSet("nodes update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var manifestURL, signatureRef, publicKeyFile, publicKeyValue, destination string
	var timeout time.Duration
	var asJSON bool
	fs.StringVar(&manifestURL, "url", "", "node catalog manifest URL")
	fs.StringVar(&signatureRef, "signature", "", "detached signature path or URL")
	fs.StringVar(&publicKeyFile, "public-key", "", "Ed25519 public key file")
	fs.StringVar(&publicKeyValue, "public-key-value", "", "inline base64, hex, or PEM Ed25519 public key")
	fs.StringVar(&destination, "cache", "", "cache destination (default user cache)")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "complete update timeout")
	fs.BoolVar(&asJSON, "json", false, "output JSON")
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
		fmt.Fprintln(os.Stderr, "error: --url, --signature, and a positive --timeout are required")
		return 2
	}
	key, err := explicitPublicKey(publicKeyFile, publicKeyValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	loaded, err := nodecatalog.Update(ctx, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	fmt.Fprintf(os.Stdout, "updated node catalog %s (%d nodes) at %s\n", loaded.Manifest.Revision, len(loaded.Manifest.Nodes), loaded.Path)
	writeCatalogNotice(os.Stderr, loaded)
	return 0
}

func runNodesHealth(args []string) int {
	fs := flag.NewFlagSet("nodes health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var loadFlags nodeLoadFlags
	var asJSON bool
	var kind, family, region, city, carrier string
	var timeout time.Duration
	var concurrency int
	addNodeLoadFlags(fs, &loadFlags)
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.StringVar(&kind, "kind", "", "filter kind")
	fs.StringVar(&family, "ip-family", "", "filter IP family")
	fs.StringVar(&region, "region", "", "filter region")
	fs.StringVar(&city, "city", "", "filter city")
	fs.StringVar(&carrier, "carrier", "", "filter carrier")
	fs.DurationVar(&timeout, "timeout", 5*time.Second, "timeout per node")
	fs.IntVar(&concurrency, "concurrency", 8, "concurrent checks (1-32)")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: vmbench nodes health [flags]") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || timeout <= 0 || concurrency < 1 || concurrency > 32 {
		fmt.Fprintln(os.Stderr, "error: timeout must be positive and concurrency must be between 1 and 32")
		return 2
	}
	if err := validateNodeFilter(kind, family); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	loaded, err := loadFlags.load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	filter := nodecatalog.Filter{Kind: kind, IPFamily: family, Region: region, City: city, Carrier: carrier}
	if len(loaded.Manifest.Select(filter)) == 0 {
		fmt.Fprintln(os.Stderr, "error: no nodes match the selected health filters")
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
		fmt.Fprintln(tw, "NODE\tSTATUS\tMETHOD\tLATENCY\tDETAIL")
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
	fmt.Fprintf(w, "catalog: %s (%s)\n", loaded.Manifest.Revision, loaded.Source)
	if loaded.Warning != "" {
		fmt.Fprintf(w, "warning: %s\n", loaded.Warning)
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

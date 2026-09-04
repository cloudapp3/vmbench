//go:build ignore

// gen_isp_nodes regenerates the China carrier speed-test nodes embedded in
// nodecatalog/nodes.json from the MIT-licensed datasets published by the ecs
// project:
//
//	https://github.com/spiritLHLS/speedtest.cn-CN-ID   (direct HTTP endpoints)
//	https://github.com/spiritLHLS/speedtest.net-CN-ID  (Ookla server IDs)
//
// Usage:
//
//	go run scripts/gen_isp_nodes.go [-out nodes.json] [-traffic 52428800]
//
// It prints isp_download node entries as JSON and, unless -out is given,
// prints the per-carrier Ookla server ID table for bench/netio/nodes.go.
// The output is a maintainer aid: review it, merge it into the embedded
// snapshot, bump the manifest revision, and run `go test ./...`.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	cnBase  = "https://raw.githubusercontent.com/spiritLHLS/speedtest.cn-CN-ID/main"
	netBase = "https://raw.githubusercontent.com/spiritLHLS/speedtest.net-CN-ID/main"
)

// cityTranslations maps the Chinese city names used by speedtest.cn onto the
// English city convention used by the embedded catalog.
var cityTranslations = map[string]string{
	"南京": "Nanjing", "苏州": "Suzhou", "杭州": "Hangzhou", "宁波": "Ningbo",
	"武汉": "Wuhan", "新乡": "Xinxiang", "福州": "Fuzhou", "大连": "Dalian",
	"太原市": "Taiyuan", "成都": "Chengdu", "绵阳": "Mianyang", "北京": "Beijing",
	"上海": "Shanghai", "广州": "Guangzhou", "深圳": "Shenzhen", "济南": "Jinan",
	"天津": "Tianjin", "重庆": "Chongqing", "长沙": "Changsha", "西安": "Xian",
}

type carrierInfo struct {
	label  string
	cnCSV  string
	netCSV string
	ids    []string
}

var carriers = []carrierInfo{
	{label: "China Telecom", cnCSV: "telecom.csv", netCSV: "CN_Telecom.csv", ids: []string{"电信"}},
	{label: "China Unicom", cnCSV: "unicom.csv", netCSV: "CN_Unicom.csv", ids: []string{"联通"}},
	{label: "China Mobile", cnCSV: "mobile.csv", netCSV: "CN_Mobile.csv", ids: []string{"移动"}},
}

type node struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Region       string `json:"region"`
	City         string `json:"city"`
	Carrier      string `json:"carrier"`
	IPFamily     string `json:"ip_family"`
	Protocol     string `json:"protocol"`
	Endpoint     string `json:"endpoint"`
	Port         int    `json:"port"`
	URL          string `json:"url"`
	TrafficBytes int64  `json:"traffic_bytes"`
	Source       string `json:"source"`
}

func main() {
	out := flag.String("out", "", "write generated isp_download nodes into this catalog JSON (updates revision)")
	traffle := flag.Int64("traffic", 52428800, "per-node traffic budget in bytes")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}

	var generated []node
	for index, carrier := range carriers {
		rows := fetchCSV(client, cnBase+"/"+carrier.cnCSV)
		records := parseCSV(rows)
		if len(records) == 0 {
			fmt.Fprintf(os.Stderr, "warning: no records for %s\n", carrier.label)
			continue
		}
		slug := []string{"telecom", "unicom", "mobile"}[index]
		seen := map[string]bool{}
		for _, record := range records {
			if len(record) < 22 || record[1] != "1" || !strings.HasPrefix(record[21], "http") {
				continue
			}
			city := translate(record[7])
			if seen[city] {
				continue
			}
			seen[city] = true
			parsed, err := url.Parse(record[21])
			if err != nil || parsed.Host == "" {
				continue
			}
			port := 80
			if parsed.Port() != "" {
				port = 0
				fmt.Sscanf(parsed.Port(), "%d", &port)
			}
			generated = append(generated, node{
				ID:           fmt.Sprintf("isp-cn-%s-%s-%s", slug, strings.ToLower(city), record[0]),
				Name:         fmt.Sprintf("%s %s (speedtest.cn %s)", carrier.label, city, record[0]),
				Kind:         "isp_download",
				Region:       "China",
				City:         city,
				Carrier:      slug,
				IPFamily:     "v4",
				Protocol:     parsed.Scheme,
				Endpoint:     parsed.Hostname(),
				Port:         port,
				URL:          record[21],
				TrafficBytes: *traffle,
				Source:       "speedtest.cn-CN-ID",
			})
			if countCities(seen) >= 4 {
				break
			}
		}
	}

	if *out != "" {
		writeCatalog(*out, generated)
		return
	}

	encoded, _ := json.MarshalIndent(generated, "", "  ")
	fmt.Println(string(encoded))
	fmt.Fprintln(os.Stderr, "\nOokla per-carrier server IDs (update ooklaCarrierServers in bench/netio/nodes.go):")
	for _, carrier := range carriers {
		rows := parseCSV(fetchCSV(client, netBase+"/"+carrier.netCSV))
		ids := make([]string, 0, len(rows))
		for _, record := range rows {
			if len(record) >= 1 {
				ids = append(ids, record[0])
			}
		}
		fmt.Fprintf(os.Stderr, "  %-14s %v\n", carrier.label, ids)
	}
}

func fetchCSV(client *http.Client, endpoint string) []string {
	resp, err := client.Get(endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: fetch %s: %v\n", endpoint, err)
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: read %s: %v\n", endpoint, err)
		return nil
	}
	return strings.Split(string(body), "\n")
}

func parseCSV(lines []string) [][]string {
	var records [][]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "id,") {
			continue
		}
		if record, err := csv.NewReader(strings.NewReader(line)).Read(); err == nil {
			records = append(records, record)
		}
	}
	return records
}

func translate(city string) string {
	if mapped, ok := cityTranslations[city]; ok {
		return mapped
	}
	return city
}

func countCities(seen map[string]bool) int { return len(seen) }

func writeCatalog(path string, generated []node) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read %s: %v\n", path, err)
		os.Exit(1)
	}
	var manifest struct {
		SchemaVersion int    `json:"schema_version"`
		Revision      string `json:"revision"`
		GeneratedAt   string `json:"generated_at"`
		ExpiresAt     string `json:"expires_at"`
		Nodes         []node `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "error: decode %s: %v\n", path, err)
		os.Exit(1)
	}
	kept := make([]node, 0, len(manifest.Nodes))
	for _, existing := range manifest.Nodes {
		if existing.Kind != "isp_download" {
			kept = append(kept, existing)
		}
	}
	manifest.Nodes = append(kept, generated...)
	manifest.Revision = time.Now().UTC().Format("2006-01-02.1")
	manifest.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	manifest.ExpiresAt = time.Now().UTC().AddDate(1, 0, 0).Format(time.RFC3339)

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: encode: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %d nodes (%d isp_download) to %s at revision %s\n",
		len(manifest.Nodes), len(generated), path, manifest.Revision)
}

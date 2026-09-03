# vmbench

Cross-platform VPS benchmark suite written in Go, with TUI interface.

[![CI](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cloudapp3/vmbench.svg)](https://pkg.go.dev/github.com/cloudapp3/vmbench)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Documentation: [中文说明](docs/README.zh-CN.md) · [Product](docs/product.md) · [Tech stack](docs/tech-stack.md) · [Current state](docs/current-state.md) · [Docs source](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs)

[Quick start](#quick-start) · [Why vmbench](#why-vmbench) · [Commands](#commands) · [VPS Suite](#vps-suite) · [Reports](#report-formats) · [Community](#community--support)

Hardware benchmarks use **external tools only**. Defaults are sysbench/fio/OpenSSL on Linux, OpenSSL on macOS, and WinSAT on Windows; dd, STREAM, mbw, Geekbench, and the remaining adapters are opt-in. Missing tools are reported as structured errors; vmbench does not fall back to in-process benchmark code.

vmbench reports raw metrics and structured diagnostics. It does **not** output a benchmark total score, grade, or category score.

## Quick Start

```bash
# Install from source
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest

# Or build locally
go build -ldflags "-X github.com/cloudapp3/vmbench.Version=$(git describe --tags 2>/dev/null || echo dev)" -o vmbench ./cmd/vmbench/

# Run (opens TUI by default)
./vmbench

# Or use CLI directly
./vmbench run
./vmbench run --json report.json --html report.html
./vmbench run --filter 'sysbench|fio'
./vmbench run --hardware-tool sysbench,openssl,fio,dd
./vmbench run --scope network --iterations 1
./vmbench run --scope all --iterations 1

# VPS scenario suite (one-command default, ECS-like sections)
./vmbench suite
./vmbench suite --preset quick
./vmbench suite --preset website --json suite.json --html suite.html
./vmbench suite --only ping,mail
./vmbench suite --speed-provider cloudflare,speedtest_net
./vmbench suite --only hardware --hardware-tool dd,stream,mbw

# Versioned network nodes and reproducible Suite evidence
./vmbench nodes list --node-catalog embedded
./vmbench nodes health --node-catalog auto --ip-family v6
./vmbench suite --node-catalog auto --node-revision 2026-07-13.1 --save-history
./vmbench history compare --last 3

# Expose vmbench to LLM clients through MCP stdio
./vmbench mcp serve --transport stdio
```

## Why vmbench

vmbench is built for VPS buyers, hosting reviewers, SREs, and server operators who need reproducible benchmark evidence instead of a single opaque score.

Use vmbench when you need to:

- measure CPU / memory / disk with standard external tools such as sysbench, fio, and openssl
- diagnose VPS network quality with route, ping, speed, IP quality, mail, and media probes
- record virtualization, public IPv4/IPv6, ASN/provider, conservative NAT evidence, and website/Telegram reachability
- run network probes against a versioned node catalog whose revision and node IDs remain in the report
- export JSON / HTML reports for automation, sharing, or long-term comparison
- compare benchmark or Suite reports using compatible raw time, throughput, and latency deltas
- keep local report history with restrictive Unix permissions and compare the latest N runs
- keep benchmark output transparent by avoiding immature total scores and grades
- expose bounded local benchmark tools to LLM clients through MCP stdio

## Preview

vmbench has three primary ways to inspect a host:

- **CLI** for scripts and repeatable benchmark runs
- **TUI** for interactive local runs and result browsing
- **Reports** for machine-readable JSON and shareable HTML output

Screenshots and richer docs are being staged in the external docs repository: [cloudapp3/vmdocs/sites/vmbench/docs](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs).

## Install from GitHub Releases

After the first tagged release is published:

```bash
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash
```

Other install options:

```bash
# Install a specific release tag
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --version v0.1.0

# Custom directory
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --dir /opt/bin
```

## Commands

| Command | Description |
|---------|-------------|
| `vmbench` | Launch interactive TUI (default) |
| `vmbench run [flags]` | Run isolated external hardware benchmarks; network scope is explicit |
| `vmbench suite [flags]` | Run VPS composite suite: hardware / network identity / route / ping / speed / IP quality / reachability / mail / media |
| `vmbench nodes <command>` | List, verify, update, or health-check the versioned network node catalog |
| `vmbench mcp serve [--transport stdio]` | Expose vmbench MCP tools for LLM clients |
| `vmbench list` | List available workloads |
| `vmbench sysinfo [--json]` | Show system information |
| `vmbench compare <a.json> <b.json> [...]` | Auto-detect and compare benchmark or Suite reports |
| `vmbench history <command>` | Add, list, show, delete, or compare local reports |
| `vmbench version` | Show version |

## CLI Flags (run)

| Flag | Default | Description |
|------|---------|-------------|
| `--iterations` | 3 | Iterations per workload (1-9) |
| `--filter` | (all) | Regex to select workloads |
| `--disk-path` | OS temp | Temp directory for disk tests |
| `--timeout` | 5m | Per-workload timeout |
| `--mode` | single | Compatibility flag; legacy `multi`/`all` values run the catalog once because each tool defines its own concurrency |
| `--scope` | hardware | `hardware`, `network`, or `all`; network/all can transfer about 1.75 GB |
| `--hardware-tool` | platform-specific | External hardware tools: sysbench, openssl, fio, dd, stream, mbw, geekbench, winsat, or all |
| `--iperf-host` | | iperf3 server for network test (comma-separated) |
| `--node-catalog` | embedded | `embedded`, `auto`, or an explicit JSON path; used by network/all scope |
| `--node-revision` | | Require an exact catalog revision before probes start |
| `--node-cache` | platform user cache | Override the cache path used with `--node-catalog auto` |
| `--json` | | Write JSON report to file |
| `--html` | | Write HTML report to file |
| `--quiet` | false | Suppress progress output |
| `--save-history` | false | Save the generated report to the local history store |
| `--history-tag` | | Optional history label; requires `--save-history` |

## VPS Suite

`vmbench suite` keeps the YABS-style one-command experience while using ECS-style modular sections. It still reports raw metrics and structured diagnostics, not a benchmark total score.

Sections:

| Section | Purpose |
|---------|---------|
| `hardware` | CPU / memory / disk benchmark report |
| `network_info` | Virtualization plus public IPv4/IPv6, ASN/provider/location, and conservative NAT evidence |
| `route` | Versioned China carrier/CERNET/CSTNET IPv4/IPv6 route diagnostics with destination-reached evidence |
| `ping` | Versioned China carrier/CERNET/CSTNET IPv4/IPv6 TCP latency / jitter / loss and connection state |
| `speed` | Cloudflare / speedtest provider download/upload measurements |
| `ip_quality` | IP reputation and risk diagnosis |
| `reachability` | Website HTTPS and Telegram DC TCP reachability with latency/status/error |
| `mail` | Sequential mail-port reachability with open/refused/timeout/error states |
| `media` | Streaming / unlock probes |

Presets:

| Preset | Sections | Scenario |
|--------|----------|----------|
| `quick` | `hardware,network_info,speed,ip_quality` | Fast VPS overview |
| `website` | `hardware,network_info,route,ping,speed,ip_quality,reachability,mail` | Website/server hosting |
| `proxy` | `network_info,route,ping,speed,ip_quality,reachability,media` | Proxy / unlock |
| `mail` | `network_info,route,ip_quality,mail` | Mail server |

Speed providers:

| Provider | Meaning |
|----------|---------|
| `cloudflare` | Cloudflare download + upload |
| `speedtest_net` | Ookla Speedtest CLI JSON |
| `speedtest_cn` | speedtest.cn compatible CLI JSON |
| `iperf3` | user-provided iperf3 host |

Common flags:

```bash
vmbench suite --preset quick
vmbench suite --preset proxy --ip-version dual
vmbench suite --only ping,mail
vmbench suite --skip media
vmbench suite --route-presets gz,bj,sh,cd,cernet,cstnet
vmbench suite --speed-provider cloudflare,speedtest_net
vmbench suite --speed-provider iperf3 --iperf-host 1.2.3.4
vmbench suite --only hardware --hardware-tool sysbench,openssl,fio,dd
vmbench suite --only hardware --hardware-tool geekbench
vmbench suite --node-catalog embedded --node-revision 2026-07-13.1
vmbench suite --node-catalog auto --save-history --history-tag weekly
vmbench suite --quiet --json suite.json
```

`--node-catalog` accepts `embedded`, `auto`, or a JSON path. `embedded` is the deterministic offline snapshot. `auto` uses the validated user cache when available and falls back to the embedded snapshot; it does not download during a benchmark. `--node-revision` pins an exact revision and fails before probes start if the selected snapshot differs. Reports retain the resolved catalog source/revision and selected node identities. A download node's `traffic_bytes` is enforced as the maximum response-body bytes read by that probe.

The `speed` section emits:

- `groups[]`: results grouped per provider
- `providers[]`: raw results for each individual probe
- `summary`: a single-provider summary when one provider is selected; with multiple providers it records `aggregation=best_per_metric` so results from different sources are not misrepresented as one node

Suite succeeds only when every enabled section ends with `status=ok`. An enabled section with an empty, `skipped`, `partial`, or `error` status makes the overall report fail; disabled sections alone emit `section.skip`. Selecting `iperf3` without a usable host is an error.

The default timeout remains 5 minutes. Hardware applies it per workload. Network identity, route, ping, speed, IP quality, reachability, mail, and media each derive a section timeout from the caller context; cancellation/deadline is recorded as a structured section error. By default the Suite CLI writes each section's running/final lifecycle to stderr; `--quiet` suppresses this progress stream without changing reports.

TCP Ping treats an accepted connection and a TCP RST/connection-refused response as received evidence: both contribute latency and do not count as packet loss. Each target records `connection_state=open|refused|mixed|no_response`, while a true timeout or other no-response failure remains loss. Route results record `resolved_target`, `destination_reached`, and `status=ok|partial|error`; valid hops that never reach the resolved destination are `partial`, not a successful trace. Mail probes run the built-in ports sequentially so the shared target is not hit with a connection burst, and classify each result as `open`, `refused`, `timeout`, or `error`; DNS failures remain probe errors rather than port timeouts.

### Node catalog

The embedded catalog is a versioned data snapshot, not executable code. Each entry carries a stable node ID plus region/city, carrier, ASN, IP family, protocol, endpoint, source, and traffic budget where applicable. The current snapshot extends route/ping evidence to Chengdu, CERNET, CSTNET, and IPv6 targets.

```bash
vmbench nodes list [--node-catalog embedded|auto|PATH] [--node-revision REV] [--json]
vmbench nodes verify [--node-catalog embedded|auto|PATH] [--signature FILE_OR_URL --public-key FILE]
vmbench nodes update --url URL --signature FILE_OR_URL --public-key FILE
vmbench nodes health [--node-catalog embedded|auto|PATH] [--ip-family v4|v6] [--json]
```

`nodes update` requires an explicit Ed25519 trust root, verifies the detached signature over the exact manifest bytes, validates the strict schema, and only then atomically replaces the cache (`0600` mode on Unix). A bad signature or invalid manifest leaves the existing cache untouched. `nodes health` is a bounded availability check and does not change benchmark results.

## MCP Server

`vmbench mcp serve --transport stdio` exposes vmbench as a local Model Context Protocol server for LLM clients. The MCP server reuses the same Go APIs as the CLI and returns structured JSON content; it does not allow arbitrary shell execution.

Available tools:

| Tool | Purpose |
|------|---------|
| `vmbench_capabilities` | List version, suite sections, presets, hardware tools, speed providers, and workloads |
| `vmbench_sysinfo` | Collect current host system information |
| `vmbench_run` | Run raw workload benchmarks; defaults to hardware scope and one iteration |
| `vmbench_suite` | Run VPS suite sections; defaults to hardware only so network diagnostics are opt-in |

Example MCP client config:

```json
{
  "mcpServers": {
    "vmbench": {
      "command": "/usr/local/bin/vmbench",
      "args": ["mcp", "serve", "--transport", "stdio"]
    }
  }
}
```

Safety defaults:

- stdout is reserved for MCP JSON-RPC; diagnostics go to stderr.
- `hardware_tools`, suite sections, presets, and speed providers are enum-limited.
- CLI, TUI, and MCP normalize the same Suite fields: iterations, timeout, hardware tools, speed providers, iperf hosts, IP version, sections, route selection, `catalog_source`, and `catalog_revision`.
- MCP catalog input accepts `embedded`, `auto`, or an explicit path; a pinned revision mismatch is rejected before any network probe starts.
- omitted `iterations` / `timeout_ms` default to 1 / 5 minutes; explicit non-positive or out-of-range values and invalid regex are rejected.
- enum arrays are validated as a whole: mixing a valid item with an unknown section/provider/tool/route preset rejects the tool call instead of silently dropping the unknown item.
- network tests run only when explicitly requested by run scope, suite preset, or `only`.
- validation errors return `isError=true` without starting a measurement. Measurement failures retain the full `structuredContent.report` and also set `isError=true`.
- results contain raw metrics and structured errors, not benchmark total scores or grades.

## Hardware Benchmark Architecture

vmbench hardware measurements are external-tool based. Go only orchestrates commands, parses output, and writes structured reports; CPU / memory / disk benchmark numbers are not produced by in-process Go algorithms.

| Tool | Test | How |
|------|------|-----|
| **sysbench** | CPU single-core + multi-core prime; memory read/write bandwidth + random-read latency | Linux default; PATH or executable-adjacent `binaries/` directory |
| **fio** | 4K random read/write Q1/Q32 IOPS + 1M sequential read/write Q1/Q8 throughput | Linux default; direct I/O with unique auto-cleaned files |
| **openssl speed** | AES-256-CBC + SHA256 throughput | Linux/macOS default; System PATH |
| **dd** | Disk sequential write/read throughput | Opt-in; Linux reads use `iflag=direct`, while platforms without a guaranteed uncached read fail closed and direct users to fio |
| **STREAM** | Memory bandwidth kernels | Opt-in via `--hardware-tool stream` |
| **mbw** | Memory copy bandwidth | Opt-in via `--hardware-tool mbw` |
| **Geekbench** | Upstream CPU single/multi score | Opt-in via `--hardware-tool geekbench`; not a vmbench total score |
| **WinSAT** | Windows CPU/memory/disk probes | Windows default; selectable via `--hardware-tool winsat` |
| **iperf3** | Network TCP bandwidth | System PATH / user-provided host |

Before `run` or a Suite with hardware enabled starts, vmbench reports unresolved external tools only when the current `--filter` selects at least one workload from that adapter; on Linux it also prints a Debian/Ubuntu package hint when known. The affected workloads are still kept as structured errors instead of being silently skipped or replaced with in-process benchmark implementations.

The default hardware set now emits separate rows for memory read bandwidth, memory write bandwidth, memory random-read latency, and eight fio disk probes: 4K random read/write at Q1/Q32 plus 1M sequential read/write at Q1/Q8. Reports still contain raw throughput, latency, detail, and error fields only; no disk, memory, category, or total score is generated.

Official source and release archives do not vendor third-party benchmark tools by default. Linux fallback binaries are only loaded from `binaries/` next to the resolved vmbench executable, never from the current working directory.

### Platform Strategy

| Platform | Hardware benchmark strategy |
|----------|-----------------------------|
| **Linux amd64/arm64** | Default sysbench/fio/OpenSSL; optional executable-adjacent `binaries/` fallback |
| **macOS** | Default OpenSSL; other Homebrew tools are opt-in |
| **Windows** | Default WinSAT; Geekbench and other adapters are opt-in |

## TUI Interface

Launch with `./vmbench` (no arguments). Features:

- **Dashboard**: External benchmark, suite, compare, and sysinfo entry points
- **Running**: Real-time progress with per-workload status
- **Results**: Raw timing, throughput, latency, detail/error tables
- **Suite Configuration**: The same normalized iterations, IP version, timeout, tools/providers, iperf, section, route, catalog source, and revision fields used by CLI/MCP
- **Compact terminals**: Suite config/running/results switch to focused or one-line summaries below 40 rows and fit an 80x24 terminal without horizontal overflow
- **Compare**: Side-by-side benchmark report comparison; Suite Compare is available through CLI/history
- **Themes**: Press `t` on Dashboard to cycle color themes; the selection is saved locally

Key bindings: `↑↓` navigate, `Enter` select, `t` cycle theme on Dashboard, `Tab` toggle view, `s` save, `Esc` back, `q` quit.

## Measurement Model

vmbench reports measured raw metrics instead of a synthetic total score:

- **Time**: median elapsed time across iterations.
- **Throughput**: workload-specific throughput such as MiB/s, MB/s, events/s, or Mbit/s.
- **Latency**: ns/op for latency-oriented workloads.
- **Details**: structured text from external/network probes when available.
- **Isolation**: workloads always run serially; sysbench/fio/OpenSSL/WinSAT arguments define single-thread, multi-thread, and queue-depth behavior.
- **Iterations**: hardware workloads use the requested 1-9 samples. Traffic-generating and diagnostic network workloads are capped at one real probe and report `iterations: 1`.
- **Legacy mode**: `--mode multi/all` remains accepted for compatibility, emits a warning, and runs the external catalog once.
- **Processed quantities**: `bytes_processed` / `ops_processed` are emitted only for explicitly identified cumulative bytes/operations. Rates, scores, latency, and unknown values are never guessed into these fields.

This keeps reports comparable without implying an immature global ranking formula.

## Report Formats

| Format | Output | Description |
|--------|--------|-------------|
| Console | Default terminal output | Workload table with time, throughput, latency, detail/error |
| JSON | `--json report.json` | Schema v2 with scope/optional iperf hosts, actual iterations, raw samples, optional cumulative processed quantities, metrics, detail, and errors |
| HTML | `--html report.html` | Visual report with system cards and measured workload tables |

Suite JSON uses a schema-v2 envelope with `report_kind`, `report_id`, app build metadata, system information, timestamps/duration, normalized config, section states, and catalog provenance while retaining legacy v1 fields for existing consumers. Route results include resolved-destination evidence and ping results include `connection_state` in addition to the actual `probe_protocol` / `probe_tool`; successful zero-valued ping latency, jitter, and loss remain explicit raw metrics. Suite HTML renders hardware workload metrics, network identity, route evidence, ping metrics/status, provider-level speed, IP quality, reachability, mail, media, warnings, and structured failures.

CLI `--json` and `--html` exports are written through a same-directory temporary file, synced, and renamed into place. Exported files use mode `0600` on Unix because reports can contain hostnames, public addresses, and route hops.

`vmbench compare` auto-detects benchmark versus Suite JSON and rejects mixed report kinds. Suite Compare aligns raw metrics across two or more reports, but calculates a delta only when unit, actual protocol/IP family, provider/probe tool, target/node identity, and required catalog revision are compatible; otherwise it preserves values and prints the incompatibility reason. Route metrics additionally require explicit `status=ok` and `destination_reached=true`; legacy route entries without destination evidence are unavailable for delta calculation. Mail latency is comparable only for `status=open`; refused, timeout, and error durations are not successful connection latency. `vmbench history add/list/show/delete` manages atomic local records (`0700` directory and `0600` files on Unix), `run`/`suite --save-history [--history-tag TAG]` saves directly, and `vmbench history compare --last N` compares the latest same-kind reports.

`run` exits with status 1 when no workload matches or any selected workload fails. Its JSON config records normalized `scope`, optional `iperf_hosts`, and network-only catalog source/revision/node IDs; hardware reports set `extensions=false`, while network/all set it to `true`. Suite returns a non-zero CLI exit unless every enabled section is `ok`; enabled empty/skipped/partial/error states all fail. No report or comparison produces a benchmark total score, grade, or category score.

## Platform Support

| Capability | Linux | macOS | Windows |
|------------|:-----:|:-----:|:-------:|
| `vmbench run` core CLI | ✅ | ✅ | ✅ |
| TUI | ✅ | ✅ | ✅ |
| `sysinfo` | ✅ | ✅ | ✅ |
| JSON / HTML reports | ✅ | ✅ | ✅ |
| Compare reports | ✅ | ✅ | ✅ |
| Default external hardware tools | ✅ | ⚠️ install via package manager | ⚠️ use platform tools |
| `winsat` adapter | ❌ | ❌ | ✅ |
| Suite network diagnostics | ✅ | ✅ | ⚠️ partial / environment-dependent |
| MCP stdio server | ✅ | ✅ | ✅ |

Network-related suite sections depend on local routing, DNS, firewall, IPv6, and sandbox permissions. Failures are recorded as structured errors instead of being hidden.

## Project Structure

```
vmbench/
├── bench/
│   ├── common/        # GC control, random source, stats, sink
│   ├── integer/       # Legacy in-process workloads (not registered for hardware suite)
│   ├── float/         # Legacy in-process workloads (not registered for hardware suite)
│   ├── memory/        # Legacy in-process workloads (not registered for hardware suite)
│   ├── diskio/        # Legacy in-process workloads (not registered for hardware suite)
│   ├── runner.go      # Multi-iteration executor with progress
│   └── workload.go    # Workload interface
├── catalog/
│   └── catalog.go     # External-tool workload registry + parsers
├── nodecatalog/       # Embedded/versioned node data, strict loader, Ed25519 update, health checks
├── history/           # Atomic local run/Suite report history
├── cmd/vmbench/
│   ├── main.go        # CLI entry (run/suite/nodes/history/mcp/list/sysinfo/compare/...)
│   ├── mcp.go         # MCP stdio server command
│   └── tui.go         # TUI entry
├── docs/
│   ├── product.md     # Product spec (Chinese)
│   ├── tech-stack.md  # Technical architecture (Chinese)
│   ├── tui-design.md  # TUI design document
│   ├── README.zh-CN.md # Chinese quick reference
│   └── CHANGELOG.md   # Project changelog
├── mcp/                # MCP JSON-RPC stdio server and tool dispatch
├── report/
│   ├── types.go       # Document / ResultEntry data structures
│   ├── console.go     # Console table output
│   ├── json.go        # JSON output
│   ├── html.go        # Visual HTML report
│   └── compare.go     # Side-by-side comparison output
├── suite/             # Nine-section VPS Suite, schema-v2 envelope, Console/JSON/HTML
├── suitecompare/      # Suite raw-metric alignment and compatibility gating
├── sysinfo/           # Cross-platform system info (CPU/GPU/Memory/OS/Disk/Network)
├── tui/
│   ├── app.go         # Main TUI model with page routing
│   ├── dashboard.go   # Main menu
│   ├── running.go     # Real-time progress
│   ├── results.go     # Result tables
│   ├── compare.go     # Report comparison
│   ├── styles.go      # Lip Gloss styles (categories, status)
│   └── keys.go        # Key bindings
├── run.go             # RunCore() orchestration
├── events.go          # Event types
├── emit.go            # Event emission helpers
├── options.go         # Options + normalization (Engine, Mode, IperfHosts)
├── types.go           # Package-level type aliases
└── version.go         # Version (build-time injected)
```

## Dependencies

### Runtime (Go)
| Dependency | Version | Purpose |
|-----------|---------|---------|
| charmbracelet/bubbletea | v1.3+ | TUI framework |
| charmbracelet/lipgloss | v1.1+ | TUI styling |
| charmbracelet/bubbles | v1.0+ | TUI components (keys) |
| shirou/gopsutil | v4.25 | System info collection |

### Runtime (external tools, auto-detected)
| Tool | Platforms | Required? |
|------|----------|-----------|
| sysbench | PATH or optional local Linux `binaries/` fallback | Default hardware tool |
| openssl | All | Default hardware tool |
| fio | PATH or optional local Linux `binaries/` fallback | Default hardware tool |
| dd | Unix-like systems | Optional hardware tool |
| stream | Linux/macOS when installed | Optional hardware tool |
| mbw | Linux when installed | Optional hardware tool |
| geekbench | Platforms supported by Geekbench | Optional hardware tool, not default |
| winsat | Windows | Optional hardware tool |
| iperf3 | All | Optional network tool, requires `--iperf-host` |

## Build from source

```bash
# Standard build
go build -ldflags "-X github.com/cloudapp3/vmbench.Version=$(git describe --tags 2>/dev/null || echo dev)" -o vmbench ./cmd/vmbench/

# Project build script: CGO-disabled build into a temp directory
# (override the destination with VMBENCH_OUTPUT_DIR)
./sh/build.sh

# Cross-compile
GOOS=linux GOARCH=arm64 go build -ldflags "..." -o vmbench-arm64 ./cmd/vmbench/
GOOS=darwin GOARCH=arm64 go build -ldflags "..." -o vmbench-mac ./cmd/vmbench/
```

`sh/build.sh` sets `CGO_ENABLED=0`, matching the static-build policy used by GoReleaser.

## Development

```bash
gofmt -w $(git ls-files '*.go')
go mod tidy
go test ./...
go vet ./...
go build -o vmbench ./cmd/vmbench
```

Useful smoke checks:

```bash
./vmbench list
./vmbench sysinfo --json
./vmbench run --filter 'SHA|AES' --iterations 1 --quiet --json /tmp/vmbench.json
./vmbench suite --only mail --json /tmp/vmbench-mail.json
```

## Documentation

- [Chinese quick reference](docs/README.zh-CN.md)
- [Current technical and product state](docs/current-state.md)
- [Product spec](docs/product.md)
- [Technical architecture](docs/tech-stack.md)
- [TUI design](docs/tui-design.md)
- [Changelog](docs/CHANGELOG.md)
- [External docs source](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs)

## License

[MIT](LICENSE)

## Community & Support

- 🐛 Found a bug or want a feature? [Open an issue](https://github.com/cloudapp3/vmbench/issues/new)
- 📚 Prefer to start with docs? See [Documentation](#documentation)
- 🤝 Want to contribute? Start with [CONTRIBUTING.md](CONTRIBUTING.md)
- 💬 Community chat: [VMPulse Telegram](https://t.me/VMPulse)

Feedback, benchmark reports, platform-specific failures, and external-tool parser fixes directly help improve vmbench.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

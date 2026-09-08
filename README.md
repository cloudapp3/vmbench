# vmbench

Cross-platform VPS benchmark suite written in Go, with a TUI interface.

[![CI](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cloudapp3/vmbench.svg)](https://pkg.go.dev/github.com/cloudapp3/vmbench)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

vmbench measures CPU / memory / disk with **external tools only** (sysbench, fio, OpenSSL by default on Linux; more opt-in), diagnoses VPS network quality (route, ping, speed, IP quality, mail, media unlock), and exports JSON / HTML reports for comparison and automation. It reports raw metrics and structured diagnostics — deliberately no total score or grade.

Documentation: [中文说明](docs/README.zh-CN.md) · [Full capability reference](docs/capabilities.md) · [Tech stack](docs/tech-stack.md) · [Changelog](docs/CHANGELOG.md)

[Quick start](#quick-start) · [Install](#install) · [Commands](#commands) · [Flags](#common-flags) · [VPS Suite](#vps-suite) · [TUI](#tui) · [Documentation](#documentation)

## Quick Start

```bash
# One-line install (Linux / macOS; latest GitHub Release, SHA-256 verified)
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash

vmbench                          # interactive TUI (default)
vmbench run                      # hardware benchmark via external tools
vmbench suite                    # VPS scenario suite, one command
vmbench suite --preset quick     # fast overview: hardware + network info + speed + IP quality
vmbench compare a.json b.json    # auto-detect and compare reports
vmbench update                   # self-update from GitHub Releases
```

## Install

The one-liner above downloads the release archive for your OS/arch, verifies its SHA-256 against `checksums.txt`, and installs to the first writable directory among `/usr/local/bin`, `~/.local/bin`, and `~/bin`.

```bash
# Specific release tag
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --version v0.1.0

# Custom directory
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --dir /opt/bin

# Go toolchain
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest
```

Windows: download `vmbench-<version>-windows-<arch>.zip` from [Releases](https://github.com/cloudapp3/vmbench/releases) (WinSAT provides the default hardware probes).

### Self-update

```bash
vmbench update                   # check and replace this binary in place
vmbench update --check           # report the latest release without installing
vmbench update --version v0.6.0  # pin / downgrade to a specific release
vmbench update --check --json    # machine-readable status for scripts
```

Downloads are SHA-256 verified against the release `checksums.txt`, then atomically renamed over the running executable. `GITHUB_TOKEN`/`GH_TOKEN` is honored for API rate limits; `deb`/`rpm` installs should prefer the package manager.

## Commands

| Command | Description |
|---------|-------------|
| `vmbench` | Launch interactive TUI (default) |
| `vmbench run [flags]` | Run external hardware benchmarks (hardware only; network diagnostics live in `suite`) |
| `vmbench suite [flags]` | Run the VPS composite suite |
| `vmbench nodes <command>` | List / verify / update / health-check the versioned node catalog |
| `vmbench mcp serve [--transport stdio]` | Expose vmbench tools to LLM clients via MCP stdio |
| `vmbench list` | List available workloads |
| `vmbench sysinfo [--json]` | Show system information |
| `vmbench compare <a.json> <b.json> [...]` | Auto-detect and compare benchmark or Suite reports |
| `vmbench history <command>` | Add / list / show / delete / compare local reports |
| `vmbench update [--check] [--version TAG]` | Self-update from GitHub Releases (SHA-256 verified) |
| `vmbench version` | Show version |

## Common Flags

| Flag | Applies to | Default | Description |
|------|-----------|---------|-------------|
| `--iterations` | run, suite | 3 | Iterations per hardware workload (1-9) |
| `--filter` | run | all | Regex to select workloads |
| `--hardware-tool` | run, suite | platform default | sysbench, openssl, fio, dd, stream, mbw, geekbench, winsat, or all |
| `--preset` | suite | — | Scenario preset: `quick`, `website`, `proxy`, `mail` |
| `--only` / `--skip` | suite | — | Select or skip sections |
| `--ip-version` | suite | v4 | `v4`, `v6`, or `dual` |
| `--speed-provider` | suite | cloudflare | cloudflare, speedtest_net, speedtest_cn, china_isp, speedtest_isp, iperf3 |
| `--ip-quality-source` | suite | builtin | builtin; opt-in `securitycheck` (external 18-database binary) |
| `--media-set` | suite | all | `globe`, `tw`, `hk`, `jp`, `kr`, `na`, `sa`, `eu`, `afr`, `sea`, `oce`, `ai`, or combinations |
| `--node-catalog` | suite | embedded | `embedded`, `auto`, or a JSON path |
| `--node-revision` | suite | — | Pin an exact catalog revision; fails before probes start on mismatch |
| `--iperf-host` | suite | — | iperf3 server for the iperf3 speed provider |
| `--json` / `--html` | run, suite | — | Write JSON / HTML report to file |
| `--quiet` | run, suite | false | Suppress progress output |
| `--save-history` | run, suite | false | Save the report to local history (`--history-tag` to label) |
| `--lang` | all | auto | `en` or `zh-CN` (also `VMBENCH_LANG`) |

`run` is hardware-only; all network diagnostics live in `vmbench suite`. Full flag tables: `vmbench <command> --help` or the [capability reference](docs/capabilities.md).

## VPS Suite

`vmbench suite` keeps the YABS-style one-command experience with ECS-style modular sections:

| Section | Purpose |
|---------|---------|
| `hardware` | CPU / memory / disk benchmark report |
| `network_info` | Virtualization, public IPs, ASN/provider, NAT evidence, BGP/RDAP ownership view |
| `route` | China carrier / CERNET / CSTNET route diagnostics with return-route line classification (163 / 9929 / 4837 / CN2 / CMIN2 / CMI) |
| `ping` | China carrier TCP latency / jitter / loss with connection state |
| `speed` | Cloudflare / speedtest download + upload; optional China carrier (三网) providers |
| `ip_quality` | IP reputation: ip-api.com, ipapi.is, DNSBL, mail ports; opt-in securityCheck |
| `reachability` | Website HTTPS and Telegram DC TCP reachability |
| `mail` | Sequential mail-port reachability (open / refused / timeout / error) |
| `media` | Streaming / AI platform unlock probes (200+ services via UnlockTests) |

Presets: `quick` (hardware, network_info, speed, ip_quality) · `website` (+ route, ping, reachability, mail) · `proxy` (network focus + media) · `mail`.

```bash
vmbench suite --preset proxy --ip-version dual
vmbench suite --only ping,mail
vmbench suite --route-presets gz,bj,sh,cd,cernet,cstnet
vmbench suite --speed-provider china_isp
vmbench suite --media-set jp,kr
vmbench suite --only hardware --hardware-tool geekbench
vmbench suite --node-catalog auto --save-history --history-tag weekly
```

Suite succeeds only when every enabled section ends `status=ok`; enabled empty / skipped / partial / error states all fail the run. Reports keep the resolved node catalog source/revision and selected node IDs.

## TUI

Launch with `vmbench` (no arguments):

- **Dashboard** with benchmark / suite / compare / sysinfo entry points; mouse clicks and wheel scrolling everywhere
- **Run Config**: iterations, hardware tools, and a workload filter with live planned-workload count and missing-tool preflight
- **Suite Config**: the same normalized fields as CLI/MCP, with a planned-duration summary and `1-9` section jumps
- **Results**: cards / grouped / flat views; `d` opens per-workload detail with metrics, samples, errors, and raw tool output
- **Compare picker**: browse history, view a record, or compare two — benchmark deltas in-TUI, suite via the same output as the CLI
- **Themes**: press `t` on Dashboard to cycle; the choice is saved locally

Keys: `?` help · `↑↓` navigate · `Enter` select · `Tab` switch view · `d` detail · `s` save · `Esc` back · `q` quit. Every page scrolls with `PgUp/PgDn`, `Home/End`, and the mouse wheel; fits an 80x24 terminal.

## Languages

CLI, TUI, and console/HTML report labels are localized in English and Simplified Chinese — select with `--lang`, `VMBENCH_LANG`, or the TUI config. JSON field names, status enums, section IDs, and workload names stay English in every locale.

## Platform Support

| Capability | Linux | macOS | Windows |
|------------|:-----:|:-----:|:-------:|
| CLI / TUI / JSON / HTML / compare | ✅ | ✅ | ✅ |
| Default hardware tools | ✅ sysbench / fio / openssl | ⚠️ openssl (others via package manager) | ⚠️ WinSAT |
| Suite network diagnostics | ✅ | ✅ | ⚠️ partial / environment-dependent |
| MCP stdio server | ✅ | ✅ | ✅ |

Network sections depend on local routing, DNS, firewall, IPv6, and sandbox permissions; failures are recorded as structured errors instead of being hidden.

## Documentation

| Topic | Where |
|-------|-------|
| Full capability reference (中文): CLI flags, suite, node catalog, MCP, report formats, Go API | [docs/capabilities.md](docs/capabilities.md) |
| 中文快速说明 | [docs/README.zh-CN.md](docs/README.zh-CN.md) |
| MCP tools, client config, safety defaults | [MCP 大模型接入](docs/capabilities.md#9-mcp-大模型接入) |
| Node catalog trust model and commands | [版本化 Node Catalog](docs/capabilities.md#版本化-node-catalog) |
| Hardware tool matrix and discovery | [硬件测评能力](docs/capabilities.md#4-硬件测评能力) |
| Tech architecture / current state | [docs/tech-stack.md](docs/tech-stack.md) · [docs/current-state.md](docs/current-state.md) |
| Contributing / building from source | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Changelog | [docs/CHANGELOG.md](docs/CHANGELOG.md) |
| External docs site source | [cloudapp3/vmdocs](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs) |

## Community & Support

- 🐛 Found a bug or want a feature? [Open an issue](https://github.com/cloudapp3/vmbench/issues/new)
- 🤝 Want to contribute? Start with [CONTRIBUTING.md](CONTRIBUTING.md)
- 💬 Community chat: [VMPulse Telegram](https://t.me/VMPulse)

## License

[MIT](LICENSE)

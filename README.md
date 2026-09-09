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
# One-line install (Linux / macOS; latest GitHub Release, SHA-256 verified),
# then put it on PATH for the current shell
VMBENCH_BIN_DIR="$(
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --print-install-dir
)" && export PATH="$VMBENCH_BIN_DIR:$PATH"

vmbench                          # interactive TUI (default)
vmbench --json report.json       # hardware benchmark via external tools (default selection)
vmbench --preset quick           # fast overview: hardware + network info + speed + IP quality
vmbench compare a.json b.json    # auto-detect and compare reports
vmbench update                   # self-update from GitHub Releases
```

## Install

The one-liner above installs the **latest release** for your OS/arch and verifies its SHA-256 against `checksums.txt`. Without `--dir`, the install directory is picked automatically: an existing installation in `~/.local/bin`, `~/bin`, or `/usr/local/bin` is reused; a root install uses `/usr/local/bin`; an unprivileged user prefers `~/.local/bin` or `~/bin` when either is already on `PATH`. For an automatic home-directory install that is not yet on `PATH`, the installer adds an idempotent entry to the current shell's startup file (`.zshrc`, `.bashrc`, or `.profile`) and prints the exact reload command; other shells get a warning.

```bash
# Custom directory (never modifies shell startup files; prints a PATH hint)
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --dir /opt/bin

# System-wide install (uses sudo only for target checks and writes)
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --system

# Go toolchain
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest
```

Other installer flags: `--version vX.Y.Z` pins a release, `--no-modify-path` keeps shell startup files untouched, `--print-install-dir` prints the selected directory for scripts, and `--skip-verify` skips checksum verification. `VMBENCH_INSTALL_DIR` mirrors `--dir`; `GITHUB_TOKEN`/`GH_TOKEN` helps with API rate limits or private releases.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --uninstall
```

Removes the binary, the platform data directory (`~/.local/share/vmbench` on Linux, `~/Library/Application Support/vmbench` on macOS) including all locally stored benchmark history, the TUI preferences directory (`~/.config/vmbench` on Linux; on macOS it lives inside the data directory), any manually created `vmbench` systemd/launchd unit, and the installer-owned `# vmbench user install` PATH entries from `.zshrc`/`.bashrc`/`.profile`. Hand-written PATH lines and reports under a custom `VMBENCH_HISTORY_DIR` are preserved. Use `sudo bash -s -- --uninstall` for a root-owned system installation.

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
| `vmbench` | Interactive TUI (no flags) — or run the benchmark when flags are present |
| `vmbench [flags]` | Run the benchmark: hardware only by default, suite sections via preset / only / skip |
| `vmbench nodes <command>` | List / verify / update / health-check the versioned node catalog |
| `vmbench mcp serve [--transport stdio]` | Expose vmbench tools to LLM clients via MCP stdio |
| `vmbench list` | List available workloads |
| `vmbench sysinfo [--json]` | Show system information |
| `vmbench compare <a.json> <b.json> [...]` | Auto-detect and compare benchmark or Suite reports |
| `vmbench history <command>` | Add / list / show / delete / compare local reports |
| `vmbench update [--check] [--version TAG]` | Self-update from GitHub Releases (SHA-256 verified) |
| `vmbench version` | Show version |

## Common Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--iterations` | 3 | Iterations per hardware workload (1-9) |
| `--filter` | all | Regex to select workloads |
| `--hardware-tool` | platform default | sysbench, openssl, fio, dd, stream, mbw, geekbench, winsat, or all |
| `--preset` | — | Scenario preset: `quick`, `website`, `proxy`, `mail` |
| `--only` / `--skip` | — | Select or skip sections |
| `--ip-version` | v4 | `v4`, `v6`, or `dual` |
| `--speed-provider` | cloudflare | cloudflare, speedtest_net, speedtest_cn, china_isp, speedtest_isp, iperf3 |
| `--ip-quality-source` | builtin | builtin; opt-in `securitycheck` (external 18-database binary) |
| `--media-set` | all | `globe`, `tw`, `hk`, `jp`, `kr`, `na`, `sa`, `eu`, `afr`, `sea`, `oce`, `ai`, or combinations |
| `--node-catalog` | embedded | `embedded`, `auto`, or a JSON path |
| `--node-revision` | — | Pin an exact catalog revision; fails before probes start on mismatch |
| `--iperf-host` | — | iperf3 server for the iperf3 speed provider |
| `--json` / `--html` | — | Write JSON / HTML report to file |
| `--quiet` | false | Suppress progress output |
| `--save-history` | false | Save the report to local history (`--history-tag` to label) |
| `--lang` | auto | `en` or `zh-CN` (also `VMBENCH_LANG`) |

Without `--preset` / `--only` / `--skip` the run is hardware-only; network sections are opt-in via a preset or explicit selection. When the effective selection is exactly the `hardware` section the output is a benchmark (run-kind) report — identical to pre-v0.8.0 `vmbench run` — otherwise a composite suite report. Full flag tables: `vmbench --help` or the [capability reference](docs/capabilities.md).

## VPS Suite

The suite keeps the YABS-style one-command experience with ECS-style modular sections:

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
vmbench --preset proxy --ip-version dual
vmbench --only ping,mail
vmbench --route-presets gz,bj,sh,cd,cernet,cstnet
vmbench --speed-provider china_isp
vmbench --media-set jp,kr
vmbench --only hardware --hardware-tool geekbench
vmbench --node-catalog auto --save-history --history-tag weekly
```

Suite succeeds only when every enabled section ends `status=ok`; enabled empty / skipped / partial / error states all fail the run. Reports keep the resolved node catalog source/revision and selected node IDs.

## TUI

Launch with `vmbench` (no arguments):

- **Dashboard** with benchmark / compare / sysinfo entry points; mouse clicks and wheel scrolling everywhere
- **Config**: one page for the same normalized fields as CLI/MCP — preset pills lead with Hardware Only (the CLI default) plus Custom and the suite presets; section toggles reveal tool, filter, speed, route, media, and IP-source cards on demand, with a live planned-duration summary, missing-tool preflight, and `1-9` section jumps
- **Running**: one progress page for both kinds — workload grid for hardware runs, section grid for suite runs, cancel modal included
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

Missing `fio` or `sysbench` on a Linux host can be fixed without touching system packages:

```sh
vmbench tools fetch fio sysbench   # pinned static builds, SHA-256 verified, ~/.cache/vmbench/binaries
```

Downloads come from the `tools` release assets of this repository and are verified against hashes compiled into vmbench; `--url` accepts a mirror.

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

# vmbench

**Check everything your VPS provider promised — in one command.**

[![CI](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cloudapp3/vmbench.svg)](https://pkg.go.dev/github.com/cloudapp3/vmbench)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](../LICENSE)

> The root [README.md](../README.md) of this repository is in Simplified Chinese; this is the English edition.

vmbench is a one-command checkup for people who buy, compare, and post about VPSes. It answers the questions you actually have about a new box:

- **Did I get the hardware I paid for?** — CPU / memory / disk benchmarks via sysbench, fio, and OpenSSL (external tools only; Geekbench, dd, STREAM opt-in)
- **Is that "premium line" for real?** — China carrier / CERNET / CSTNET traceroutes with return-route line classification: 163 / 9929 / 4837 / CN2 / CMIN2 / CMI
- **How fast is it, really?** — Cloudflare, Ookla, speedtest.cn, per-ISP China (三网) downloads, or iperf3 against your own endpoint
- **What does it unlock?** — 200+ streaming & AI platforms (Netflix, Disney+, ChatGPT…), the globe set, everything, or by region
- **Is the IP clean?** — reputation, DNSBL listings, and mail ports: can it send mail, or is it already burned
- **Can I paste this to the forum without leaking my IP?** — yes: this machine's public IPv4/IPv6 are masked by default on every surface, and nothing uploads unless you run `vmbench share` yourself
- **Will it mess up the box?** — no: static fio/sysbench binaries land in the user cache, system packages stay untouched, and `vmbench uninstall` removes everything

Drive it from the interactive TUI, the CLI, or MCP — the same normalized config behind all three. vmbench reports raw metrics and structured failures, never an invented total score; the optional `vmbench score` command derives a deterministic assessment (dimension indexes, scenario fit, coverage disclosure) from a versioned baseline, and raw measurements always remain the source of truth. Output: console / JSON / HTML / forum-ready markdown, in English and 简体中文, on Linux, macOS, and Windows.

Documentation: [中文快速说明](README.zh-CN.md) · [Full capability reference](capabilities.md) · [Tech stack](tech-stack.md) · [Changelog](CHANGELOG.md)

[Quick start](#quick-start) · [Install](#install) · [Commands](#commands) · [Flags](#common-flags) · [VPS Checkup](#vps-checkup) · [TUI](#tui) · [Documentation](#documentation)

## Quick Start

```bash
# One-line install (Linux / macOS; latest GitHub Release, SHA-256 verified),
# then put it on PATH for the current shell
VMBENCH_BIN_DIR="$(
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --print-install-dir
)" && export PATH="$VMBENCH_BIN_DIR:$PATH"

vmbench                          # interactive TUI: tick the tests you want, press Enter
vmbench --json report.json       # plain hardware benchmark (the default run)
vmbench --preset quick           # first-five-minutes check: hardware + network info + speed + IP quality
vmbench --preset proxy           # the proxy-box question: route, ping, speed, IP quality, unlock, reachability
vmbench --markdown report.md     # forum-pasteable report — public IP already masked
vmbench score report.json        # deterministic assessment (dimension indexes, profile fit, coverage)
vmbench share report.json --dry-run  # preview the redacted paste, then drop --dry-run to upload
vmbench update                   # self-update from GitHub Releases
```

## Install

The one-liner above installs the **latest release** for your OS/arch and verifies its SHA-256 against `checksums.txt`. Without `--dir`, the install directory is picked automatically: an existing installation in `~/.local/bin`, `~/bin`, or `/usr/local/bin` is reused; a root install uses `/usr/local/bin`; an unprivileged user prefers `~/.local/bin` or `~/bin` when either is already on `PATH`. For an automatic home-directory install that is not yet on `PATH`, the installer adds an idempotent entry to the current shell's startup file (`.zshrc`, `.bashrc`, or `.profile`) and prints the exact reload command; other shells get a warning.

```bash
# System-wide install (uses sudo only for target checks and writes)
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --system

# Go toolchain
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest
```

Other installer flags: `--version vX.Y.Z` pins a release, `--no-modify-path` keeps shell startup files untouched, `--print-install-dir` prints the selected directory for scripts, and `--skip-verify` skips checksum verification. `--dir PATH` (mirrored by `VMBENCH_INSTALL_DIR`) selects a custom directory; explicit directories never modify shell startup files, so make sure the directory is on `PATH`. `GITHUB_TOKEN`/`GH_TOKEN` helps with API rate limits or private releases.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --uninstall
```

Removes the binary, the platform data directory (`~/.local/share/vmbench` on Linux, `~/Library/Application Support/vmbench` on macOS) including all locally stored benchmark history, the TUI preferences directory (`~/.config/vmbench` on Linux; on macOS it lives inside the data directory), any manually created `vmbench` systemd/launchd unit, and the installer-owned `# vmbench user install` PATH entries from `.zshrc`/`.bashrc`/`.profile`. Hand-written PATH lines and reports under a custom `VMBENCH_HISTORY_DIR` are preserved. Use `sudo bash -s -- --uninstall` for a root-owned system installation.

The installed binary can also uninstall itself: `vmbench uninstall` prints the removal plan (history record count, fetched tools, TUI preferences, data directory, the binary itself), asks for confirmation on a terminal, and removes owned directories before the binary. It keeps the binary whenever a removal fails, so an interrupted uninstall can simply be rerun. Shell startup files and services are left to `install.sh --uninstall`, which delegates to this command when available and stops a manually created service and cleans its PATH entries afterwards. Flags: `--dry-run` previews the plan, `--yes` skips the prompt, `--json` emits a structured plan/result for scripts.

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
| `vmbench [flags]` | Run the benchmark: hardware only by default, checkup sections via preset / only / skip |
| `vmbench nodes <command>` | List / verify / update / health-check the versioned node catalog |
| `vmbench mcp serve [--transport stdio]` | Expose vmbench tools to LLM clients via MCP stdio |
| `vmbench list` | List available workloads |
| `vmbench sysinfo [--json]` | Show system information |
| `vmbench score <report.json\|->` | Derive a deterministic assessment from a report against the versioned scoring baseline |
| `vmbench share <report.json\|->` | Project a saved report into a redacted paste and upload it to an explicitly chosen provider |
| `vmbench history <command>` | Add / list / show / delete local reports |
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
| `--redact` | ips | Mask this machine's public IPv4/IPv6 in reports (`none` to keep real addresses) |
| `--media-set` | `globe` | `globe` covers the 41 international platforms (AI services included); `all` runs every platform, or combine region IDs |
| `--node-catalog` | embedded | `embedded`, `auto` (fetch each run, silent cache/embedded fallback), or a JSON path |
| `--node-revision` | — | Pin an exact catalog revision; fails before probes start on mismatch |
| `--iperf-host` | — | iperf3 server for the iperf3 speed provider |
| `--json` / `--html` / `--markdown` | — | Write JSON / HTML / forum-pasteable markdown report to file |
| `--quiet` | false | Suppress progress output |
| `--auto-swap` | false | (Linux) low-memory geekbench runs: create a temporary swapfile without asking, remove it afterwards |
| `--save-history` | false | Save the report to local history (`--history-tag` to label) |
| `--lang` | auto | `en` or `zh-CN` (also `VMBENCH_LANG`) |

Without `--preset` / `--only` / `--skip` the run is hardware-only; network sections are opt-in via a preset or explicit selection. When the effective selection is exactly the `hardware` section the output is a benchmark (run-kind) report — identical to pre-v0.8.0 `vmbench run` — otherwise a composite checkup report. Full flag tables: `vmbench --help` or the [capability reference](capabilities.md).

Reports are redacted by default: this machine's public IPv4/IPv6 addresses are consistently replaced with documentation-range placeholders (`203.0.113.x`, `2001:db8::x`) across every surface — console, JSON, HTML, markdown, history, TUI, and MCP — including BGP/CIDR prefixes and reverse DNSBL labels that embed the address. Internal IPs, hostnames, route hops, and remote node IPs are kept. `--redact none` opts out (the CLI prints a sharing warning); TUI and MCP always redact.

`--markdown report.md` exports a share-ready paste: one heading per section with textgrid-aligned tables inside fenced code blocks (CJK-safe in any renderer), the media section folded to per-region counts plus exception items, and a provenance line (version, UTC time, catalog revision). It derives from the same redacted report as JSON/HTML and embeds no scores.

## Sharing

`vmbench share <report.json|->` turns a saved run/checkup report into a paste-friendly payload and — only when you run this command — uploads it, returning a link:

```bash
vmbench share report.json --dry-run           # print the payload + redaction inventory, zero network
vmbench share report.json                     # upload to dpaste (default), print the URL
vmbench share report.json --provider dpaste,0x0   # explicit fallback chain
vmbench share report.json --format json       # upload the redacted report JSON instead of markdown
vmbench share report.json --save-payload out.md    # keep a copy of exactly what was uploaded
```

The payload reuses the `--markdown` projection (or `--format json` for the redacted report verbatim) and appends a footer that states what was actually redacted. Sharing is explicit-only: nothing else in vmbench — benchmark, TUI, or MCP — uploads anything. Beyond the report-layer IP masking, share additionally masks the structured hostname. `--redact none` keeps real addresses and prints a warning; `--media full` lists every media service instead of the folded view. Providers: `dpaste` (default), `0x0`, `paste_rs`, and `custom` (requires `--share-endpoint`, https only). Fallback tries the next provider only on network errors or 5xx responses — a payload is uploaded successfully at most once. Payloads above 512 KiB are rejected with convergence advice instead of being truncated.

## VPS Checkup

The checkup keeps the YABS-style one-command experience with ECS-style modular sections:

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

The checkup succeeds only when every enabled section ends `status=ok`; enabled empty / skipped / partial / error states all fail the run. Reports keep the resolved node catalog source/revision and selected node IDs.

## TUI

Launch with `vmbench` (no arguments):

- **Dashboard** with benchmark / history / sysinfo entry points; mouse clicks and wheel scrolling everywhere. The System card shows the machine's public IPv4/IPv6 with ASN org (probed async at startup, silent when offline; System Info expands to country and ISP)
- **Config**: one page for the same normalized fields as CLI/MCP — preset pills lead with Hardware Only (the CLI default) plus Custom and the checkup presets; section toggles reveal tool, filter, speed, route, media, and IP-source cards on demand, with a live planned-duration summary, missing-tool preflight, and `1-9` section jumps
- **Running**: one progress page for both kinds — workload grid for hardware runs, section grid for checkup runs, cancel modal included
- **Results**: cards / grouped / flat views; `d` opens per-workload detail with metrics, samples, errors, and raw tool output
- **History**: browse reports saved with `--save-history` (time, kind, tag, ID); `Enter` reopens a run in Results or a checkup in the checkup page, `Esc` returns
- **Themes**: press `t` on Dashboard to cycle; the choice is saved locally

Keys: `?` help · `↑↓` navigate · `Enter` select · `Tab` switch view · `d` detail · `s` save · `Esc` back · `q` or `Ctrl+C` quit. Every page scrolls with `PgUp/PgDn`, `Home/End`, and the mouse wheel; fits an 80x24 terminal.

## Languages

CLI, TUI, and console/HTML report labels are localized in English and Simplified Chinese — by default they follow the system locale (`LC_ALL`/`LANG`). Override with `--lang` or `VMBENCH_LANG`, or press `l` (or click the language line) on the TUI dashboard to cycle auto (system) → English → 中文; an explicit choice is saved to the TUI config. JSON field names, status enums, section IDs, and workload names stay English in every locale.

## Platform Support

| Capability | Linux | macOS | Windows |
|------------|:-----:|:-----:|:-------:|
| CLI / TUI / JSON / HTML | ✅ | ✅ | ✅ |
| Default hardware tools | ✅ sysbench / fio / openssl | ⚠️ openssl (others via package manager) | ⚠️ WinSAT |
| Checkup network diagnostics | ✅ | ✅ | ⚠️ partial / environment-dependent |
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
| 简体中文主 README | [README.md](../README.md) |
| Full capability reference (中文): CLI flags, checkup, node catalog, MCP, report formats, Go API | [capabilities.md](capabilities.md) |
| 中文快速说明 | [README.zh-CN.md](README.zh-CN.md) |
| MCP tools, client config, safety defaults | [MCP 大模型接入](capabilities.md#9-mcp-大模型接入) |
| Node catalog trust model and commands | [版本化 Node Catalog](capabilities.md#版本化-node-catalog) |
| Hardware tool matrix and discovery | [硬件测评能力](capabilities.md#4-硬件测评能力) |
| Tech architecture / current state | [tech-stack.md](tech-stack.md) · [current-state.md](current-state.md) |
| Contributing / building from source | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Changelog | [CHANGELOG.md](CHANGELOG.md) |
| External docs site source | [cloudapp3/vmdocs](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs) |

## Community & Support

- 🐛 Found a bug or want a feature? [Open an issue](https://github.com/cloudapp3/vmbench/issues/new)
- 🤝 Want to contribute? Start with [CONTRIBUTING.md](../CONTRIBUTING.md)
- 💬 Community chat: [VMPulse Telegram](https://t.me/VMPulse)

## License

[MIT](../LICENSE)

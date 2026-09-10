# vmbench 产品说明

> 最新实现状态与最近一次修正，见 [`current-state.md`](current-state.md) 与 [`CHANGELOG.md`](CHANGELOG.md)。

## 产品定位

vmbench 是一款跨平台 VPS 测评工具，用 Go 编写，强调：

- 原始指标，不做总分
- 结构化报告
- TUI / CLI 双入口，TUI 内支持主题切换、全页滚动、帮助页、鼠标操作、历史记录对比与 workload 详情
- 适合自动化采集和横向对比
- 一条命令双形态：不带参数只测硬件，`--preset` / `--only` 选择网络 section 后提供类似 YABS 的一键完整体检（含全部网络诊断）
- 结构像 ECS：按模块、按场景、按原始指标展示

## 核心能力

- **硬件测评**：CPU / 内存 / 磁盘
- **网络身份**：虚拟化、公网 IPv4/IPv6、ASN/provider/location、保守 NAT 证据、STUN NAT 类型（Full Cone/Restricted/Symmetric、hairpin、端口保持）、IP 的 BGP/RDAP 归属（注册网段/RIR/注册日期、上游/对等/IXP 与 Tier 1 标注）
- **网络诊断**：版本化节点上的 route（含回程线路类型 163/9929/4837/CN2/CTGNET/CMIN2/CMI 判定与逐跳 ASN 标注）、ping、speed，以及网站/Telegram 可达性
- **IP 质量**：IP reputation / DNSBL / 邮件端口 / ipapi.is 归属交叉验证，opt-in 的 securityCheck 外部 18 库视角
- **流媒体检测**：UnlockTests 全平台 200+ 流媒体/AI 服务解锁状态（可按地区子集运行）
- **速度测试**：Cloudflare / Speedtest.net / Speedtest.cn / iperf3，以及三网（电信/联通/移动）provider
- **报告输出**：console / JSON / HTML
- **结果对比**：自动识别 benchmark/体检，按兼容的原始指标比较两份或更多报告
- **本地历史**：安全保存、查询、删除并比较最近 N 份报告
- **MCP 调用**：通过 `vmbench mcp serve --transport stdio` 暴露给大模型客户端

## 测试维度

### Hardware
- CPU：sysbench single-core / multi-core prime，openssl speed；Geekbench 可选启用
- Memory：sysbench 顺序读/写带宽、随机读延迟；STREAM / mbw 可选启用
- Disk：fio 4K 随机读/写 Q1/Q32、1M 顺序读/写 Q1/Q8；dd sequential write/read 可选启用，Linux read 使用 direct I/O
- Windows：默认使用 WinSAT CPU / memory / disk
- 说明：硬件跑分只依赖外部工具；CLI 只为当前 filter 会命中的 workload 预检缺失工具，并提示可用的 Debian/Ubuntu 安装命令与 `vmbench tools fetch`（pin 版静态二进制，装到用户缓存目录，不改动系统），受影响 workload 仍进入结构化 error，不回退到进程内算法

### Network
- Network Info：本机虚拟化 + 公网 IPv4/IPv6、ASN、provider、location 与 `direct/translated/unknown` NAT 证据
- Route：基于版本化 catalog 的三网、成都、CERNET、CSTNET、IPv4/IPv6 traceroute，记录解析目标、是否到达和 `ok|partial|error`
- Ping：同一 catalog 节点上的 TCP latency / jitter / loss；RST/refused 作为收到响应，另记 `connection_state`
- Speed：可选 provider，包括 Cloudflare、Speedtest.net、Speedtest.cn、iperf3
- IP Quality：IP、DNSBL、邮件端口
- Reachability：Google/GitHub/Cloudflare HTTPS 与 Telegram DC TCP 可达性，保留 latency/status/error
- Mail：顺序探测 25/465/587/2525/110/143/993/995，并区分 `open|refused|timeout|error`
- Media：Netflix / YouTube / Disney+ / ChatGPT / TikTok / Prime 等

## 输出模型

vmbench 输出的是原始测量数据：

- median time
- actual iterations / raw samples
- throughput
- latency
- optional cumulative bytes / ops processed（仅语义明确时输出）
- detail / error

硬件默认 workload 会拆分输出 memory read/write/latency 与 fio 4K/1M 多队列深度结果，便于直接比较具体瓶颈；这些细分项仍然只是原始指标。

原始指标是唯一事实来源。确定性派生评估（综合 index/等级）由独立命令 `vmbench score` 基于版本化基线生成，派生命令不会回写或修改原始报告。

体检 JSON 使用 schema v2 envelope：`report_kind`、`report_id`、app build、system、timestamps/duration、规范化 config、catalog provenance 和九个 section，同时保留旧 `version`/Unix time 字段给 v1 consumer。Route 结果包含 `resolved_target/destination_reached/status`，Ping 结果包含 `connection_state`。体检 HTML 展示硬件 workload、网络身份、完整 route hops、ping、speed provider、IP quality、网站/TG、mail、media 及其 detail/error，而不是只给 section 摘要。

## 界面语言

CLI / TUI / 报告标签支持英文与简体中文（`--lang`、`VMBENCH_LANG`、config.json `lang`、系统 locale 依次优先）。JSON 输出与状态 token 不随语言变化，保证 compare 与历史记录的可比性。

## 使用方式

```bash
vmbench --json report.json
vmbench --filter 'sysbench|fio|OpenSSL'
vmbench --hardware-tool sysbench,openssl,fio,dd
vmbench --preset quick
vmbench --preset website
vmbench --only ping,mail
vmbench --ip-version dual
vmbench --quiet --json checkup.json
vmbench --node-catalog auto --node-revision 2026-07-13.1 --save-history
vmbench mcp serve --transport stdio
vmbench compare a.json b.json
vmbench history compare --last 3
vmbench score report.json
vmbench nodes list --node-catalog embedded
vmbench nodes health --node-catalog auto --ip-family v6
```

`vmbench` 一个命令覆盖硬件基准与综合测评：不带 `--preset` / `--only` / `--skip` 时只编排 sysbench / fio / OpenSSL / WinSAT 等外部工具的硬件基准（run 报告，固定 `scope=hardware`、`extensions=false`），preset 或 `--only` 选择网络 section 后走综合测评（体检报告），网络诊断（route、speed、IP 质量等）都在同一命令面上。所有 workload 串行隔离执行，线程数和队列深度由适配器定义。硬件测评会在执行前检查当前 filter 实际涉及的工具是否可解析，但缺失工具不会被静默跳过。

## 体检场景预设

不带 section 参数时默认只跑 hardware（等价 v0.7.0 的 `vmbench run`）；`--preset` 用于按 VPS 使用场景选择 section，让新用户保持一键体验，也让自动化任务可以稳定复用同一组维度。

| Preset | 使用场景 | Sections |
|---|---|---|
| `quick` | 快速概览，接近 YABS 的轻量一键体验 | `hardware,network_info,speed,ip_quality` |
| `website` | 建站 / 服务端部署 | `hardware,network_info,route,ping,speed,ip_quality,reachability,mail` |
| `proxy` | 代理 / 解锁 / 网络体验 | `network_info,route,ping,speed,ip_quality,reachability,media` |
| `mail` | 邮件服务器可用性 | `network_info,route,ip_quality,mail` |

预设只是 section 选择，不改变输出模型，也不产生综合分。仍可用：

```bash
vmbench --preset proxy --ip-version dual
vmbench --preset website --skip media
vmbench --only ping,mail
```

速度测试也支持 provider 选择：

```bash
vmbench --speed-provider cloudflare,speedtest_net
vmbench --speed-provider china_isp
vmbench --speed-provider speedtest_isp
vmbench --speed-provider iperf3 --iperf-host 1.2.3.4
vmbench --only hardware --hardware-tool dd,stream,mbw
vmbench --only hardware --hardware-tool geekbench
```

`speed` section 的输出会按 provider 分组展示下载、上传、延迟、状态和错误信息，便于区分 Cloudflare / Ookla / speedtest.cn / iperf3 的失败原因。`china_isp` 使用版本化 catalog 中的 `isp_download` 节点（speedtest.cn 直连端点，数据来自 MIT 的 speedtest.cn-CN-ID），按电信/联通/移动顺序下载，同运营商节点依次 fallback；`speedtest_isp` 将 Ookla `speedtest` CLI 固定到按运营商的 speedtest.net 节点 ID（数据来自 MIT 的 speedtest.net-CN-ID），需要本机安装 speedtest CLI。流媒体与 IP 质量可分别用 `--media-set jp,kr` 与 `--ip-quality-source builtin,securitycheck` 收敛范围（securityCheck 二进制需自行安装，缺失时记录 `unavailable` 而不影响 section）。

体检只有在每个 enabled section 都是 `status=ok` 时成功。enabled section 的空状态、`skipped`、`partial`、`error` 都会使总体失败并让 CLI 返回非零；disabled section 才只发 `section.skip`。选择 iperf3 provider 但没有可用 host 会直接返回 speed error。

默认 timeout 仍为 5 分钟：hardware 按 workload 应用，其余网络 section 各自派生 section timeout；调用方 cancel/deadline 会写成结构化 section error。体检 CLI 默认把 section 的 running/完成状态实时写到 stderr，`--quiet` 可关闭进度而不改变报告。

Ping 会保留逐目标结果，全部目标失败时额外返回聚合错误。TCP connect 成功和 TCP RST/refused 都证明目标已响应，均计入 `received` 和延迟而不计为丢包；`connection_state` 区分 `open/refused/mixed/no_response`。Route 会先解析实际目标并记录 `resolved_target`；只有到达该地址才是 `status=ok`，已有有效 hop 但未到达为 `partial`，无有效证据或探测失败为 `error`。Mail 对内置端口逐个顺序连接，避免共享目标对并发突发限流，逐项状态为 `open/refused/timeout/error`；DNS 失败保持为探测 `error`，不会伪装成端口 timeout。

输出层级：

- `groups`：按 provider 聚合
- `providers`：具体探测项
- `summary`：单 provider 摘要；多 provider 时明确标记 `aggregation=best_per_metric`

## 可复现节点与历史

网络节点从硬编码常量迁移为版本化 catalog。内置快照保证离线可运行；`--node-catalog embedded|auto|PATH` 选择数据源，`--node-revision` 固定 revision。`auto` 只读取已验证缓存并在缓存不可用时回退 embedded，不在测评过程中隐式更新。节点 ID、地区、城市、运营商、ASN、IP family、protocol、endpoint、source 和流量预算进入 catalog，并由报告记录最终 source/revision/node IDs；download 的 `traffic_bytes` 会限制单次响应体读取量。

```bash
vmbench nodes list --node-catalog embedded --json
vmbench nodes verify --node-catalog PATH --signature nodes.sig --public-key nodes.pub
vmbench nodes update --url URL --signature URL --public-key nodes.pub
vmbench nodes health --node-catalog auto --kind route --ip-family v6 --json
```

更新流程要求显式 Ed25519 公钥，先验证 detached signature 和严格 schema，再原子替换缓存；Unix 文件 mode 为 `0600`。签名、revision 或 schema 不满足时 fail-closed，不启动探测也不覆盖旧缓存。

`vmbench` 可用 `--save-history [--history-tag TAG]` 保存报告；也可用 `history add/list/show/delete` 管理已有 JSON，`history compare --last N` 比较最近 N 份同类型报告。CLI 的 `--json` / `--html` 导出和 history 都先写同目录临时文件、sync 后 rename；Unix 导出/历史文件 mode 为 `0600`，其他平台仍应依赖系统 ACL 保护。报告可能包含 hostname、公网 IP 和 route hops，任何未来 upload/share 都必须显式授权并支持脱敏。Route/Ping 报告区分 catalog protocol 与实际 `probe_protocol/probe_tool`；体检 Compare 只有在 unit、实际 protocol/IP family、provider/probe tool、target/node 以及需要时 catalog revision 全部兼容时才计算 delta。不兼容值仍展示，但明确给出 reason。Route 还必须显式为 `status=ok` 且 `destination_reached=true`，旧报告没有到达证据时不计算 delta。Mail 只比较 `status=open` 的成功连接延迟，拒绝、超时和错误耗时不参与 latency delta。

## 自升级

`vmbench update` 让已安装的二进制从 GitHub Releases 自升级：查询最新 release（`releases/latest`，天然排除 draft/prerelease），与当前构建版本比较后下载对应 OS/arch 的发布归档，按 release `checksums.txt` 做 SHA-256 校验，解出二进制并以临时文件 + rename 原地替换当前可执行文件（Windows 先将旧文件移到 `.old` 再替换）。校验模型与 `install.sh` 一致：checksums over TLS，release 资产本身不做签名；`--version TAG` 可固定/降级版本（等于当前版本也重装，可用于修复），`--check` 只报告不安装，`--json` 输出结构化状态。`GITHUB_TOKEN`/`GH_TOKEN` 会被作为 bearer token 转发给 GitHub API 以缓解速率限制。deb/rpm 安装的实例建议走包管理器升级，或用 `--dest` 指定可写路径；目标不可写时命令 fail-closed 并提示替代方案。

## 卸载

`vmbench uninstall` 一条命令移除二进制在机器上创建的一切：先打印删除计划（历史报告条数、已获取的静态工具、TUI 偏好、数据目录、二进制本体），终端交互确认后按「目录 → 二进制本体」顺序删除。二进制最后删除，且其余任何一项失败即保留，卸载可安全重跑。三条边界与安装脚本一致：自定义 `VMBENCH_HISTORY_DIR`（及 `VMBENCH_CONFIG` 重定向位置）下的报告保留；shell 启动文件里的 PATH 条目归 `install.sh --uninstall` 清理，本命令不碰 rc 文件；手动创建的 systemd/launchd unit 只检测提示、不停止不删除。`--dry-run` 只打印计划，`--json` 输出结构化计划/结果供脚本使用，`--yes` 跳过确认；非终端 stdin（含 `install.sh` 的委托调用）跳过确认直接执行。dpkg/rpm 管理的二进制会收到改用包管理器卸载的警告。

## MCP 给大模型调用

`vmbench mcp serve --transport stdio` 提供本地 MCP Server，让 Claude、Codex、Cursor、Cline 等客户端通过 tools 调用 vmbench，而不是让模型执行任意 shell。

首批 tools：

| Tool | 说明 |
|---|---|
| `vmbench_capabilities` | 输出版本、checkup sections、presets、hardware tools、speed providers、workload 列表 |
| `vmbench_sysinfo` | 输出当前主机系统信息和 warning |
| `vmbench_run` | 运行基准（MCP 默认 `iterations=1`；不带 section 参数只跑 hardware，preset/only/skip 选择网络 section 后返回体检报告） |

MCP 输出仍然遵守 vmbench 的产品原则：只返回原始指标和结构化诊断；派生评估由 `vmbench score` CLI 提供，MCP 本身不评分。IP Quality 的风险评分属于业务诊断，不是 benchmark 总分。

安全边界：

- stdout 只写 MCP JSON-RPC，日志/错误写 stderr。
- 不接受任意 shell 命令。
- `hardware_tools`、section、preset、speed provider 都限制在内置枚举。
- CLI、TUI、MCP 共同使用规范化后的体检配置字段：iterations、timeout、hardware tools、speed providers、iperf hosts、IP version、sections、route selection、`catalog_source`、`catalog_revision`。
- 三个入口复用同一校验/归一化契约，但按交互场景暴露字段子集；Go TUI 在低于 40 行时使用紧凑体检视图，配置、运行和结果页可在 `80x24` 内完整操作和查看状态。
- catalog source 只接受 `embedded`、`auto` 或显式路径；revision pin 不匹配会在网络 probe 前失败。
- 省略 `iterations` / `timeout_ms` 时分别默认 1 / 5 分钟；显式非正或超出上限的值、非法 regex、混入未知项的枚举数组会使整个调用失败，不会静默丢弃非法项后继续测量。
- `iterations` 最大 9，`timeout_ms` 最大 15 分钟。
- 同一时间只允许一个 benchmark 运行，避免压垮机器。
- 参数校验失败返回 `isError=true` 和错误文本；测量失败仍返回完整 `structuredContent.report`，同时设置 `isError=true`，便于调用方读取 workload/section 的结构化错误。

## Go API

```go
report := vmbench.RunCore(context.Background(), vmbench.Options{
	Iterations:    3,
	Engine:        "external",
	HardwareTools: []string{"sysbench", "openssl", "fio", "dd"},
})
```

`RunCore` 只执行硬件基准；需要网络诊断时使用 `checkup.Run`（catalog source/revision 等参数见 checkup 包）。报告 JSON 当前为 schema v2，每项保留实际迭代次数、`samples_ms`、吞吐、延迟和结构化错误；`bytes_processed` / `ops_processed` 只在 workload 明确报告累计字节/操作数时出现，不会从 events/s、IOPS、MB/s 或 score 猜测。任一选中 workload 失败时 CLI 返回非零状态。

## 设计原则

- 不强行归一化
- 不伪造总分
- 保留失败信息
- 保留原始细节
- 对比优先于排名
- 预设只表达测评场景，不表达性能等级

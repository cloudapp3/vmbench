# vmbench 产品说明

> 最新实现状态与最近一次修正，见 [`current-state.md`](current-state.md) 与 [`CHANGELOG.md`](CHANGELOG.md)。

## 产品定位

vmbench 是一款跨平台 VPS 测评工具，用 Go 编写，强调：

- 原始指标，不做总分
- 结构化报告
- TUI / CLI 双入口，TUI 内支持主题切换
- 适合自动化采集和横向对比
- `run` 默认只测硬件，`suite` 提供类似 YABS 的一键完整测评
- 结构像 ECS：按模块、按场景、按原始指标展示

## 核心能力

- **硬件测评**：CPU / 内存 / 磁盘
- **网络身份**：虚拟化、公网 IPv4/IPv6、ASN/provider/location、保守 NAT 证据
- **网络诊断**：版本化节点上的 route、ping、speed，以及网站/Telegram 可达性
- **IP 质量**：IP reputation / DNSBL / 邮件端口
- **流媒体检测**：常见平台解锁状态
- **速度测试**：Cloudflare / Speedtest.net / Speedtest.cn / iperf3 provider
- **报告输出**：console / JSON / HTML
- **结果对比**：自动识别 benchmark/Suite，按兼容的原始指标比较两份或更多报告
- **本地历史**：安全保存、查询、删除并比较最近 N 份报告
- **MCP 调用**：通过 `vmbench mcp serve --transport stdio` 暴露给大模型客户端

## 测试维度

### Hardware
- CPU：sysbench single-core / multi-core prime，openssl speed；Geekbench 可选启用
- Memory：sysbench 顺序读/写带宽、随机读延迟；STREAM / mbw 可选启用
- Disk：fio 4K 随机读/写 Q1/Q32、1M 顺序读/写 Q1/Q8；dd sequential write/read 可选启用，Linux read 使用 direct I/O
- Windows：默认使用 WinSAT CPU / memory / disk
- 说明：硬件跑分只依赖外部工具；CLI 只为当前 filter 会命中的 workload 预检缺失工具，并提示可用的 Debian/Ubuntu 安装命令，受影响 workload 仍进入结构化 error，不回退到进程内算法

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

硬件默认 workload 会拆分输出 memory read/write/latency 与 fio 4K/1M 多队列深度结果，便于直接比较具体瓶颈；这些细分项仍然只是原始指标，不参与综合打分。

不输出综合评分、等级或 category score。

Suite JSON 使用 schema v2 envelope：`report_kind`、`report_id`、app build、system、timestamps/duration、规范化 config、catalog provenance 和九个 section，同时保留旧 `version`/Unix time 字段给 v1 consumer。Route 结果包含 `resolved_target/destination_reached/status`，Ping 结果包含 `connection_state`。Suite HTML 展示硬件 workload、网络身份、完整 route hops、ping、speed provider、IP quality、网站/TG、mail、media 及其 detail/error，而不是只给 section 摘要。

## 使用方式

```bash
vmbench run
vmbench run --filter 'sysbench|fio|OpenSSL'
vmbench run --hardware-tool sysbench,openssl,fio,dd
vmbench run --scope network --iterations 1
vmbench run --scope all --iterations 1
vmbench run --json report.json
vmbench suite
vmbench suite --preset quick
vmbench suite --preset website
vmbench suite --only ping,mail
vmbench suite --ip-version dual
vmbench suite --quiet --json suite.json
vmbench suite --node-catalog auto --node-revision 2026-07-13.1 --save-history
vmbench mcp serve --transport stdio
vmbench compare a.json b.json
vmbench history compare --last 3
vmbench nodes list --node-catalog embedded
vmbench nodes health --node-catalog auto --ip-family v6
```

`vmbench run` 默认 `scope=hardware`。只有显式使用 `--scope network` 或 `--scope all` 才运行网络 workload；这类运行会输出约 1.75 GB 的流量提示，网络 workload 最多执行一次并在报告中记录实际 `iterations=1`。报告 config 会保留规范化后的 `scope`、可选 `iperf_hosts` 和网络实际使用的 `catalog_source/catalog_revision/node_ids`；hardware 的 `extensions=false`，network/all 为 `true`。所有 workload 串行隔离执行，线程数和队列深度由 sysbench/fio/OpenSSL/WinSAT 适配器定义；旧的 `--mode multi/all` 仅为兼容保留，不再并发 workload 或生成重复 pass。`run` 和启用 hardware 的 `suite` 会在执行前检查当前 filter 实际涉及的工具是否可解析，但缺失工具不会被静默跳过。

## Suite 场景预设

`vmbench suite` 默认执行完整 VPS 测评。`--preset` 用于按 VPS 使用场景选择 section，让新用户保持一键体验，也让自动化任务可以稳定复用同一组维度。

| Preset | 使用场景 | Sections |
|---|---|---|
| `quick` | 快速概览，接近 YABS 的轻量一键体验 | `hardware,network_info,speed,ip_quality` |
| `website` | 建站 / 服务端部署 | `hardware,network_info,route,ping,speed,ip_quality,reachability,mail` |
| `proxy` | 代理 / 解锁 / 网络体验 | `network_info,route,ping,speed,ip_quality,reachability,media` |
| `mail` | 邮件服务器可用性 | `network_info,route,ip_quality,mail` |

预设只是 section 选择，不改变输出模型，也不产生综合分。仍可用：

```bash
vmbench suite --preset proxy --ip-version dual
vmbench suite --preset website --skip media
vmbench suite --only ping,mail
```

速度测试也支持 provider 选择：

```bash
vmbench suite --speed-provider cloudflare,speedtest_net
vmbench suite --speed-provider iperf3 --iperf-host 1.2.3.4
vmbench suite --only hardware --hardware-tool dd,stream,mbw
vmbench suite --only hardware --hardware-tool geekbench
```

`speed` section 的输出会按 provider 分组展示下载、上传、延迟、状态和错误信息，便于区分 Cloudflare / Ookla / speedtest.cn / iperf3 的失败原因。

Suite 只有在每个 enabled section 都是 `status=ok` 时成功。enabled section 的空状态、`skipped`、`partial`、`error` 都会使总体失败并让 CLI 返回非零；disabled section 才只发 `section.skip`。选择 iperf3 provider 但没有可用 host 会直接返回 speed error。

默认 timeout 仍为 5 分钟：hardware 按 workload 应用，其余网络 section 各自派生 section timeout；调用方 cancel/deadline 会写成结构化 section error。Suite CLI 默认把 section 的 running/完成状态实时写到 stderr，`--quiet` 可关闭进度而不改变报告。

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

`vmbench run` / `vmbench suite` 可用 `--save-history [--history-tag TAG]` 保存；也可用 `history add/list/show/delete` 管理已有 JSON，`history compare --last N` 比较最近 N 份同类型报告。CLI 的 `--json` / `--html` 导出和 history 都先写同目录临时文件、sync 后 rename；Unix 导出/历史文件 mode 为 `0600`，其他平台仍应依赖系统 ACL 保护。报告可能包含 hostname、公网 IP 和 route hops，任何未来 upload/share 都必须显式授权并支持脱敏。Route/Ping 报告区分 catalog protocol 与实际 `probe_protocol/probe_tool`；Suite Compare 只有在 unit、实际 protocol/IP family、provider/probe tool、target/node 以及需要时 catalog revision 全部兼容时才计算 delta。不兼容值仍展示，但明确给出 reason。Route 还必须显式为 `status=ok` 且 `destination_reached=true`，旧报告没有到达证据时不计算 delta。Mail 只比较 `status=open` 的成功连接延迟，拒绝、超时和错误耗时不参与 latency delta。

## MCP 给大模型调用

`vmbench mcp serve --transport stdio` 提供本地 MCP Server，让 Claude、Codex、Cursor、Cline 等客户端通过 tools 调用 vmbench，而不是让模型执行任意 shell。

首批 tools：

| Tool | 说明 |
|---|---|
| `vmbench_capabilities` | 输出版本、suite sections、presets、hardware tools、speed providers、workload 列表 |
| `vmbench_sysinfo` | 输出当前主机系统信息和 warning |
| `vmbench_run` | 运行原始 workload；MCP 默认 `scope=hardware`、`iterations=1` |
| `vmbench_suite` | 运行 VPS suite；MCP 默认只跑 `hardware`，网络 section 必须通过 preset 或 `only` 显式开启 |

MCP 输出仍然遵守 vmbench 的产品原则：只返回原始指标和结构化诊断，不输出 benchmark 总分、等级或 category score。IP Quality 的风险评分属于业务诊断，不是 benchmark 总分。

安全边界：

- stdout 只写 MCP JSON-RPC，日志/错误写 stderr。
- 不接受任意 shell 命令。
- `hardware_tools`、section、preset、speed provider 都限制在内置枚举。
- CLI、TUI、MCP 共同使用规范化后的 Suite 配置字段：iterations、timeout、hardware tools、speed providers、iperf hosts、IP version、sections、route selection、`catalog_source`、`catalog_revision`。
- 三个入口复用同一校验/归一化契约，但按交互场景暴露字段子集；Go TUI 在低于 40 行时使用紧凑 Suite 视图，配置、运行和结果页可在 `80x24` 内完整操作和查看状态。
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
	Scope:         vmbench.ScopeHardware,
	HardwareTools: []string{"sysbench", "openssl", "fio", "dd"},
})
```

需要网络 workload 时显式使用 `vmbench.ScopeNetwork` 或 `vmbench.ScopeAll`，并可设置 `CatalogSource` / `CatalogRevision`；规范化后 config 记录 `catalog_source` / `catalog_revision` / `node_ids`。报告 JSON 当前为 schema v2，config 保留 scope 和可选 iperf hosts，每项保留实际迭代次数、`samples_ms`、吞吐、延迟和结构化错误；`bytes_processed` / `ops_processed` 只在 workload 明确报告累计字节/操作数时出现，不会从 events/s、IOPS、MB/s 或 score 猜测。任一选中 workload 失败时 CLI 返回非零状态。

## 设计原则

- 不强行归一化
- 不伪造总分
- 保留失败信息
- 保留原始细节
- 对比优先于排名
- 预设只表达测评场景，不表达性能等级

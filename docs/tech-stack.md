# vmbench 技术栈

> 最新实现快照与运行语义，见 [`current-state.md`](current-state.md) 与 [`CHANGELOG.md`](CHANGELOG.md)。

## 语言与运行时

| 项目 | 说明 |
|---|---|
| 语言 | Go 1.26.5 |
| CLI | 标准库 `flag` |
| TUI | Bubble Tea / Lip Gloss |
| 系统信息 | gopsutil |
| 报告 | Console / JSON / HTML |
| MCP Server | 标准库 JSON-RPC over stdio，暴露给大模型客户端 |

## 当前架构

```text
vmbench/
├── bench/              # workload 接口、runner 与网络 probe
│   ├── integer/        # legacy 进程内 workload，不注册到硬件跑分
│   ├── float/          # legacy 进程内 workload，不注册到硬件跑分
│   ├── memory/         # legacy 进程内 workload，不注册到硬件跑分
│   ├── diskio/         # legacy 进程内 workload，不注册到硬件跑分
│   ├── netio/          # identity/reachability/ping/route/speed/ip/media/mail probe
│   └── runner.go
├── catalog/            # 外部工具 workload registry + parser
├── nodecatalog/        # embedded snapshot、strict loader、Ed25519 update、health
├── history/            # 原子本地报告存储
├── cmd/
│   ├── vmbench/        # CLI/TUI 入口
│   │   ├── mcp.go      # vmbench mcp serve 入口
│   └── vmbench-rendertest/ # TUI 渲染快照(build tag rendertest)
├── mcp/                # MCP JSON-RPC stdio server + tools/list/tools/call
├── report/             # JSON/HTML/console/compare
├── suite/              # VPS composite suite + section event
├── suitecompare/       # Suite raw metric alignment + compatibility gate
├── sysinfo/            # 系统信息采集
├── tui/                # Bubble Tea TUI
│   ├── theme/          # 8 主题 + AdaptiveColor
│   ├── comp/           # 组件库:card/progress/spinner/...
│   ├── dashboard.go
│   ├── running.go / results.go / compare.go
│   ├── suite_config.go / suite_running.go / suite_results.go
│   └── config.go       # TUI 主题持久化
├── run.go              # RunCore 编排
├── config_validation.go # run 共享校验/归一化 + catalog resolve
├── emit.go             # 事件发射
├── events.go           # Event 类型
├── options.go          # run 参数数据模型与内部默认值
```

## MCP 架构

`mcp/` 包实现轻量 MCP JSON-RPC server，当前只支持 stdio transport：

```text
cmd/vmbench/mcp.go
  -> mcp.ServeStdio(ctx, stdin, stdout, stderr)
     -> initialize / ping / tools/list / tools/call
        -> vmbench_capabilities
        -> vmbench_sysinfo  -> sysinfo.Collect
        -> vmbench_run      -> vmbench.RunCore
        -> vmbench_suite    -> suite.Run
```

实现约束：

- 不新增第三方依赖，避免扩大 release 体积和供应链风险。
- stdout 只写 JSON-RPC response；stderr 用于 server 诊断。
- tool input schema 使用 enum 限定 section、preset、hardware tool 和 speed provider。
- CLI、TUI、MCP 复用同一 Suite 参数归一化/校验契约；各入口按场景暴露字段子集，TUI 覆盖 iterations、timeout、hardware tools、speed providers、iperf hosts、IP version、sections、route selection、`catalog_source`、`catalog_revision`，CLI/MCP 另可传 filter 等自动化参数。
- Go TUI 在低于 40 行时为 SuiteConfig、SuiteRunning、SuiteResults 切换紧凑布局；`80x24` 下使用当前字段卡或逐 section 单行摘要，并保持完整卡片布局用于更高终端。
- catalog source 只接受 `embedded`、`auto` 或显式路径；revision pin 在探测前校验。
- MCP 默认 `iterations=1`，`vmbench_run` 默认 `scope=hardware`，`vmbench_suite` 默认只跑 `hardware`。
- 省略 `timeout_ms` 时默认 5 分钟；显式非正或超出 15 分钟上限的 iterations/timeout、非法 regex，以及混入未知值的 section/provider/tool/route preset 数组都会使整个 tool call 校验失败，不启动测量。
- `Server` 内部有运行互斥锁，同一时刻只允许一个 benchmark。
- tool 返回 MCP `content` 文本摘要和 `structuredContent` 结构化报告。测量失败时仍保留完整 `structuredContent.report`，并设置 `isError=true`；参数校验失败只返回错误文本和 `isError=true`。

首批 tools 不做 report 缓存、HTTP transport、progress notification 或 cancel API；这些能力可作为后续阶段补充。

## Benchmark 数据模型

`bench.RunDetail` 保留测量值：

- `iterations`
- `median_time`
- `samples`
- `throughput`
- `throughput_unit`
- `bytes_processed`
- `ops_processed`
- `average_latency_ns`
- `detail`
- `error`

报告层 `report.ResultEntry` 转成 JSON-friendly 字段：

- `iterations`
- `median_ms`
- `samples_ms`
- `throughput_per_sec`
- `throughput_unit`
- `bytes_processed`
- `ops_processed`
- `avg_ns_per_access`
- `detail`
- `error`

`bytes_processed` / `ops_processed` 是可选累计量：只有 workload 通过 `ProcessedMetricReporter` 明确声明 `ProcessedBytes` / `ProcessedOperations`，且所有成功 sample 的语义一致时才写入。dd 与 HTTP download/upload 可写累计 bytes，sysbench memory latency 只有解析到 total events 时才写 ops；events/s、IOPS、MB/s、score、latency 或其他语义未知值不会被猜测到任一字段。

报告根节点使用 `schema_version: 2`。`config` 记录 `scope`、实际启用的可选 `iperf_hosts`，以及 network/all 的 `catalog_source/catalog_revision/node_ids`；hardware scope 的 `extensions=false` 且清除 network provenance，network/all scope 为 `true`。未启用 network/speed 时，规范化层会清除未使用的 iperf host。项目不再包含 `score/` 包，也不再输出 benchmark 总分。

Runner 行为：

- workload 始终串行、隔离执行，不并发不同 benchmark，也不修改进程级 `GOMAXPROCS`、GC 或线程绑定状态。
- `--mode single` 是标准模式；旧的 `multi` / `all` 仍可被 CLI 接受，但会输出兼容警告、归一化为 `single`，并且只运行一次外部工具 catalog。线程数和队列深度由 sysbench/fio/OpenSSL/WinSAT 各自参数定义。
- 硬件 workload 使用请求的 1-9 次迭代并聚合中位数；所有 `bench/netio` workload 通过 `IterationLimiter` 限制为一次真实探测，并在结果中记录实际 `iterations: 1`。
- `vmbench run` 默认 `scope=hardware`；只有显式指定 `--scope network` 或 `--scope all` 才注册网络 workload。CLI 会在启用网络 scope 时提示基础 workload 最多约 1.75 GB 流量，另加可选 speedtest/iperf 流量。
- CLI 对非法 mode/scope/filter/iteration/tool 直接返回参数错误；没有 workload 命中或任一 workload 失败时，`run` 返回退出码 1。
- `OnWorkloadStart` 在每个 workload 的首个 sample 进入前同步触发，`OnWorkloadDone` 在该 workload 返回后立即触发；`RunCore` 据此逐项发射 `suite_start` 与 `suite_done` / `suite_fail`，不等待整批结束。同名 workload 也会逐项发射，不按名称去重。

## Hardware 外部工具模型

硬件 section 只注册外部工具 workload：

| 维度 | 工具 | 指标 |
|---|---|---|
| CPU | `sysbench cpu` / optional `geekbench` / optional `winsat cpu` | events/sec / upstream score / MB/s |
| CPU crypto | `openssl speed` | MiB/s |
| Memory | `sysbench memory` read/write/rnd-read / optional `stream` / optional `mbw` / optional `winsat mem` | MiB/s / ops/s / ns/op / MB/s |
| Disk | `fio` 4K random read/write Q1/Q32 + 1M sequential read/write Q1/Q8 / optional `dd` / optional `winsat disk` | MiB/s / IOPS / ns/op / MB/s |

`catalog.ExternalHardwareDefinitionsForTools` 按 `hardware_tools` 注册外部工具 workload。默认值按平台选择：Linux 为 `sysbench,openssl,fio`，macOS 为 `openssl`，Windows 为 `winsat`；其余 adapter 可通过 `--hardware-tool` 显式启用。`catalog.MissingHardwareToolsForFilter` 先用与 runner 相同的 Definition Name/Category 正则语义筛选 adapter，再解析实际命令；`run` 和启用 hardware 的 `suite` 只在 stderr 提示当前 filter 会运行但缺失的工具，并在 Linux 上给出已知的 Debian/Ubuntu 包安装命令。预检只提前暴露环境问题：受影响 workload 仍作为 `error` 写入 console/JSON/HTML/TUI，不会被静默跳过。`MissingHardwareTools` 保留为无 filter 的兼容入口。

Linux 默认集中的 `sysbench` 内存 workload 拆为顺序读带宽、顺序写带宽、随机读延迟三项；`fio` 磁盘 workload 拆为 4K random read/write Q1/Q32 和 1M sequential read/write Q1/Q8 八项。runner 会在每次 iteration 后采集外部工具解析出的吞吐和延迟，再对样本取中位数，避免多次迭代时只使用最后一次外部工具解析值。可选 `dd` read 在 Linux 使用 `iflag=direct` 避免页缓存产生虚假吞吐；其他平台无法保证 uncached direct read，因此 fail-closed 并提示改用 fio。

外部工具解析顺序：

1. `PATH`
2. Linux 下检查**解析后的 vmbench 可执行文件相邻位置**：`<exe-dir>/binaries/<tool>_<arch>`，其次 `<exe-dir>/<tool>_<arch>`
3. 工具缺失时写入结构化 `error`

当前工作目录中的 `binaries/` 不参与工具发现。官方源码与 release 包默认不内置第三方 benchmark 二进制，避免许可证和可追溯性风险。

## Network Probe 可靠性

- `network_info` 通过显式网络 section 获取公网 IPv4/IPv6、ASN/provider/location，并只输出可验证的 `direct` / `translated` / `unknown` NAT 结论；hardware-only `run` 不会因此发起公网请求。
- `reachability` 以受限并发探测内置 website HTTPS 与 Telegram DC TCP 目标，逐项保留 protocol、endpoint、latency、HTTP status 和 error。
- Go 主线 traceroute 使用系统 `traceroute` / `tcptraceroute` / `tracepath`，Windows 使用 `tracert`；目标最多 4 路并发，并按 catalog IP family 解析地址。每个结果记录实际 `resolved_target`、`destination_reached`、`probe_protocol`、`probe_tool` 与 `status=ok|partial|error`。只有 hop 到达解析后的目标才是 `ok`；有有效 hop 但没有到达目标是 `partial`；命令缺失、没有有效 hop 或探测失败是 `error`。
- Net Ping 使用 `tcp-connect/go-net-dialer` 实际探测证据；connect 成功与 TCP RST（包括 connection refused/reset）都证明目标已响应，计入 RTT/received 而不算丢包。逐目标 `connection_state` 为 `open|refused|mixed|no_response`；真正的 timeout/无响应才计入 loss。全部目标失败时返回非 nil 聚合错误，同时保留结构化 results，成功结果的 0 latency/jitter/loss 仍显式序列化。
- Mail 与 IP Quality 的端口证据复用同一顺序 TCP 探测器，避免同时连接 `portquiz.net` 的多个端口触发突发限制；每项状态严格分类为 `open|refused|timeout|error`。DNS 解析超时属于探测 `error`，不会伪装成端口 `timeout`。
- IP Quality 采用 fail-closed：元数据、公网 IPv4、DNSBL 或 Port 25 探测未得到确定结论时保留 detail/error，但不生成 0-100 `score`。只有输入完整且各项得到确定结果时才计算业务风险分。
- DNSBL zone 并发查询；Cloudflare upload 使用流式请求体，避免为 50 MiB 上传数据分配同等大小内存。

## Versioned Node Catalog

`nodecatalog/` 管理只包含数据的节点快照：

- Manifest：`schema_version`、`revision`、`generated_at`、`expires_at`、`nodes[]`
- Node：稳定 `id`、`name`、`kind`、`region/city`、`carrier/asn`、`ip_family`、`protocol`、`endpoint/port/url`、`traffic_bytes`、`source`；download 的 `traffic_bytes` 是响应体读取上限
- kind：`download`、`upload`、`route`、`ping`、`route_ping`

加载模式为 `embedded`、`auto`、显式 JSON path。默认 `embedded` 保证离线确定性；`auto` 优先读取 user cache，缓存不存在/损坏/不匹配时回退 embedded。`--node-revision` 是精确 pin，任何候选 revision 不匹配都会在 probe 前失败。过期 snapshot 产生 warning，但不会静默替换数据。报告中的 source 规范化为 `embedded|auto|path`，真实本地路径只出现在管理 CLI 的 `path` 字段，避免泄露 home path。

`vmbench nodes update` 要求调用方提供 Ed25519 trust root 和 detached signature，签名覆盖 manifest 精确字节；通过签名和严格 schema 校验后，原子写入 user cache（Unix mode `0600`）。`nodes verify` 可只验证 schema/revision，也可验证 detached signature；`nodes health` 对 HTTP/DNS/TCP endpoint 做有界并发可用性检查并保留逐节点错误。当前 embedded snapshot 覆盖全球 download，以及广州/北京/上海/成都、三网、CERNET、CSTNET 和 IPv6 route/ping 证据。

## Suite Sections

```text
hardware
network_info
route
ping
speed
ip_quality
reachability
mail
media
```

对应 CLI：

```bash
vmbench suite --preset quick|website|proxy|mail
vmbench suite --only ping,mail
vmbench suite --skip media
vmbench suite --ip-version v4|v6|dual
vmbench suite --quiet --json suite.json
```

CLI 默认通过 `suite.Options.OnEvent` 把 `section.start`、完成/失败状态和 `suite.done` 实时写到 stderr，因此 JSON/HTML 输出路径和 stdout console 内容不受进度文本污染；`--quiet` 只关闭这条进度流。

`--preset` 在 `suite.Options.Preset` 中记录，并在 `Config.preset` 输出到 JSON/HTML/Console。预设只负责 section 编排：

| Preset | Sections |
|---|---|
| `quick` | `hardware,network_info,speed,ip_quality` |
| `website` | `hardware,network_info,route,ping,speed,ip_quality,reachability,mail` |
| `proxy` | `network_info,route,ping,speed,ip_quality,reachability,media` |
| `mail` | `network_info,route,ip_quality,mail` |

优先级：

1. `--preset` 选择默认 section 集合。
2. `--only` 会覆盖 preset 的 section 集合。
3. `--skip` 和 `--no-*` 在最终 section 集合上继续关闭指定 section。
4. `--ip-version`、`--route-presets` 等显式参数优先于 preset 默认值。

`speed` section 还支持 `suite.Options.SpeedProviders`：

| Provider | 说明 |
|---|---|
| `cloudflare` | Cloudflare 多线程下载 + 上传 |
| `speedtest_net` | Ookla Speedtest CLI JSON |
| `speedtest_cn` | speedtest.cn 兼容 CLI JSON |
| `iperf3` | 用户提供的 `--iperf-host` |

默认只启用 `cloudflare`。选择多个 provider 时，顶层 `speed.summary` 的 `aggregation` 为 `best_per_metric`，表示下载、上传、延迟可能分别取自不同 provider，不能把它解读为一次单节点测量。

输出结构：

- `speed.summary`：单 provider 汇总，或多 provider 的 `best_per_metric` 汇总
- `speed.groups[]`：按 provider 聚合后的分组结果
- `speed.providers[]`：每个 provider 的下载 / 上传 / 延迟 / 状态 / 错误
- `config.speed_providers`：本次启用的 provider 列表

Suite 只把 enabled 且 `status=ok` 的 section 视为成功。enabled section 的空状态、`skipped`、`partial`、`error` 都使 `SuiteReport.HasFailures()` 为 true、总体 `status=failed` 并发射 `section.fail`；只有 disabled section 发射 `section.skip` 且不单独构成失败。没有任何 enabled section 也视为失败。

`Options.Timeout` 默认 5 分钟。hardware 将它作为每个 workload 的 timeout，不再额外套 section deadline；其余网络 section 各自从调用方 context 派生 section timeout，deadline/cancel 会覆盖 section 为 `error` 并写入结构化 message，调用方更早的 deadline 始终优先。选择 iperf3 provider 却没有可用 host 时，speed provider/section 直接返回 error。

## Sysinfo Context

`sysinfo.Collect(ctx)` 将调用方 context 传给各平台 collector；已取消的 context 会立即返回结构化 warning。所有外部命令使用该 parent context，并额外设置最长 30 秒的子超时，因此调用方 cancel/deadline 可以提前终止 sysinfo 采集。`system.virtualization` 通过 gopsutil host 信息和 Linux/macOS/Windows fallback 输出 best-effort `system/role`，采集失败保留 warning。

## 事件系统

vmbench workload 事件:

| EventKind | 触发时机 |
|---|---|
| `suite_start` | 每个 workload 首个 sample 进入前；同名实例也逐项触发 |
| `suite_progress` | 迭代进度 |
| `suite_done` | 当前 workload 完成后立即发射 |
| `suite_skip` | workload 跳过 |
| `suite_fail` | workload 失败 |
| `bench_done` | run 完成 |
| `bench_log` | 警告/日志 |

Suite section 事件(`suite.Event`):

| EventKind | 触发时机 |
|---|---|
| `section.start` | section 启动 |
| `section.done` | section 完成且 status=ok |
| `section.fail` | enabled section 完成但 status≠ok（包括 skipped/partial/error/空状态） |
| `section.skip` | section 未启用 |
| `suite.done` | 全部 section 完成 |

事件只携带原始 metric,不携带总分/等级。TUI 通过 `suite.Options.OnEvent` 回调订阅，CLI 则用同一回调在 stderr 输出 section 生命周期；`--quiet` 时不安装 CLI 进度回调。

## 报告与对比

- JSON：机器可解析
- HTML：人类可读
- Console：终端表格
- Compare：自动识别 benchmark/Suite JSON，按 raw metric 对齐两份或更多报告
- History：本地 add/list/show/delete 与 `compare --last N`

Suite JSON 使用 schema v2 envelope：`schema_version=2`、`report_kind=suite`、唯一 `report_id`、app build、system、UTC timestamps/duration、规范化 config、catalog provenance 与九个 section。旧 `version=1` 和 Unix time 字段继续保留，避免破坏旧 consumer。Suite HTML 从同一结构渲染硬件 workload、network identity、route hops、ping、provider-level speed、IP quality、reachability、mail、media、warning/error；network-only Suite 也包含 system/app/catalog 元数据。CLI 的 JSON/HTML 导出在目标同目录创建 mode `0600` 临时文件，写入后执行 fsync 并 rename 替换，最终再次收紧为 `0600`；非 Unix 平台还依赖系统 ACL。

对比规则：

- time：越低越好
- latency：越低越好
- throughput：越高越好

Benchmark Compare 会忽略带 `error` 的 metric；`ms avg` 按 latency 处理（越低越好）；throughput 单位不兼容时不计算 delta。迭代次数、mode、scope、hardware tool 或 iperf host 选择不同，或单份报告出现重复 workload 时会输出可比性警告。

Suite Compare 同时检查 unit、实际 protocol/IP family、provider/probe tool、target/node identity，以及节点型指标所需的 catalog revision。只有全部兼容才计算 delta；不兼容时仍对齐展示原始值，并输出明确 reason/warning。Route 指标额外要求逐项显式为 `status=ok` 且 `destination_reached=true`，缺少新到达证据的旧报告 fail-closed 为 unavailable。HTTP status 等分类码不参与百分比 delta。硬件 time/latency 越低越好、throughput 越高越好；route hop count 等中性证据只展示，不伪造“提升”。不同 report kind 不允许混合比较，IP Quality PortProbe 的状态门禁不会影响未知扩展 section。

Mail Compare 只比较 `status=open` 的成功连接延迟；`refused/timeout/error` 的耗时分别是拒绝响应、超时阈值或失败开销，不作为可比较 latency。

`history/` 按平台 data directory 保存独立 JSON record，使用临时文件 + fsync + rename 原子落盘；Unix 目录 mode `0700`、文件 mode `0600`，其他平台依赖系统 ACL。`--save-history` 可从 run/suite 直接写入，`--history-tag` 只作标签；`history compare --last N` 要求最近 N 份记录属于同一 report kind。

## 构建一致性

`sh/build.sh` 显式设置 `CGO_ENABLED=0` 后构建 `/root/temp/vmbench`，与 GoReleaser 的无 CGO 构建策略保持一致，避免本地验证产物意外依赖宿主动态库。

## 许可证注意

vmbench 采用 MIT。借鉴其他项目时可以参考功能设计，但不要直接复制 GPL 项目的代码。

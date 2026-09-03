# vmbench 最新技术与产品梳理

这份文档用于快速对齐当前仓库里的“最新实现状态”。

## 产品定位

vmbench 是一个 Go 编写的跨平台 VPS / 主机测评工具，面向三类使用方式：

- CLI：批量执行、导出 JSON / HTML / Console
- TUI：交互式跑分、结果浏览、两份 benchmark 报告对比
- MCP：通过 stdio 暴露给大模型客户端安全调用
- Evidence：版本化节点、Suite v2、本地 history 和 compatible-only compare

产品原则保持不变：

- 只输出原始指标，不输出综合总分
- 保留结构化错误和 detail
- Compare 只基于 time / throughput / latency
- Suite delta 只在 unit/protocol/provider/node/catalog revision 兼容时计算
- IP Quality 的 0-100 评分是业务诊断，不是 benchmark 总分

## 最新技术状态

### 1. Runner / workload 调度

- workload 始终串行、隔离执行，不并发不同 benchmark，也不修改进程级 runtime/GC/线程状态
- `single` 是标准执行模式；旧的 `multi` / `all` 只为兼容保留，会输出 warning、归一化为 `single`，且只运行一次外部工具 catalog
- `vmbench run` 默认 `scope=hardware`；网络 workload 只在显式 `--scope network` / `--scope all` 时注册，并提示约 1.75 GB 基础流量
- 硬件 workload 使用请求的 1-9 次迭代；所有网络 workload 最多执行一次真实探测并记录实际 `iterations=1`
- workload start event 在首个 sample 前逐项同步发射，done/fail 在当前 workload 返回后立即发射；同名 workload 也不会合并
- CLI 对非法参数返回退出码 2；没有 workload 命中或任一 workload 失败时返回退出码 1

### 2. Benchmark / probe 形态

- 硬件测评只注册外部工具 workload；默认值按平台为 Linux `sysbench/openssl/fio`、macOS `openssl`、Windows `winsat`
- 可显式选择：`sysbench` / `openssl` / `fio` / `dd` / `stream` / `mbw` / `geekbench` / `winsat`
- Linux 本地 fallback 只搜索解析后的 vmbench 可执行文件相邻位置，不搜索当前工作目录的 `binaries/`
- `bench/netio` 负责网络诊断和速度测试：
  - public IPv4/IPv6 / ASN / provider / NAT evidence
  - website / Telegram reachability
  - ping
  - route / traceroute
  - download / multi-download / upload
  - ip quality
  - media unlock
  - iperf3
- traceroute 使用系统 `traceroute` / `tcptraceroute` / `tracepath` / `tracert`，最多 4 路并发；逐项记录解析目标、是否到达和 `ok/partial/error`，命令缺失或无有效 hop 结构化报错
- TCP Ping 将 connect 成功和 RST/refused 都作为响应计入 RTT/received，并用 `connection_state` 区分 open/refused/mixed/no_response
- Mail 端口顺序探测并区分 `open/refused/timeout/error`；DNS 失败保持为 probe error
- IP Quality fail-closed：元数据、公网 IPv4、DNSBL 或 Port 25 结论不完整时不生成风险 score；DNSBL zone 并发查询
- Cloudflare upload 使用流式请求体，避免分配 50 MiB payload；所有 HTTP 传输保留状态码/body copy 错误
- `nodecatalog/` 用 embedded/auto/path 三种 source 提供版本化节点；revision pin 在 probe 前检查，Ed25519 signed update 通过严格 schema 验证后才原子写入缓存
- embedded snapshot 已覆盖成都、CERNET、CSTNET 与 IPv6 route/ping；节点 ID、protocol、ASN、流量预算和 revision 进入可追溯证据

### 3. Suite 产品

- section：`hardware / network_info / route / ping / speed / ip_quality / reachability / mail / media`
- preset：`quick / website / proxy / mail`
- speed providers：`cloudflare / speedtest_net / speedtest_cn / iperf3`
- speed 默认只启用 `cloudflare`；选择多个 provider 时顶层 summary 标记 `aggregation=best_per_metric`
- suite 输出保留：
  - `summary`
  - `groups`
  - `providers`
  - `sections`
- Suite 使用 schema-v2 envelope：`report_kind/report_id/app/system/timestamps/duration/config`，同时保留 v1 compatibility fields
- Suite 只有所有 enabled section 都为 `ok` 才成功；enabled 的空/skipped/partial/error 状态均使 CLI 返回退出码 1，disabled 才只发 skip event
- 默认 timeout 为 5 分钟：hardware 按 workload 应用，其他网络 section 各自派生 timeout context，并把 deadline/cancel 写成结构化 error
- Net Ping 全目标失败会保留逐目标 results 并返回聚合 error；iperf3 provider 无可用 host 直接失败
- suite 事件驱动仍然是主线：start / done / fail / skip / suite.done
- `vmbench compare` 自动识别 benchmark/Suite；Suite 只有在 unit/protocol/provider/target-node/catalog revision 兼容时才产生 delta，Route 另要求显式到达目标，Mail/IP Quality 端口延迟只接受 open
- `history add/list/show/delete/compare --last N` 提供原子本地记录（Unix `0700/0600`）；run/suite 可用 `--save-history` 直接落盘

### 4. MCP

- `vmbench mcp serve --transport stdio`
- 首批 tools：
  - `vmbench_capabilities`
  - `vmbench_sysinfo`
  - `vmbench_run`
  - `vmbench_suite`
- 默认安全策略：
  - `vmbench_run` 默认 `scope=hardware`
  - `vmbench_suite` 默认只跑 `hardware`
  - `iterations` 默认 1，最大 9
  - `timeout_ms` 最大 15 分钟
- MCP 严格区分省略值与显式非法值：非法 iterations/timeout/regex 或混入未知项的枚举数组直接拒绝，不启动测量
- workload/section 测量失败仍返回完整 `structuredContent.report`，同时设置 `isError=true`
- CLI/TUI/MCP 共享 Suite 配置字段与校验：iterations/timeout/filter、sections、IP version、tools/providers/iperf、route、catalog source/revision

### 5. TUI / 报告

- 8 套主题，支持本地持久化
- Dashboard / Running / Results / Compare / SuiteConfig / SuiteRunning / SuiteResults
- Go TUI 只保留 Hardware Benchmark 入口，不再提供独立 Multi-Core 入口；SuiteConfig 默认实际应用包含 `network_info` 的 Quick sections 和 Cloudflare provider，并使用与 CLI/MCP 相同的 catalog/config 模型
- Results 只展示原始时间、吞吐、延迟、detail/error
- benchmark JSON 使用 schema v2；config 记录实际 scope/可选 iperf hosts，network/all 另记录 catalog source/revision/node IDs；hardware 清除网络 provenance 且 `extensions=false`
- 结果保留实际 iterations 与 `samples_ms`；processed 字段仅在明确为累计 bytes/ops 且 sample 语义一致时出现
- TUI benchmark Compare 忽略 error metric；CLI/history Suite Compare 另检查 protocol/provider/node/catalog revision 与 Route 到达证据，并提示所有不兼容原因
- Console / JSON / HTML 是同一数据模型的不同视图
- Suite HTML 已覆盖硬件 workload、网络身份、route hops/到达状态、ping connection state、speed/IP/reachability/mail/media 明细与结构化失败；TUI 在 IP Quality fail-closed 时保留 error、风险摘要和 Port 25 证据
- sysinfo 采集向所有平台 collector 传递调用方 context；外部命令同时受 parent deadline 与 30 秒上限约束

## 文档入口

- [产品说明](product.md)
- [技术栈](tech-stack.md)
- [TUI 设计](tui-design.md)
- [变更记录](CHANGELOG.md)

## 当前结论

当前仓库的技术主线已经收敛为：

1. 外部工具硬件测评
2. 版本化节点 + network identity / route / ping / speed / reachability / media / mail / IP quality
3. CLI + TUI + MCP 三入口
4. Suite v2 + compatible-only compare + 本地 history + 结构化错误

后续新增能力，应优先补到 `docs/product.md`、`docs/tech-stack.md` 和 `docs/CHANGELOG.md`，保持产品文档、技术文档和变更记录同步。

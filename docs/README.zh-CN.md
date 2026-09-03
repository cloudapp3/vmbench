# vmbench 中文说明

vmbench 是一个 Go 编写的跨平台 VPS / 主机测评工具，提供 CLI、TUI、版本化网络节点、JSON/HTML、Suite 对比和本地历史能力。

## 最新梳理

- [最新技术与产品梳理](current-state.md)
- [产品说明](product.md)
- [技术栈](tech-stack.md)
- [TUI 设计](tui-design.md)
- [变更记录](CHANGELOG.md)

## 产品原则

- 输出原始指标，不输出综合总分
- 保留结构化错误，便于自动化系统判断工具缺失或网络受限
- Compare 只基于原始指标：
  - time / latency 越低越好
  - throughput 越高越好
- IP Quality 的 0-100 风险评分属于业务诊断，不等于 benchmark 总分

## 快速开始

```bash
# 从源码安装
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest

# 或本地构建
go build -o vmbench ./cmd/vmbench

# 项目验证脚本使用 CGO_ENABLED=0，另输出 /root/temp/vmbench
./sh/build.sh

# 默认进入 TUI
./vmbench

# CLI 测评
./vmbench run
./vmbench run --json report.json --html report.html
./vmbench run --filter 'sysbench|fio|OpenSSL'
./vmbench run --hardware-tool sysbench,openssl,fio,dd
./vmbench run --scope network --iterations 1
./vmbench run --scope all --iterations 1

# VPS 综合测评
./vmbench suite
./vmbench suite --preset quick
./vmbench suite --preset website --json suite.json --html suite.html
./vmbench suite --only ping,mail
./vmbench suite --only hardware --hardware-tool dd,stream,mbw
./vmbench suite --node-catalog auto --node-revision 2026-07-13.1 --save-history
./vmbench suite --quiet --json suite.json

# 报告对比
./vmbench compare a.json b.json
./vmbench history compare --last 3

# 节点目录
./vmbench nodes list --node-catalog embedded
./vmbench nodes health --node-catalog auto --ip-family v6
```

`vmbench run` 默认只运行 `hardware` scope。`network` / `all` 必须显式选择，CLI 会提示基础网络 workload 可能传输约 1.75 GB 数据；所有网络 workload 最多执行一次真实探测。workload 始终串行隔离，旧的 `--mode multi/all` 只保留兼容 warning，不会并发 workload 或生成第二轮重复结果。

## Suite sections

| Section | 说明 |
|---|---|
| `hardware` | CPU / 内存 / 磁盘外部工具测评 |
| `network_info` | 虚拟化、公网 IPv4/IPv6、ASN/provider/location、保守 NAT 证据 |
| `route` | 版本化三网/成都/CERNET/CSTNET/IPv4/IPv6 回程路由，保留是否到达目标的证据 |
| `ping` | 同一节点目录上的 TCP 延迟 / jitter / loss / connection state |
| `speed` | Cloudflare / Speedtest.net / Speedtest.cn / iperf3 |
| `ip_quality` | IP reputation / DNSBL / 邮件端口风险诊断 |
| `reachability` | 网站 HTTPS 与 Telegram DC TCP 可达性、latency/status/error |
| `mail` | 顺序探测邮件端口，区分 open/refused/timeout/error |
| `media` | 流媒体 / AI 服务解锁检测 |

Suite speed 默认只启用 Cloudflare。Suite 只有每个 enabled section 都是 `ok` 时成功；enabled 的空状态、`skipped`、`partial`、`error` 都使 CLI 返回非零，disabled 才只发 skip event。多个 speed provider 的顶层 summary 使用 `aggregation=best_per_metric`，下载、上传和延迟可能来自不同 provider。

默认 timeout 为 5 分钟：hardware 按 workload 应用，其余网络 section 各自派生 timeout context。Suite CLI 默认在 stderr 实时显示每个 section 的 running/最终状态；`--quiet` 关闭进度，但不改变 Console/JSON/HTML 报告。

TCP Ping 将连接成功和 TCP RST/refused 都视为收到目标响应，计入 RTT/received 而不算丢包；`connection_state` 为 `open|refused|mixed|no_response`，真正 timeout/无响应才计入 loss。全部目标失败仍保留逐目标结果并返回聚合错误。Route 结果记录 `resolved_target`、`destination_reached` 和 `status=ok|partial|error`：有有效 hop 但没有到达解析目标时是 `partial`。Mail 对 8 个内置端口顺序连接，逐项状态为 `open|refused|timeout|error`；DNS 失败保持为探测 `error`，Compare 只使用 `open` 连接延迟。选择 iperf3 provider 却没有可用 host 也会直接失败。

常用参数：

```bash
vmbench suite --preset quick|website|proxy|mail
vmbench suite --only ping,mail
vmbench suite --skip media
vmbench suite --ip-version v4|v6|dual
```

网络节点来自版本化 catalog。`--node-catalog` 支持 `embedded`、`auto` 或 JSON 路径；`--node-revision` 固定精确 revision，不匹配时在 probe 前失败。`auto` 只使用已验证缓存并可回退 embedded，不在测评时隐式下载。`vmbench nodes verify/update` 使用调用方显式提供的 Ed25519 公钥和 detached signature；`nodes health` 进行有界可用性检查。

## TUI

直接运行：

```bash
vmbench
```

Dashboard 支持：

- 运行串行隔离的外部工具硬件测评（无独立 Multi-Core 入口）
- 运行 VPS suite
- 配置与 CLI/MCP 相同的 iterations/timeout/IP version/tools/providers/iperf/sections/route/catalog source/revision
- 打开系统信息
- 比较两份 benchmark 报告；Suite Compare 使用 CLI/history
- 按 `t` 循环切换 8 种颜色主题，退出后持久化到本地配置

## 外部工具策略

硬件测评只调度外部工具：

- Linux 默认：`sysbench` / `openssl` / `fio`
- macOS 默认：`openssl`
- Windows 默认：`winsat`
- `dd`：可选磁盘顺序写/读；Linux read 使用 `iflag=direct`，其他平台无法保证 uncached read 时 fail-closed 并提示使用 fio
- `stream` / `mbw`：可选内存带宽
- `geekbench`：可选 CPU upstream score，不默认跑，不作为 vmbench 总分
- `winsat`：Windows CPU / 内存 / 磁盘（Windows 默认，也可显式选择）

`run` 和启用 hardware 的 `suite` 会在执行前提示当前 filter 会命中的缺失工具，Linux 还会输出已知 Debian/Ubuntu 安装命令。未被 filter 选中的 adapter 不产生误导提示；实际缺失的工具不会触发进程内 fallback 或被静默跳过，而是进入结构化 `error` 字段。官方源码和 release 包默认不内置第三方二进制工具；Linux 本地 fallback 只从解析后的 vmbench 可执行文件相邻 `binaries/` 或同目录加载，例如 `<exe-dir>/binaries/sysbench_x64`，不会搜索当前工作目录。

## 报告与网络失败语义

- benchmark JSON 使用 schema v2；config 记录规范化后的 `scope` 与可选 `iperf_hosts`，network/all 另记录 `catalog_source/catalog_revision/node_ids`；hardware 清除网络 provenance 且 `extensions=false`。
- 每项结果包含实际 `iterations` 和 `samples_ms`；`bytes_processed` / `ops_processed` 只在 workload 明确报告累计字节/操作数且 sample 语义一致时出现，不从速率、score 或 latency 猜测。
- `run` 没有匹配 workload 或任一 workload 失败时返回退出码 1；非法 regex/mode/scope/iteration/tool 返回参数错误。
- MCP 省略 iterations/timeout 时使用 1 次/5 分钟默认值；显式非法数值、regex 或混入未知项的枚举数组直接以 `isError=true` 拒绝。测量失败仍保留完整 `structuredContent.report` 并标记 `isError=true`。
- Go traceroute 依赖系统 `traceroute` / `tcptraceroute` / `tracepath` / `tracert`；逐目标保留解析地址、是否到达和状态，命令缺失或无有效 hop 会结构化报错，未到目标但有 hops 则为 `partial`。
- IP Quality 只有在元数据、公网 IPv4、DNSBL 和 Port 25 探测均得到确定结论时才生成 0-100 风险 score，不确定时 fail-closed 并保留 error/detail。
- Suite JSON 使用 schema-v2 envelope，包含 report/app/system/time/config/catalog provenance，并保留旧 v1 字段；Route 包含 `resolved_target/destination_reached/status`，Ping 包含 `connection_state`。Suite HTML 展示硬件 workload、网络身份、完整 route hops、各网络 section 明细和 error。
- `vmbench compare` 自动识别 benchmark/Suite，支持两份以上报告；Suite 只有 unit、protocol、provider、target/node 和所需 catalog revision 兼容时才计算 delta。Route 还必须显式包含 `status=ok` 与 `destination_reached=true`，旧报告缺少到达证据时不计算 delta。
- CLI 的 `--json` / `--html` 使用同目录临时文件、fsync、rename 原子导出，Unix mode 为 `0600`。`run` / `suite --save-history [--history-tag TAG]` 可写入原子本地历史（Unix 目录 `0700`、文件 `0600`）；`history add/list/show/delete/compare --last N` 管理和比较同类型报告。

## 文档

- [产品说明](product.md)
- [技术栈](tech-stack.md)
- [TUI 设计](tui-design.md)
- [变更记录](CHANGELOG.md)

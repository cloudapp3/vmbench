# vmbench TUI 设计

> 本文记录当前 Go Bubble Tea TUI 的页面与配置契约；视觉组件细节另见 [`tui-redesign.md`](tui-redesign.md)。

## 页面结构

```text
Dashboard -> Running -> Results
Dashboard -> SuiteConfig -> SuiteRunning -> SuiteResults
Dashboard -> Compare
Dashboard -> System Info
```

`vmbench ecs-diff` 仍以 CLI/JSON 输出为主，`vmbench mcp serve` 是给大模型客户端使用的后台 stdio server，两者不进入 TUI 页面路由。

## Dashboard

展示：

- 当前版本
- CPU / Memory / OS / GPU 摘要（GPU 可用时）
- Go 主线入口菜单：
  - Run Hardware Benchmark
  - Run Suite (VPS Composite)
  - Compare Reports
  - System Info
  - Quit

说明：Go TUI 已移除误导性的独立 Multi-Core 入口。硬件 workload 串行执行，CPU 线程数和磁盘队列深度由外部工具参数定义；默认工具按平台选择，Linux 为 sysbench/OpenSSL/fio，macOS 为 OpenSSL，Windows 为 WinSAT。sysbench 拆出 memory read/write/latency，fio 拆出 4K random read/write Q1/Q32 与 1M sequential read/write Q1/Q8。CLI/Suite 可通过 `--hardware-tool` 显式选择其他 adapter；工具缺失时展示结构化错误，不再提供进程内 benchmark fallback。

按键：

- `↑↓` / `j k`：选择
- `Enter`：确认
- `t`：循环切换主题
- `q`：退出

## Running

展示：

- 当前阶段
- 总进度
- workload 状态
- workload 完成后的原始 metric

Go runner 始终串行执行 workload；进度总数按各 workload 的实际迭代次数计算。硬件 workload 使用配置的迭代数，网络 workload 限制为一次真实探测。旧的 `multi/all` mode 不会产生第二轮结果或并发不同 workload。

Go TUI 使用 spinner、progress bar、event log viewport 和取消 modal 展示执行状态。

状态：

- waiting
- running
- done
- fail
- skip

按键：

- `Esc`：取消确认
- `q`：退出

## Results

不再显示分数卡、等级、category score。

展示：

```text
Benchmark Results
workloads: 17  ok: 17  failed: 0

view: flat [Tab to toggle]
Workload                 Category      Time      Throughput       Latency      Status
CPU Single-Core          CPU           1010.0ms  532 events/sec   -            ok
Disk 4K Random Read Q1   Disk          3000ms    18200 IOPS       54000ns      ok
```

Go TUI 支持交互式视图切换：

- flat：平铺 workload
- grouped：按 category 展开/折叠

按键：

- `Tab`：切换视图
- `↑↓` / `j k`：移动
- `Enter`：展开/折叠
- `s`：保存 JSON
- `Esc`：返回 Dashboard

## SuiteConfig 与 SuiteResults

Go SuiteConfig 初始选择 `quick` preset，并实际启用 `hardware,network_info,speed,ip_quality`；speed provider 默认只选 Cloudflare。用户切换 preset 后，section 集合同步更新。

TUI 不维护另一套隐式默认值，而是构造与 CLI/MCP 相同的规范化 Suite 配置。可配置字段覆盖：preset/sections、iterations、timeout、IP version、hardware tools、speed providers、iperf hosts、route selection、catalog source 和 catalog revision。catalog source 支持 `embedded` / `auto` / 显式 path；revision pin 不匹配时停在运行前错误状态。

Go TUI 在终端低于 40 行时使用紧凑 Suite 布局：Config 只展开当前聚焦字段，Running 与 Results 对每个 section 使用单行状态摘要；在 `80x24` 下页面宽高均受终端边界约束，字段导航、启动和取消仍可操作。更高终端继续显示完整卡片与详细结果。

Go 主线覆盖 `hardware / network_info / route / ping / speed / ip_quality / reachability / mail / media` 九个 section。Network Info 展示虚拟化、公网 IP、ASN/provider 和 NAT 证据；Reachability 展示 website/Telegram 的 protocol/latency/status/error。所有这些状态都不会折算为 benchmark 总分。Suite 只有所有 enabled section 都是 `ok` 时成功；enabled 的空状态、`skipped`、`partial`、`error` 均表示失败，disabled section 才只发 skip event。网络 section timeout/cancel 也会显示为结构化 `error` message。

Route/Ping 的完整 JSON、Console 和 HTML 会区分 catalog protocol 与实际 `probe_protocol/probe_tool`。TUI 的完整卡片和 `80x24` 紧凑摘要保留 section 状态，不在界面层重新推断协议；CLI/history Compare 直接使用报告中的实际协议、工具和 IP family 做兼容门槛。

## Compare

Go TUI 当前对比两份 benchmark JSON。Suite Compare 使用 CLI `vmbench compare` 或 `history compare --last N`，不把 Suite JSON 作为 benchmark 文档载入 TUI。

展示：

- 系统信息差异
- 每个 workload 的：
  - time delta
  - throughput delta
  - latency delta

方向规则：

- time 越低越好
- latency 越低越好
- throughput 越高越好

CLI/history 的 Suite delta 额外要求 unit、protocol、provider、target/node identity 和所需 catalog revision 一致；不兼容时保留原始值和原因，但不显示伪 delta。TUI benchmark Compare 仍按 workload 的 time/throughput/latency 规则工作。

## 样式原则

- 绿色：ok / 提升
- 红色：fail / 下降
- 灰色：等待 / 无数据
- 蓝色：主标题 / 当前选择

不使用等级色彩，不使用 S/A/B/C/D/F。

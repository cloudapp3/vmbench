# vmbench TUI 设计

> 本文记录当前 Go Bubble Tea TUI 的页面与配置契约；视觉组件细节另见 [`tui-redesign.md`](tui-redesign.md)。

## 页面结构

```text
Dashboard -> RunConfig -> Running -> Results -> ResultDetail
Dashboard -> SuiteConfig -> SuiteRunning -> SuiteResults
Dashboard -> ComparePicker -> Compare
Dashboard -> System Info
任意页 -> Help（? 切换，Esc 返回来源页）
```

`vmbench mcp serve` 是给大模型客户端使用的后台 stdio server，不进入 TUI 页面路由。

## 全局按键与滚动

所有页面共享一组全局按键，注册表在 `tui/help.go`（footer 提示与帮助页同源）：

- `?`：打开/关闭帮助页（文本输入聚焦时抑制）
- `PgUp` / `PgDn`：按视口高度翻页
- `Home` / `End`：跳到顶部/底部
- 鼠标滚轮：上下滚动 3 行
- footer 右侧在内容可滚时显示 `n/m` 位置提示

滚动实现是中央裁剪（`tui/scroll.go`）：每帧渲染完整页面内容后按 `offset` 切片，页面切换与窗口 resize 自动归零 offset；每行再做 ANSI 感知的宽度守卫（`i18n.TruncateStyled`），保证 80 列终端不出现软折行撑破视口。光标移动（Results 表格、ComparePicker 列表）后滚动最小量保持焦点行可见。

鼠标点击目前只在 Dashboard 菜单区做命中测试（点击 = 选中 + 确认）；主题行点击切换主题。

## Dashboard

展示：

- 当前版本
- CPU / Memory / OS / GPU 摘要（GPU 可用时）
- Go 主线入口菜单：
  - Run Hardware Benchmark（进入 RunConfig 页）
  - Run Suite (VPS Composite)
  - Compare Reports（进入 ComparePicker 页）
  - System Info
  - Quit

说明：Go TUI 已移除误导性的独立 Multi-Core 入口。硬件 workload 串行执行，CPU 线程数和磁盘队列深度由外部工具参数定义；默认工具按平台选择，Linux 为 sysbench/OpenSSL/fio，macOS 为 OpenSSL，Windows 为 WinSAT。sysbench 拆出 memory read/write/latency，fio 拆出 4K random read/write Q1/Q32 与 1M sequential read/write Q1/Q8。CLI/Suite 可通过 `--hardware-tool` 显式选择其他 adapter；CLI 会在开始前提示当前 filter 涉及的缺失工具，TUI/报告继续展示结构化错误，不提供进程内 benchmark fallback。可选 dd read 只有 Linux 能以 direct I/O 运行，其他平台 fail-closed 并提示改用 fio。

按键：

- `↑↓` / `j k`：选择（鼠标点击菜单行等效）
- `Enter`：确认
- `t`：循环切换主题（鼠标点击主题行等效）
- `q`：退出

## RunConfig

硬件跑分前的配置页（`tui/run_config.go`），取代旧的"直接开跑"。Start 时构造与 CLI 相同的 `vmbench.Options`（`Scope=hardware, Mode=single, Engine=external`）并经 `NormalizeOptions` 校验。

字段：

- Iterations：1-9，默认 3
- Hardware Tools：多选，默认平台推荐集合；决定 workload 集合
- Filter：All / CPU / Disk / Memory / Custom（Custom 为正则手输，语义与 CLI `--filter` 一致，匹配 workload Name/Category）
- Preflight：进入页面异步检查所选工具缺失情况，warning 卡非阻塞（对齐 CLI 行为）
- Start：按钮行；无可运行 workload 时置灰并提示

页面顶部实时显示 "N workloads planned"——该计数镜像 runner 的工具×过滤逻辑，Running 页预填的 workload 列表来自同一来源，不会出现永远 waiting 的幽灵行。

按键：

- `↑↓` / `Tab`：字段间移动
- `←→`：调节当前字段（iterations 增减、tools 光标移动、filter chip 轮换）
- `spc` / `x`：切换工具选中
- `Enter`：开始运行（聚焦 Start 时）；Custom filter 聚焦时确认输入
- `Esc`：返回 Dashboard

## Running

展示：

- 当前阶段与 ETA（首个 workload 完成后显示 `~xx left`，按已完成 workload 墙钟均值外推）
- 采样进度（`n/m samples`，来自 `EventSuiteProgress`）
- workload 状态、迭代迷你条（`▰▱ 2/3`）与当前 workload 已耗时（墙钟）
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

- cards：按 category 分组的摘要卡
- grouped：按 category 展开/折叠
- flat：平铺 workload 表格，光标可移动

按键：

- `Tab`：切换视图（cards -> grouped -> flat）
- `↑↓` / `j k`：移动光标（grouped/flat）
- `Enter`：grouped 下展开/折叠 category
- `d`：flat 下打开光标行的 ResultDetail
- `s`：保存 JSON
- `Esc`：返回 Dashboard（从 ComparePicker 查看进入时返回 picker）

## ResultDetail

单个 workload 的完整证据页（`tui/result_detail.go`），从 Results flat 视图 `d` 进入：

- 名称 + category 状态 pill
- Metrics 卡（KVGrid：time / throughput / latency / iterations / bytes / ops）
- Error 红卡（结构化错误原文）
- Samples 卡（每次迭代的耗时）
- Raw Output 卡（适配器 Detail 原始输出，逐行截宽保换行，超长输出截断到 40 行）

按键：

- `Esc` / `Enter` / `d`：返回 Results（光标保留）
- `q`：退出

## SuiteConfig 与 SuiteResults

Go SuiteConfig 初始选择 `quick` preset，并实际启用 `hardware,network_info,speed,ip_quality`；speed provider 默认只选 Cloudflare。用户切换 preset 后，section 集合同步更新。

配置页顶部有摘要卡（`tui/suite_summary.go`）：启用 section 数、节点目录规模、计划 workload 数和预计总时长。预计时长优先用历史均值（history 中近 10 条 suite 记录各 section 的 `FinishTime-StartedTime` 均值，`n=k history`），无历史时退回静态粗估（`~rough`，hardware 部分随 iterations 缩放）。摘要数据经 `catalogStatsMsg` / `historyStatsMsg` 异步加载，View 无 IO。

TUI 不维护另一套隐式默认值，而是构造与 CLI/MCP 相同的规范化 Suite 配置。可配置字段覆盖：preset/sections、iterations、timeout、IP version、hardware tools、speed providers（含 `china_isp` / `speedtest_isp` 三网 provider）、iperf hosts、route selection、media sets（默认 `all`，与地区选择互斥：选 `all` 清空地区、选任一地区取消 `all`）、IP quality sources（默认 `builtin`，`securitycheck` 为 opt-in 多选）、catalog source 和 catalog revision。catalog source 支持 `embedded` / `auto` / 显式 path；revision pin 不匹配时停在运行前错误状态。

SuiteConfig 按键（除全局）：

- `↑↓`：字段导航
- `←→`：取值切换
- `spc` / `x`：多选切换
- `1-9`：跳到对应 section 并把 preset 切到 Custom（Advanced 文本字段聚焦时忽略）
- `Enter`：启动 Suite
- `Esc`：返回 Dashboard

Go TUI 在终端低于 40 行时使用紧凑 Suite 布局：Config 只展开当前聚焦字段，Running 与 Results 对每个 section 使用单行状态摘要；在 `80x24` 下页面宽高均受终端边界约束，字段导航、启动和取消仍可操作。更高终端继续显示完整卡片与详细结果。

Go 主线覆盖 `hardware / network_info / route / ping / speed / ip_quality / reachability / mail / media` 九个 section。Network Info 展示虚拟化、公网 IP、ASN/provider 和 NAT 证据；Reachability 展示 website/Telegram 的 protocol/latency/status/error。所有这些状态都不会折算为 benchmark 总分。Suite 只有所有 enabled section 都是 `ok` 时成功；enabled 的空状态、`skipped`、`partial`、`error` 均表示失败，disabled section 才只发 skip event。Running 页保留 `PARTIAL` 独立样式但将它计入终态非成功数量，不伪装成 `ERROR` 或成功；网络 section timeout/cancel 也会显示为结构化 `error` message。

Route/Ping 的结构化结果会区分 catalog protocol 与实际 `probe_protocol/probe_tool`。Route 另记录 `resolved_target`、`destination_reached` 和 `status=ok|partial|error`；TUI Route 卡片使用同一有效状态，跑满 hops 但未到目标会显示 `PARTIAL`，不会伪装为成功。Ping 将 TCP RST/refused 视为收到目标响应并以 `status=ok` 展示，报告中的 `connection_state=open|refused|mixed|no_response` 保留端口状态差异，真正无响应才是 loss。Mail 卡片消费顺序探测产生的 `open|refused|timeout|error`，只有 `open` 显示为可达。IP Quality fail-closed 且没有 score 时，结果卡仍展示 section error、RiskSummary 和 Port 25 状态/消息。CLI/history Compare 直接使用报告中的实际协议、工具和 IP family 做兼容门槛。

## ComparePicker 与 Compare

Compare 入口先进入选择器页（`tui/compare_picker.go`）：

- 历史列表：来自 `history` store（最新在前，重验证每条记录，上限 50 条），列为 marker/time/kind/tag/id，异步加载
- `↑↓` 移动，`spc`/`Enter` 标记 A/B（最多两条，第三条顶掉最旧），`c` 发起对比
- `v` 查看单条记录（run -> Results 页、suite -> SuiteResults 页，`Esc` 返回 picker）
- `m` 切换手输路径模式（A/B 两个 textinput，`Tab` 换框、`Esc` 退回列表）——供 `-compare-a/-b` flag 预填或直接粘贴文件路径
- kind 路由：两条 run 记录 -> benchmark delta 表；两条 suite 记录 -> `suitecompare` 的 textgrid 输出（等宽渲染，可滚动）；混合 kind -> toast 拒绝
- flag 直达：`-compare-a` 与 `-compare-b` 同时给出时跳过 picker 直接加载对比

Compare 页展示（benchmark 文档）：

- 系统信息差异
- 每个 workload 的：
  - time delta
  - throughput delta
  - latency delta

方向规则：

- time 越低越好
- latency 越低越好
- throughput 越高越好

CLI/history 的 Suite delta 额外要求 unit、protocol、provider、target/node identity 和所需 catalog revision 一致；Route 还要求显式 `status=ok` 且 `destination_reached=true`。不兼容或缺到达证据时不显示伪 delta。TUI benchmark Compare 仍按 workload 的 time/throughput/latency 规则工作；Suite 对比在 TUI 内以 textgrid 快照呈现，不再要求用户退出到 CLI。

Compare/ComparePicker 按键：`Esc` 返回上一级（Compare -> Picker -> Dashboard），`r`（Compare 页）返回 picker 重选。

## 样式原则

- 绿色：ok / 提升
- 红色：fail / 下降
- 灰色：等待 / 无数据
- 蓝色：主标题 / 当前选择

不使用等级色彩，不使用 S/A/B/C/D/F。

## 工程约束

- View() 纯函数无 IO：所有异步工作走 Cmd -> Msg（sysinfo、history 列表、catalog 统计、compare 加载、单条记录查看），Compare 页不再在渲染路径上读盘。
- 文本输入态（SuiteConfig Advanced、RunConfig Custom filter、Picker 手输）抑制全局单键绑定 `?`/`q`，避免按键被路由进输入框或误退出（`textEntryActive()`）。
- footer 提示与帮助页共用 `helpFor()` 注册表，新增页面需同时补两处 i18n key（en/zh-CN 双语 parity 测试强制）。
- 渲染边界测试：每页在 80x24 的 en 与 zh-CN 下 `assertRenderBounds`（高度不超视口、每行显示宽度不超终端宽）。

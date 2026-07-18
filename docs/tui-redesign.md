# vmbench TUI 重新设计

最近一轮 TUI 重写记录。目标:超越 ECS (`spiritLHLS/ecs`) bash 脚本的视觉,在终端里给 VPS 测试更现代、更清晰、更友好的体验。

## 痛点回顾(改之前)

- 配色硬编码浅色,深色终端反白难读 (`tui/styles.go` 旧版)
- 列宽 / 分隔线写死(`%-23s`、`strings.Repeat("─", 88)`),80 宽折行,200 宽空旷
- 进度条手写 `█/░`,无 spinner、无表格、无卡片、无 toast
- 缺 suite 入口 — `vmbench suite` CLI 已上线但 TUI 仅暴露 `run` 与 `compare`
- 标题就一行蓝字,无 logo、无 sysinfo 卡片、无主题
- 取消/保存反馈弱,save 后无确认,cancel 是裸文本

## 总览

页面新拓扑:

```text
Dashboard ─┬─> RunConfig (隐含, mode/engine 由菜单选项决定) ─> Running ─> Results
           ├─> SuiteConfig ─> SuiteRunning ─> SuiteResults
           ├─> Compare
           └─> System Info (内联展开)
```

新增模块:

| 路径 | 内容 |
|---|---|
| `tui/theme/theme.go` | 8 主题 + `AdaptiveColor` 自动深浅适配 |
| `tui/comp/` | 组件库:card / progress / spinner / statuspill / kvgrid / bar / sparkline / header / footer / tabs / toast / banner / modal / table / layout |
| `tui/suite_config.go` | Suite 配置表单 |
| `tui/suite_running.go` | Suite 跑动页 |
| `tui/suite_results.go` | Suite 结果卡片 |
| `tui/config.go` | `~/.config/vmbench/config.json` 持久化主题 |
| `cmd/vmbench-rendertest/` | 仅在 `-tags rendertest` 启用,渲染快照工具 |

事件流补强:

- `suite/options.go` 加 `EventHandler` + `Event{Kind, Section, Status, Message, Time}`
- `suite/run.go` 每 section 发 `EventSectionStart/Done/Fail/Skip`,跑完发 `EventSuiteDone`
- TUI 的 `tui/suite_running.go` 通过 channel 订阅,刷新卡片 + log viewport

## 主题系统

文件 `tui/theme/theme.go`。

定义 `Theme` 结构包含:

- 基础:`Bg / Surface / Overlay / Fg / Muted / Subtle`
- 动作:`Primary / Secondary / Accent`
- 状态:`Success / Warning / Danger / Info`
- 边框:`Border / BorderFocus`
- 分类:`CPU / Memory / Disk / Network / System`

每个色位是 `lipgloss.AdaptiveColor{Dark, Light}`,渲染时由 lipgloss 按终端背景自动选。

主题列表(`ThemeOrder`):

`dracula`(默认) · `tokyonight` · `catppuccin` · `nord` · `gruvbox` · `rose-pine` · `solarized` · `monochrome`

主题来源:

- Dashboard `[t]` 键循环切换
- `VMBENCH_THEME` 环境变量
- `~/.config/vmbench/config.json` 上次选择
- 默认 `dracula`

Dashboard `[t]` 键循环,退出时持久化。

## 自适应布局

`tui/comp/layout.go` 提供 `BreakpointFor(width)` 与 `AllocateColumns`。

| 断点 | 宽度 | 列数 |
|---|---|---|
| Tiny | <80 | 1 |
| Compact | 80–119 | 1 |
| Normal | 120–159 | 2 |
| Wide | 160–199 | 3 |
| Ultra | ≥200 | 4 |

各页面据此切布局:

- Dashboard:`<120` 上下堆叠 menu + sysinfo card,`≥120` 左右并排
- Running / SuiteRunning:`<120` 卡片单列,`≥120` 两列网格
- Results Cards:卡片网格列数随宽度
- Results Flat:`<100` 隐藏 Category/Latency 列

最低尺寸 60×18,小于则提示扩大终端。

## 组件库

每个组件返回 `string`,用 lipgloss 渲染。Card 与 Modal 用 `lipgloss.RoundedBorder`。

### Card

```go
comp.Card{
    Title: "Hardware",
    Subtitle: "ok",
    Body: "...",
    Footer: "freq 2445 / 2445 MHz",
    Accent: theme.Active.CategorySystem,
    Width: 80,
    Focused: false,
}.Render()
```

顶部彩色 band(`▔▔▔` 主题色)区分分类。

### ProgressLine / ProgressBar

- `ProgressBar(width, ratio, accent)` — `▰▱` 风格
- `ProgressLine(width, ratio, label, accent)` — bar + 百分比 + label

### StatusPill

```text
● done   ◐ running   ○ waiting   ✗ fail   ⊘ skipped
```

颜色绑主题 Success/Warning/Muted/Danger。

### Spinner

包 `bubbles/spinner` 用主题 Accent 上色,默认 `spinner.Dot`。

### Header / Footer

- Header:`[ VMBENCH ]` brand pill + version + CPU 摘要 + 主题徽章 + 时钟
- Footer:hint pill 列表(key + 描述),底部下划线

### Banner

宽度自适应的 ASCII art logo("VMBENCH" block letters),`<40` 改 mini 版。

### Tabs / Toast / Modal / Table / Sparkline / KVGrid / Bar

参见 `tui/comp/*.go`,接口都遵循 `Render() string` 或 `func RenderXxx(args) string`。

## 页面规格

### Dashboard

```text
┌─ VMBENCH v0.x ──── CPU info ──── ⬢ dracula ─ 14:23:05 ─┐
│                                                         │
│  Banner ASCII logo                                      │
│                                                         │
│  Menu                  ╭─ System ─────────╮             │
│  ▎ Run Benchmark...    │ CPU  Xeon ...    │             │
│    Run Suite...        │ Mem  62 GB       │             │
│    Compare Reports     │ OS   Debian 12   │             │
│    System Info         │ Kernel ...       │             │
│    Theme: dracula      │ freq 2445 MHz    │             │
│    Quit                ╰──────────────────╯             │
│                                                         │
└── ↑↓ nav  ↵ select  t theme  q quit ───────────────────┘
```

### SuiteConfig

四组卡片 + Start 按钮:

- Preset 单选(Custom / Quick / Website / Proxy / Mail)
- Sections 多选(7 项 checkbox)
- Speed Providers 多选(cloudflare / speedtest_net / speedtest_cn / iperf3)
- China Route Presets 多选(GZ / BJ / SH)

按键:`↑↓` 切字段、`←→` 切选项、`spc` 切勾、`↵` start、`esc` 返回。

### Running / SuiteRunning

```text
◈ Running: Single-Core (EXTERNAL engine)    elapsed 02:14
Overall ▰▰▰▰▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱  62%  14/22  ✗0

╭─ CPU ──────────────────╮  ╭─ Disk ─────────────────╮
│ sysbench 1T  ● 532 eps │  │ fio seq      ⣾ running │
│ OpenSSL      ● 1.4 GB/s│  │ fio rand     ○ waiting │
╰────────────────────────╯  ╰────────────────────────╯
```

`tab` 切 event log,`esc` 弹 modal 确认取消。

### Results

三 tab:

- Cards:按 category 卡片网格,展示主指标
- Grouped:`▾`/`▸` 折叠展开
- Flat:表格(Workload / Category / Time / Throughput / Latency / Status)

底部 toast 提示 save 成功/失败,`s` 触发,自动 3s 消失。

### SuiteResults

每 section 一个卡片,定制内容:

| Section | 卡片内容 |
|---|---|
| Hardware | workload 列表 + 主指标 |
| Speed | 每 provider:`↓ Mbps  ↑ Mbps  rtt ms` |
| Ping | 每节点:`avg ms  loss%`,失败展示 status |
| Route | 每目标:hops + 状态图标 |
| IP Quality | IP / Country / ASN + 评分进度条 + 等级 |
| Mail Ports | 每端口:open ✓ / blocked ✗ + status 文本 |
| Media | 每服务:✓/✗ + region |

### Compare

两栏 sysinfo 卡片(A / B)+ 下方 delta 表格,每 workload 三行(time / throughput / latency),`▲` 绿色提升、`▼` 红色下降、`=` 持平。

## 配置持久化

`tui/config.go`:

```json
 {
  "theme": "tokyonight",
  "last_mode": "single",
  "last_engine": "external"
}
```

读 `os.UserConfigDir()/vmbench/config.json`,可由 `VMBENCH_CONFIG` 覆盖路径。

## 渲染快照工具

`cmd/vmbench-rendertest/`,build tag `rendertest`:

```bash
go build -tags rendertest -o /tmp/vmrtest ./cmd/vmbench-rendertest/

VMBENCH_THEME=dracula /tmp/vmrtest --page dashboard --width 140
VMBENCH_THEME=nord /tmp/vmrtest --page running --width 100
/tmp/vmrtest --page results --report report.json --width 140
VMBENCH_THEME=tokyonight /tmp/vmrtest --page suite-running --width 140
/tmp/vmrtest --page suite-results --suite-report suite.json --width 140
/tmp/vmrtest --page compare --compare-a a.json --compare-b b.json
```

输出含 ANSI,直接终端管道查看,或重定向 `> snapshot.txt` 留档。

不打入正式 binary(`-tags rendertest` 下才编译,`tui/render_test_helper.go` 同样)。

## 键位汇总

| 页面 | 键 | 作用 |
|---|---|---|
| Dashboard | ↑↓ / ↵ | 导航 / 选 |
| Dashboard | t | 主题循环 |
| Dashboard | q | 退出 |
| Running | tab | 切 event log |
| Running | esc | 弹 modal 取消 |
| Results | tab | Cards / Grouped / Flat 切换 |
| Results | ↑↓ / ↵ | 移动 / 展开 |
| Results | s | 保存 JSON(toast 反馈)|
| Results | esc | 返回 Dashboard |
| SuiteConfig | ↑↓ | 切字段 |
| SuiteConfig | ←→ | 切选项 |
| SuiteConfig | spc / x | 切勾 |
| SuiteConfig | ↵ | start(于 Start 字段或 sections 已有效)|
| SuiteRunning | tab / esc / q | 同 Running |
| Compare / SuiteResults | esc / q | 返回 / 退出 |

## 后续(P3 未做)

- Markdown 报告导出 + clipboard share-text
- Dashboard 实时 sparkline(CPU/mem 曲线,需 ticker + gopsutil 采集)
- glamour 渲染内嵌 help 页面
- 键位用户定制(读 `~/.config/vmbench/keybindings.json`)
- TUI 渲染快照单测(将快照固定为字符串对比)

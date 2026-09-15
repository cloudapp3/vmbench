# vmbench

**新 VPS 到手，一条命令验明正身。**

[![CI](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml/badge.svg)](https://github.com/cloudapp3/vmbench/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cloudapp3/vmbench.svg)](https://pkg.go.dev/github.com/cloudapp3/vmbench)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

vmbench 是给买 VPS、比 VPS、晒 VPS 的人准备的一键体检工具，回答你对一台新机器真正关心的问题：

- **硬件缩水了吗？** — 用 sysbench / fio / OpenSSL 跑 CPU / 内存 / 磁盘（只调度外部工具；Geekbench、dd、STREAM 可选启用）
- **「优质线路」是真的吗？** — 三网 / CERNET / CSTNET 回程 traceroute，自动判定线路类型：163 / 9929 / 4837 / CN2 / CMIN2 / CMI
- **速度到底有多快？** — Cloudflare、Ookla、speedtest.cn、三网分运营商下载，或用 iperf3 打自己的服务器
- **能解锁什么？** — 200+ 流媒体与 AI 平台（Netflix、Disney+、ChatGPT…），默认 globe 集、全量或按地区
- **IP 干不干净？** — 信誉、DNSBL、邮件端口：能不能发信，是不是已经被烧过
- **发论坛会泄露 IP 吗？** — 不会：本机公网 IPv4/IPv6 在所有出口默认脱敏，不亲自执行 `vmbench share` 就绝不上传
- **会把机器搞乱吗？** — 不会：静态 fio/sysbench 装进用户缓存，不碰系统软件包，`vmbench uninstall` 一键全清

TUI、CLI、MCP 三个入口共用同一套规范化配置。vmbench 只输出原始指标和结构化失败信息，绝不编造总分；可选的 `vmbench score` 命令基于版本化基线做确定性评估（维度指数、场景匹配度、覆盖度披露），原始测量永远是唯一事实来源。输出：console / JSON / HTML / 论坛直贴 markdown，界面与报告支持简体中文和英文，跑在 Linux、macOS、Windows 上。

English README: [docs/README.en.md](docs/README.en.md) · 文档：[能力全览](docs/capabilities.md) · [产品说明](docs/product.md) · [技术栈](docs/tech-stack.md) · [变更记录](docs/CHANGELOG.md)

[快速开始](#快速开始) · [安装](#安装) · [命令](#命令) · [常用参数](#常用参数) · [VPS 体检](#vps-体检) · [TUI](#tui) · [文档](#文档)

## 快速开始

```bash
# 一键安装（Linux / macOS，从 GitHub Releases 下载并校验 SHA-256），
# 并把安装目录加入当前 shell 的 PATH
VMBENCH_BIN_DIR="$(
  curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --print-install-dir
)" && export PATH="$VMBENCH_BIN_DIR:$PATH"

vmbench                          # 交互式 TUI：勾选要测的项目，回车开跑
vmbench --json report.json       # 纯硬件跑分（默认行为）
vmbench --preset quick           # 快速概览：硬件 + 网络信息 + 测速 + IP 质量
vmbench --preset proxy           # 代理 / 解锁场景：线路、延迟、速度、IP 质量、解锁、可达性
vmbench --markdown report.md     # 论坛直贴报告 — 公网 IP 已脱敏
vmbench score report.json        # 确定性评估（维度指数、场景匹配、覆盖度）
vmbench share report.json --dry-run  # 先预览脱敏后的 paste，去掉 --dry-run 再上传
vmbench update                   # 从 GitHub Releases 自更新
```

## 安装

上面的一键脚本安装**最新 release** 并按 `checksums.txt` 校验 SHA-256。不带 `--dir` 时自动选择安装目录：已安装在 `~/.local/bin`、`~/bin` 或 `/usr/local/bin` 的直接复用；root 安装用 `/usr/local/bin`；普通用户优先选已在 `PATH` 上的 `~/.local/bin` 或 `~/bin`。自动装进主目录且目录还不在 `PATH` 时，安装脚本会往当前 shell 的启动文件（`.zshrc`、`.bashrc` 或 `.profile`）追加一条幂等条目并打印精确的 reload 命令；其他 shell 只给警告。

```bash
# 系统级安装（只在目标检查和写入时使用 sudo）
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --system

# Go 工具链
go install github.com/cloudapp3/vmbench/cmd/vmbench@latest
```

其他安装参数：`--version vX.Y.Z` 固定版本、`--no-modify-path` 不动 shell 启动文件、`--print-install-dir` 供脚本读取安装目录、`--skip-verify` 跳过校验。`--dir PATH`（等价环境变量 `VMBENCH_INSTALL_DIR`）自定义目录；显式目录绝不修改启动文件，请自行确保目录在 `PATH` 上。`GITHUB_TOKEN`/`GH_TOKEN` 可缓解 API 限流或访问私有 release。

### 卸载

```bash
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash -s -- --uninstall
```

移除二进制、平台数据目录（Linux `~/.local/share/vmbench`，macOS `~/Library/Application Support/vmbench`，含全部本地测评历史）、TUI 偏好目录（Linux `~/.config/vmbench`；macOS 位于数据目录内）、手动创建的 `vmbench` systemd/launchd unit，以及安装脚本写入的 `# vmbench user install` PATH 条目。手写 PATH 行与自定义 `VMBENCH_HISTORY_DIR` 下的报告保留。root 系统级安装用 `sudo bash -s -- --uninstall`。

已安装的二进制也能自卸载：`vmbench uninstall` 先打印删除计划（历史报告条数、已获取的静态工具、TUI 偏好、数据目录、二进制本体），终端确认后按「目录 → 二进制」顺序删除；任一项失败即保留二进制，中断的卸载直接重跑即可。shell 启动文件与服务清理交给 `install.sh --uninstall`——它检测到该子命令会委托二进制卸载，之后停止手动创建的服务并清理 PATH 条目。参数：`--dry-run` 预览计划、`--yes` 跳过确认、`--json` 输出结构化计划/结果。

Windows：从 [Releases](https://github.com/cloudapp3/vmbench/releases) 下载 `vmbench-<version>-windows-<arch>.zip`（WinSAT 提供默认硬件探测）。

### 自更新

```bash
vmbench update                   # 检查并原地替换当前二进制
vmbench update --check           # 只报告最新版本，不安装
vmbench update --version v0.6.0  # 固定 / 降级到指定版本
vmbench update --check --json    # 供脚本使用的结构化状态
```

下载按 release `checksums.txt` 校验 SHA-256，再以临时文件 + rename 原子替换正在运行的可执行文件。`GITHUB_TOKEN`/`GH_TOKEN` 用于 API 限流；`deb`/`rpm` 安装请走包管理器。

## 命令

| 命令 | 说明 |
|------|------|
| `vmbench` | 交互式 TUI（无参数时）——带参数时直接执行测评 |
| `vmbench [flags]` | 执行测评：默认只跑硬件，preset / only / skip 选择体检 section |
| `vmbench nodes <command>` | 列出 / 校验 / 更新 / 健康检查版本化节点目录 |
| `vmbench mcp serve [--transport stdio]` | 以 MCP stdio 把 vmbench 工具暴露给 LLM 客户端 |
| `vmbench list` | 列出可用 workload |
| `vmbench sysinfo [--json]` | 查看系统信息 |
| `vmbench score <report.json\|->` | 基于版本化评分基线，从报告派生确定性评估 |
| `vmbench share <report.json\|->` | 把已保存报告投影成脱敏 paste 并上传到显式选择的 provider |
| `vmbench history <command>` | 增加 / 列出 / 查看 / 删除本地报告 |
| `vmbench update [--check] [--version TAG]` | 从 GitHub Releases 自更新（SHA-256 校验） |
| `vmbench version` | 查看版本 |

## 常用参数

| 参数 | 默认值 | 说明 |
|------|---------|------|
| `--iterations` | 3 | 每个硬件 workload 的迭代次数（1-9） |
| `--filter` | all | 正则选择 workload |
| `--hardware-tool` | 平台默认 | sysbench、openssl、fio、dd、stream、mbw、geekbench、winsat 或 all |
| `--preset` | — | 场景预设：`quick`、`website`、`proxy`、`mail` |
| `--only` / `--skip` | — | 选择或跳过 section |
| `--ip-version` | v4 | `v4`、`v6` 或 `dual` |
| `--speed-provider` | cloudflare | cloudflare、speedtest_net、speedtest_cn、china_isp、speedtest_isp、iperf3 |
| `--ip-quality-source` | builtin | builtin；可选 `securitycheck`（外部 18 库二进制） |
| `--redact` | ips | 报告中掩码本机公网 IPv4/IPv6（`none` 保留真实地址） |
| `--media-set` | `globe` | `globe` 覆盖 41 个国际平台（含 AI 服务）；`all` 跑全部，或组合地区 ID |
| `--node-catalog` | embedded | `embedded`、`auto`（每次拉取，静默回退缓存/embedded）或 JSON 路径 |
| `--node-revision` | — | 固定精确 catalog revision；不匹配时在探测前失败 |
| `--iperf-host` | — | iperf3 speed provider 的服务器 |
| `--json` / `--html` / `--markdown` | — | 导出 JSON / HTML / 论坛直贴 markdown 报告到文件 |
| `--quiet` | false | 关闭进度输出 |
| `--auto-swap` | false | （Linux）geekbench 低内存运行：免确认创建临时 swapfile，结束后移除 |
| `--save-history` | false | 报告存入本地历史（`--history-tag` 打标签） |
| `--lang` | auto | `en` 或 `zh-CN`（也支持 `VMBENCH_LANG`） |

不带 `--preset` / `--only` / `--skip` 时只跑硬件；网络 section 通过 preset 或显式选择启用。生效选择恰好只有 `hardware` section 时产出 benchmark（run）报告——等价 v0.8.0 之前的 `vmbench run`——否则产出综合体检报告。完整参数表：`vmbench --help` 或[能力全览](docs/capabilities.md)。

报告默认脱敏：本机公网 IPv4/IPv6 在所有出口（console、JSON、HTML、markdown、历史、TUI、MCP）被一致替换为文档保留段占位地址（`203.0.113.x`、`2001:db8::x`），内嵌该地址的 BGP/CIDR 网段与反向 DNSBL 标签一并替换；内网 IP、hostname、路由 hop、远端节点 IP 保留。`--redact none` 退出脱敏（CLI 会打印分享警告）；TUI 与 MCP 恒为脱敏。

`--markdown report.md` 导出论坛直贴版式：每个 section 一个二级标题 + 围栏代码块内的 textgrid 等宽表格（任何渲染器下 CJK 不乱版式），流媒体折叠为每区域计数 + 仅异常项，头部带溯源行（版本、UTC 时间、catalog revision）。与 JSON/HTML 同源同脱敏，不嵌评分。

## 分享（share）

`vmbench share <report.json|->` 把保存的 run/体检报告投影成可直贴 payload——只有执行该命令才会上传，返回链接：

```bash
vmbench share report.json --dry-run           # 打印 payload 与脱敏清单，零网络
vmbench share report.json                     # 上传 dpaste（默认），打印 URL
vmbench share report.json --provider dpaste,0x0   # 显式 fallback 链
vmbench share report.json --format json       # 改传脱敏后的报告 JSON
vmbench share report.json --save-payload out.md    # 保存一份与上传字节完全一致的副本
```

payload 复用 `--markdown` 投影（`--format json` 则是脱敏报告本体）并追加一行说明实际脱敏内容的尾注。分享是显式授权：vmbench 的其他部分——基准、TUI、MCP——都不上传任何东西。脱敏在报告层 IP 掩码之上额外掩掉结构化 hostname。`--redact none` 保留真实地址并打印警告；`--media full` 列出全部流媒体服务而不是折叠视图。provider：`dpaste`（默认）、`0x0`、`paste_rs`、`custom`（需 `--share-endpoint`，仅 https）。fallback 仅在网络错误或 5xx 时尝试下一家——同一 payload 最多成功上传一次。超过 512 KiB 的 payload 直接拒绝并给出收敛建议，不做静默截断。

## VPS 体检

体检保留 YABS 式一键体验，section 按 ECS 式模块化组织：

| Section | 用途 |
|---------|------|
| `hardware` | CPU / 内存 / 磁盘 benchmark 报告 |
| `network_info` | 虚拟化、公网 IP、ASN/运营商、NAT 证据、BGP/RDAP 归属视图 |
| `route` | 三网 / CERNET / CSTNET 路由诊断，回程线路判定（163 / 9929 / 4837 / CN2 / CMIN2 / CMI） |
| `ping` | 三网 TCP 延迟 / jitter / 丢包，含连接状态 |
| `speed` | Cloudflare / speedtest 下载 + 上传；可选三网 provider |
| `ip_quality` | IP 信誉：ip-api.com、ipapi.is、DNSBL、邮件端口；可选 securityCheck |
| `reachability` | 网站 HTTPS 与 Telegram DC TCP 可达性 |
| `mail` | 顺序探测邮件端口（open / refused / timeout / error） |
| `media` | 流媒体 / AI 平台解锁探测（200+ 服务，UnlockTests） |

预设：`quick`（hardware、network_info、speed、ip_quality）· `website`（+ route、ping、reachability、mail）· `proxy`（网络优先 + media）· `mail`。

```bash
vmbench --preset proxy --ip-version dual
vmbench --only ping,mail
vmbench --route-presets gz,bj,sh,cd,cernet,cstnet
vmbench --speed-provider china_isp
vmbench --media-set jp,kr
vmbench --only hardware --hardware-tool geekbench
vmbench --node-catalog auto --save-history --history-tag weekly
```

体检只有在每个启用 section 都 `status=ok` 时才算成功；启用的空 / `skipped` / `partial` / `error` 状态都会让整轮失败。报告记录最终解析的节点目录来源 / revision 与所选节点 ID。

## TUI

直接运行 `vmbench`（无参数）：

- **Dashboard**：测评 / 历史 / 系统信息入口，全程支持鼠标点击与滚轮。System 卡片显示本机公网 IPv4/IPv6 与 ASN 归属（启动异步探测，离线静默；展开系统信息另见国家与运营商）
- **Config**：一页搞定与 CLI/MCP 相同的规范化字段——preset 以 Hardware Only（CLI 默认）打头，外加 Custom 与各体检预设；section 开关按需展开工具、filter、speed、route、media、IP 来源卡片，实时显示预估时长、缺失工具预检，`1-9` 数字键跳转 section
- **Running**：两种形态共用一个进度页——硬件跑分显示 workload 网格，体检显示 section 网格，带取消确认框
- **Results**：卡片 / 分组 / 平铺三视图；`d` 进单 workload 详情（指标、采样、错误、原始工具输出）
- **History**：浏览 `--save-history` 保存的报告（时间、类型、标签、ID）；`Enter` 在结果页或体检页重新打开，`Esc` 逐级返回
- **Themes**：Dashboard 按 `t` 循环切换主题，选择本地持久化

按键：`?` 帮助 · `↑↓` 导航 · `Enter` 选择 · `Tab` 切换视图 · `d` 详情 · `s` 保存 · `Esc` 返回 · `q` 或 `Ctrl+C` 退出。所有页面支持 `PgUp/PgDn`、`Home/End` 和鼠标滚轮滚动；80x24 终端可完整操作。

## 界面语言

CLI、TUI 与 console/HTML 报告标签支持简体中文和英文——用 `--lang`、`VMBENCH_LANG` 或 TUI 配置选择。JSON 字段名、状态枚举、section ID 与 workload 名称在任何语言下保持英文。

## 平台支持

| 能力 | Linux | macOS | Windows |
|------|:-----:|:-----:|:-------:|
| CLI / TUI / JSON / HTML | ✅ | ✅ | ✅ |
| 默认硬件工具 | ✅ sysbench / fio / openssl | ⚠️ openssl（其余装包解决） | ⚠️ WinSAT |
| 体检网络诊断 | ✅ | ✅ | ⚠️ 部分 / 依赖环境 |
| MCP stdio server | ✅ | ✅ | ✅ |

Linux 主机缺 `fio` 或 `sysbench` 时，不动系统软件包也能补齐：

```sh
vmbench tools fetch fio sysbench   # pin 版静态构建，SHA-256 校验，装到 ~/.cache/vmbench/binaries
```

下载来自本仓库 `tools` release 资产，按编译进 vmbench 的哈希校验；`--url` 可指定镜像。

网络 section 依赖本机路由、DNS、防火墙、IPv6 与沙箱权限；失败以结构化错误记录，不会被藏起来。

## 文档

| 主题 | 位置 |
|------|------|
| 能力全览：CLI 参数、体检、节点目录、MCP、报告格式、Go API | [docs/capabilities.md](docs/capabilities.md) |
| 英文 README | [docs/README.en.md](docs/README.en.md) |
| 中文快速说明 | [docs/README.zh-CN.md](docs/README.zh-CN.md) |
| MCP 工具、客户端配置、安全默认 | [MCP 大模型接入](docs/capabilities.md#9-mcp-大模型接入) |
| 节点目录信任模型与命令 | [版本化 Node Catalog](docs/capabilities.md#版本化-node-catalog) |
| 硬件工具矩阵与发现 | [硬件测评能力](docs/capabilities.md#4-硬件测评能力) |
| 技术架构 / 当前状态 | [docs/tech-stack.md](docs/tech-stack.md) · [docs/current-state.md](docs/current-state.md) |
| 贡献 / 源码构建 | [CONTRIBUTING.md](CONTRIBUTING.md) |
| 变更记录 | [docs/CHANGELOG.md](docs/CHANGELOG.md) |
| 外部文档站源码 | [cloudapp3/vmdocs](https://github.com/cloudapp3/vmdocs/tree/main/sites/vmbench/docs) |

## 社区与支持

- 🐛 发现 bug 或想要新功能？[提 issue](https://github.com/cloudapp3/vmbench/issues/new)
- 🤝 想贡献？从 [CONTRIBUTING.md](CONTRIBUTING.md) 开始
- 💬 社区聊天：[VMPulse Telegram](https://t.me/VMPulse)

## 许可证

[MIT](LICENSE)

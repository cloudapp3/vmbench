# vmbench 对标 ECS（融合怪）分析报告

> 生成日期：2026-09-05。数据来源：spiritLHLS/ecs 与 oneclickvirt/ecs 的 GitHub README/API、本仓库 README 与 docs/。社区数据为当日快照。

## 1. 结论摘要

1. **功能覆盖度已基本对齐**：vmbench 的 Suite 已覆盖 ECS 约 85% 的测试面（网络身份/NAT、三网回程路由、ping、测速、IP 质量、邮件端口、流媒体解锁、网站/Telegram 可达性），且在 BGP/RDAP 归属、磁盘 IO 细分、测速 provider 数量上超过 ECS。
2. **差距不在"测什么"，而在"最后一公里"**：ECS 的杀手锏是"一键安装 → 跑完 → 自动生成 pastebin 分享链接 → 直接贴论坛"的零摩擦闭环；vmbench 的报告停留在本地文件，没有分享环节，这是对 VPS 玩家群体最大的缺失。
3. **两者的真实分叉是产品定位**：ECS 是"测评展示工具"（终点是一段可分享的文本），vmbench 是"测量基础设施"（终点是结构化证据 + 可对比数据 + 可被程序/AI 调用）。ECS 生态没有 JSON、对比、历史、MCP；vmbench 没有分享和中文社区渗透。
4. **同源生态**：vmbench 的四个网络依赖（UnlockTests、backtrace、gostun、basics）正是 ECS Go 版同一作者（oneclickvirt 生态）的 Apache-2.0 库。网络探测能力同源，差异在编排、语义严格度和输出模型，不存在探测维度上的代差。
5. **最年轻的短板是社区**：ECS Shell 版 7.2k star（2022 年起），Go 版 2.3k star；vmbench 2026-06 创建，尚在冷启动期。

## 2. 对标对象概览

| 维度 | ECS Shell 版<br>(spiritLHLS/ecs) | ECS Go 版<br>(oneclickvirt/ecs, goecs) | vmbench<br>(cloudapp3/vmbench) |
|---|---|---|---|
| 定位 | VPS 融合怪测评脚本（社区事实标准） | "最全能的服务器测评项目"（官方推荐继任者） | 跨平台 VPS 测评：原始指标 + 结构化报告 |
| 形态 | Bash 脚本，运行时安装依赖 | 预编译单二进制，零环境依赖 | 预编译单二进制 + 可选外部基准工具 |
| 语言/许可证 | Bash / MIT | Go / **GPL-3.0** | Go / MIT |
| Stars / 创建 | ~7,183★ / 2022-06（维护态） | ~2,334★ / 2024-06 | 0★ / 2026-06（极早期） |
| 平台 | Linux 为主（FreeBSD 半支持） | Linux/Windows/macOS/FreeBSD/Android；12 种架构（含 386/arm/mips/s390x/riscv64） | Linux/macOS/Windows × amd64/arm64 |
| 输出 | 彩色控制台 + `test_result.txt` + **pastebin 自动分享链接** | 控制台（中/英菜单）+ `goecs.txt` + **h501.io 自动分享链接** | Console / JSON (schema v2) / HTML + 本地 history + compare |
| 运行侵入性 | 会更新包管理器并安装依赖，作者明示"勿在生产环境使用" | 不装依赖、不改 resolver；支持非 root 与离线 | 不装依赖、不改系统；缺外部工具时 fail-closed 输出结构化错误 |
| 机器可读性 | 无 | 无（纯文本日志） | JSON schema v2、MCP tools、exit code 语义 |
| 自动化/集成 | 弱（菜单为主，flags 为辅） | 中（flags 较全） | 强（CLI/TUI/MCP 三入口，共享同一校验契约） |

一个值得注意的事实：vmbench 与 ECS Go 版共用同一批上游探测库（同源），因此本报告的功能对比聚焦在**编排能力、输出模型和用户路径**上。

## 3. 功能点对比矩阵

### 3.1 测试模块

| 模块 | ECS Shell | ECS Go | vmbench | 差距评注 |
|---|---|---|---|---|
| 系统信息 | 融合 bench.sh/superbench/yabs/lemonbench + sysctl 检测 + 并发 ASN | basics/gostun 自研库 | gopsutil + 自研 /proc+/sys 深度采集（uptime/load/swap/balloon/KSM/TCP 调优/嵌套虚拟化/HugePages） | **vmbench 更深**；平台深度信息是 ECS 没有的 |
| CPU | sysbench 默认；geekbench 4/5/6 可选（`-ctype`） | sysbench/geekbench/winsat，single/multi 可选 | sysbench single+multi、openssl speed；geekbench 可选；winsat（Windows） | ECS 可指定 gb4/5/6 版本；vmbench 的 geekbench 不能选版本 |
| 内存 | lemonbench 源 | stream/sysbench/dd/winsat/auto | sysbench 拆分为读带宽/写带宽/随机读延迟；STREAM/mbw 可选；winsat | 基本对齐，vmbench 输出拆分更利于定位瓶颈 |
| 磁盘 IO | dd（快/误差大）+ fio（慢/真实）双模式；`-mdisk` 多挂载盘；`-diskp` 路径 | fio/dd/winsat；多盘与路径可选 | fio 八项细分（4K 随机 Q1/Q32 + 1M 顺序 Q1/Q8 读写）+ dd 可选（Linux direct I/O）+ winsat；`--disk-path` 单路径 | vmbench fio 细分更专业；**缺多挂载盘遍历** |
| 网络身份 | 基础信息内含 ASN/NAT | basics + gostun（NAT 类型） | 公网 v4/v6、ASN/provider/location、保守 NAT 证据、STUN 分类（Full Cone/Restricted/Symmetric、hairpin、端口保持）、**BGP/RDAP 归属**（注册网段/RIR/注册日期/上游/对等/IXP/Tier1）、IPv6 on-link 前缀长度、bgp.tools 邻居估计 | **vmbench 明显更深**；BGP/RDAP 与对等关系是 ECS 全系没有的 |
| 回程路由 | backtrace（京/广/沪/成，IPv6，自定义目标） | backtrace + **nt3**（改自 NTrace-core，逐跳明细） | route section：版本化节点（三网+成都+CERNET+CSTNET，v4/v6），逐跳运营商 ASN 标注，回程线路分类（163/9929/4837/CN2GIA/CN2GT/CTGNET/CMIN2/CMI），`resolved_target`/`destination_reached`/`ok\|partial\|error` 证据 | 结论能力对齐；探测手段上 ECS 的 nt3 走 NTrace 多协议（TCP/ICMP）能穿更多被拦截的 hop，vmbench 依赖系统 traceroute（无需 root 是优点也是上限） |
| 三网 Ping | pingtest | pingtest | ping section：TCP latency/jitter/loss + `connection_state`，RST 不计丢包，CERNET/CSTNET/IPv6 节点 | 对齐，vmbench 语义更严格 |
| 测速 | ecsspeed 三网，.cn/.net 可选 | 自研 speedtest，节点 ID 自动更新 | 六个 provider：cloudflare / speedtest_net / speedtest_cn / iperf3 / china_isp / speedtest_isp，多 provider 时 `best_per_metric` 聚合标注 | **vmbench provider 更全**；但 ECS 的节点自动更新 vs vmbench 的版本化 catalog 是两种哲学（见 3.2） |
| IP 质量 | 原创 15 家数据库 + DNSBL + ASN + 邮件端口（securityCheck） | securityCheck 并发 | ip-api + ipapi.is 归属交叉 + DNSBL + 邮件端口 + opt-in securityCheck（18 库）；0-100 风险分 fail-closed（证据不全不出分） | 对齐，vmbench 的 fail-closed 出分条件更保守 |
| 邮件端口 | portchecker（25 端口可建邮局） | portchecker | mail section：8 端口顺序探测，`open/refused/timeout/error` 分类 | 对齐 |
| 流媒体解锁 | UnlockTests 二进制 + shell 双版本 | UnlockTests 并发，区域可选（0–22） | UnlockTests 200+ 服务，13 个区域子集（`--media-set`） | 同源同能力 |
| 热门网站/Telegram | `-web`/`-tgdc` 开关 | 有 | reachability section：Google/GitHub/Cloudflare HTTPS + Telegram DC TCP，带 latency/status | 对齐 |
| 综合评分 | 无总分（展示原始数据） | 无总分（`-analysis` 为汇总展示） | **明文原则：不输出总分/等级** | 一致；此维度不是差距 |
| 交互界面 | 交互式菜单（三层 `-m` 选择） | 菜单 + 完整 flags | Bubble Tea TUI（配置/运行/结果/对比，8 主题，80x24 适配） | 各有取向：菜单更低门槛，TUI 更完整 |
| 多语言 | 中文默认，`-en` | 中文默认，`-l en/zh` | CLI/报告为英文；文档有中文 | **vmbench 缺中文输出**，对目标社区是实际门槛 |
| 硬件工具策略 | 运行时自动安装（改系统） | 内置替代实现/自带二进制 | 外部工具 fail-closed，缺失时结构化报错 + Debian/Ubuntu 安装提示 | 设计取舍：vmbench 不伪造数字，代价是首次使用多一步 `apt install sysbench fio` |

### 3.2 输出与工作流

| 维度 | ECS | vmbench | 优势方 |
|---|---|---|---|
| 分享 | 跑完自动上传 pastebin/h501，拿到链接即可贴帖 | 无；HTML/JSON 在本地磁盘（文档明确：未来上传必须显式授权 + 支持脱敏） | **ECS（决定性）** |
| 机器可读输出 | 无 | JSON schema v2（envelope、config、provenance、结构化错误） | **vmbench（独有）** |
| 报告对比 | 无 | `compare` 自动识别 benchmark/Suite，兼容性门控后才算 delta | **vmbench（独有）** |
| 本地历史 | 覆盖式 txt | 原子化 history 存储（0700/0600）+ `history compare --last N` | **vmbench（独有）** |
| 可复现性 | 测速节点 ID 自动更新，隐式漂移 | 版本化节点 catalog：embedded 离线快照 + revision pin + Ed25519 签名更新 + 报告记录 source/revision/node IDs | **vmbench（独有）** |
| 语义严格度 | 文本展示为主 | route 必须到达目标才算 ok、RST 不算丢包、mail 只比较 open 延迟、fail-closed 出分 | **vmbench** |
| 隐私 | IP/路由随分享链接公开扩散 | 报告 0600 权限、不上传、未来分享需显式授权 | **vmbench** |
| AI/LLM 集成 | 无 | MCP stdio server（4 个 tools，枚举校验、单任务互斥） | **vmbench（独有）** |
| 退出码/超时语义 | 弱 | enabled section 非 ok 即非零退出码、section 派生 timeout、cancel 写结构化错误 | **vmbench** |
| 架构覆盖 | Go 版 12 架构 | amd64/arm64 | **ECS Go** |
| 离线/非 root | Go 版支持离线 + DoH/DoT 回退 | embedded catalog 离线确定性；traceroute 走系统命令 | 接近，ECS Go 略强 |

## 4. 产品层面分析

### 4.1 定位分叉

- **ECS = 测评展示工具**。产品终点是一段适合贴到论坛/TG 的文本。它的飞轮是：一键 curl → 中文菜单 → 跑完 → pastebin 链接 → 贴到 hostloc/NodeSeek → 被搜索和转发 → 新用户来自搜到的贴子。安装和分享两个环节都是零摩擦，这是 7k star 的根本原因。
- **vmbench = 测量基础设施**。产品终点是结构化证据：JSON 可入流水线、节点可复现、对比有兼容性门控、AI 可通过 MCP 安全调用。它的目标用户不只是"看一眼这台 VPS 怎么样"的人，还包括要"持续测量、横向对比、批量采集"的人。

### 4.2 双方各自的产品债

**ECS 的工程债**（vmbench 的机会）：

1. Shell 版环境问题多到作者自己重写了 Go 版；运行时 `apt update` 装依赖，官方明示勿用于生产。
2. 分享链接把公网 IP、路由 hop、ASN 隐式公开到第三方 paste 服务，无脱敏、无授权环节。
3. 纯文本输出不可机读，节点自动更新导致两次测试的节点可能不同，结果不可严格复现。
4. GPL-3.0 对想要集成的下游（面板、服务商质检系统）是法律摩擦；vmbench 的 MIT + MCP 对集成方更友好。

**vmbench 的产品债**（ECS 的优势）：

1. **没有分享闭环**：HTML 报告是本地文件，用户要自己截图/传 paste，论坛贴作者没有迁移理由。
2. **首次使用摩擦**：Linux 上 sysbench/fio 不默认存在，虽然 fail-closed 设计正确且会提示安装命令，但对比 ECS Go 的"零依赖直接出数"就是多一步。
3. **英文 CLI/输出**：核心目标社区（中文 VPS 圈）第一眼是英文表格。
4. **冷启动**：0 star、无社区贴子沉淀、无"搜 VPS 测评"能命中的存量内容——ECS 的护城河其实是四年积累的搜索结果页。
5. 架构覆盖窄：老设备/冷门架构（386、arm、mips、riscv）用户无法使用。

### 4.3 差异化赌注

vmbench 手里 ECS 完全没有的三张牌：**可复现证据链**（节点 revision + 到达性证据 + fail-closed 语义）、**compare/history**（买前测 A 家、买后测 B 家，同一节点口径对比）、**MCP**（让 AI 助手直接测 VPS）。这三者都指向同一个趋势：VPS 测评正在从"人看贴子"向"数据被程序和 AI 消费"演进，ECS 生态在这个方向上没有任何布局。

## 5. 用户层面分析

### 5.1 用户画像与场景满足度

| 用户 | 典型场景 | 现在用什么 | vmbench 满足度 | 关键缺口 |
|---|---|---|---|---|
| VPS 玩家 / 测评贴作者 | 新机到手发 hostloc/NodeSeek 贴 | ECS（垄断地位） | ★★☆☆☆ | 分享链接、中文输出、论坛格式；不解决就不会迁移 |
| 普通 VPS 买家 | 买前跑一次验证商家宣传 | ECS / YABS | ★★★☆☆ | 要先 `apt install sysbench fio`；英文界面有阅读成本；但"不污染系统 + 报告不自动上传"是真实加分项 |
| 站长 / 邮局搭建者 | 验证回程线路、25 端口、IP 黑名单 | ecs-ipcheck / 融合怪 | ★★★★☆ | `--preset mail` 精准命中；缺多盘测试（面板机常有多挂载盘） |
| SRE / 服务商质检 | 批量上架前自检、长期跟踪性能劣化 | 自写脚本 / 无工具 | ★★★★★ | JSON + 退出码 + timeout 语义 + history compare 正是为此设计；这是 ECS 完全服务不了的人群 |
| AI agent / 自动化开发者 | 让 Claude/Cursor 自己测 VPS 并解读 | 无可用工具 | ★★★★★ | MCP 独占场景 |
| 低端机 / 冷门架构玩家 | NAS、旧安卓盒子、riscv 开发板 | ECS Go（12 架构） | ☆☆☆☆☆ | 无 386/arm/mips/riscv 构建，直接不可用 |

### 5.2 用户旅程对比（新机测评）

| 旅程节点 | ECS Go | vmbench |
|---|---|---|
| 安装 | 一条 curl（多套 CDN/短链兜底） | install.sh 或 go install（对 Go 用户顺畅，对普通玩家略生） |
| 首次运行 | 直接出全部数据 | Linux 需先装 sysbench/fio，否则硬件项是结构化 error |
| 运行中 | 中文菜单/进度 | stderr 实时 section 进度或 TUI（英文） |
| 看结果 | 终端彩色文本，即看即懂 | Console 表格（英文）/ HTML 本地文件 |
| 分享 | 自动生成链接，复制即走 | 手动上传文件或截图 |
| 二次对比 | 人工肉眼对文本 | `history compare --last 3` 自动算 delta（口径不一致会明确拒绝而不是给错结论） |

结论：**前半程（安装/运行）ECS 略胜，后半程（结果消费）vmbench 全面领先，唯独"分享"这个 ECS 飞轮的发动机 vmbench 是缺失的**。

## 6. 差距清单与机会点（按优先级）

| 优先级 | 事项 | 理由 |
|---|---|---|
| **P1** | 分享闭环：`--share`（显式授权、可选脱敏公网 IP）上传自生成 paste，或生成"单文件自包含 HTML"便于手动分发 | 缩小与 ECS 差距的最大单一杠杆；且能以"授权 + 脱敏"做出比 ECS 更负责任的版本，与现有文档承诺一致 |
| **P1** | 中文输出/中文报告模板（`--lang zh` 或 `--format hostloc` 论坛格式） | 目标社区第一语言；ECS 的中文默认就是它的进入壁垒 |
| **P2** | 多挂载盘测试（`--disk-path` 支持多路径或自动枚举挂载点） | ECS `-mdisk` 已有；面板机/大盘鸡场景常用 |
| **P2** | 补充 386/arm 发布（必要时再加 mips） | Go 交叉编译成本极低，直接扩大可用人群 |
| **P2** | geekbench 版本选择（4/5/6），对齐 ECS `-ctype` | 小改动，补齐与社区口径的可比性 |
| **P3** | nt3 式多协议逐跳探测增强（TCP 指定端口探测，root 下 ICMP） | 提升被拦截网络下的 hop 完整度；注意保持"无 root 也能跑"的现状 |
| **P3** | sysctl/内核网络参数展示增强（已有 TCP 调优基础） | ECS 有 sysctl 检测项，vmbench 已有部分 /proc+/sys 深度采集，补齐展示面 |
| 不做 | 综合评分/排名 | 与产品原则冲突；ECS 也没有，不构成竞争劣势 |

## 7. 结论

vmbench 与 ECS 的功能重合度已经很高，且在 BGP/RDAP 归属、磁盘细分、测速 provider、报告工程化（JSON/compare/history/MCP）上形成了 ECS 没有的一整层能力；ECS 则在**分享闭环、中文社区渗透、架构覆盖、零摩擦安装**上保持决定性优势，这四项里前三项都不难追赶，唯独社区存量需要时间。短期最值得投入的是 P1 两项：一个"显式授权 + 可脱敏"的分享能力，和中文输出/论坛格式模板——这两件事直接决定 vmbench 能否进入"发测评贴"这个 ECS 统治了四年的用户场景。长期看，vmbench 的差异化叙事应该是"ECS 告诉你怎么样，vmbench 让你可以复测、对比、并交给程序和 AI 处理"，不必在对"一键出贴"的模仿上消耗精力。

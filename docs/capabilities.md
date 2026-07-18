# vmbench 项目能力全景文档

> 本文档对 vmbench 项目的全部能力、架构设计、接口规范、配置参数、数据模型进行系统性梳理。
> 最后更新：2026-07-17

---

## 目录

1. [项目定位与设计原则](#1-项目定位与设计原则)
2. [完整能力清单](#2-完整能力清单)
3. [CLI 命令体系](#3-cli-命令体系)
4. [硬件测评能力](#4-硬件测评能力)
5. [网络诊断能力](#5-网络诊断能力)
6. [VPS Suite 综合测评](#6-vps-suite-综合测评)
7. [报告输出体系](#7-报告输出体系)
8. [TUI 交互界面](#8-tui-交互界面)
9. [MCP 大模型接入](#9-mcp-大模型接入)
10. [系统信息采集](#10-系统信息采集)
11. [Go API 编程接口](#11-go-api-编程接口)
12. [核心数据模型](#12-核心数据模型)
13. [事件系统](#13-事件系统)
14. [构建与分发](#14-构建与分发)
15. [跨平台支持矩阵](#15-跨平台支持矩阵)
16. [ECS 对标差异](#16-ecs-对标差异)
17. [完整目录结构](#17-完整目录结构)

---

## 1. 项目定位与设计原则

### 产品定位

vmbench 是一款**跨平台 VPS 基准测试工具**，使用 Go 编写，面向三类使用方式：

- **CLI**：批量执行、管道集成、导出 JSON/HTML/Console 报告
- **TUI**：交互式跑分、结果浏览、两份 benchmark 报告对比、主题切换
- **MCP**：通过 stdio 暴露给大模型客户端（Claude、Cursor、Cline 等）安全调用

### 核心设计原则

| 原则 | 含义 |
|------|------|
| **原始指标优先** | 只输出 median time、throughput、latency、detail/error，不输出综合总分、等级或 category score |
| **外部工具驱动** | 硬件测评只调用外部工具（如 sysbench、fio、OpenSSL、WinSAT），Go 只负责编排和解析，不使用进程内算法 |
| **串行隔离测量** | 不并发不同 workload；线程数/队列深度由外部工具参数定义，网络 workload 只执行一次真实探测 |
| **结构化错误保留** | 缺失工具、网络失败等全部以结构化 error 记录，不伪造结果、不静默跳过 |
| **对比优先于排名** | `compare` 按原始指标对齐；只有 unit/实际 protocol 与 IP family/provider 与 probe tool/node/catalog revision 兼容才做 delta，不产生排名表 |
| **预设只表达场景** | suite preset（quick/website/proxy/mail）只决定跑哪些 section，不改变输出模型 |
| **不强行归一化** | 不同工具、不同维度的指标各自独立，不合并为单一数值 |
| **安全边界** | MCP 不接受任意 shell 命令，参数受限，同一时间只允许一个 benchmark 运行 |

---

## 2. 完整能力清单

### 基准测试

| 能力 | 说明 |
|------|------|
| CPU 单核测试 | sysbench CPU single-core prime |
| CPU 多核测试 | sysbench CPU multi-core prime |
| CPU 加密吞吐 | OpenSSL AES-256-CBC、SHA256 speed |
| CPU 综合评分 | Geekbench 单核/多核（可选） |
| 内存带宽 | sysbench memory bandwidth |
| 内存带宽扩展 | STREAM bandwidth kernels、mbw memory copy（可选） |
| 磁盘顺序读写 | fio sequential read/write |
| 磁盘随机读写 | fio random 4K IOPS |
| 磁盘顺序测试 | dd sequential write/read（可选） |
| Windows 硬件 | WinSAT CPU/memory/disk probes（Windows 默认） |

### 网络诊断

| 能力 | 说明 |
|------|------|
| 网络身份 | 虚拟化、公网 IPv4/IPv6、ASN/provider/location、保守 NAT evidence |
| 版本化节点 | embedded/auto/path、revision pin、Ed25519 signed update/verify/health |
| 路由追踪 | 广州/北京/上海/成都、三网/CERNET/CSTNET、IPv4/IPv6 traceroute |
| 延迟测试 | 同一 catalog 节点上的 TCP ping latency/jitter/loss |
| 下载测速 | Cloudflare 多线程下载 |
| 上传测速 | Cloudflare 上传 |
| Speedtest.net | Ookla Speedtest CLI JSON 解析 |
| Speedtest.cn | speedtest.cn 兼容 CLI JSON 解析 |
| iperf3 测速 | 用户提供的 iperf3 目标服务器 |
| IP 质量 | IP reputation、DNSBL 检测、邮件端口探测 |
| 邮件端口 | 25/465/587/2525/110/143/993/995 可达性 |
| 流媒体解锁 | Netflix/YouTube/Disney+/ChatGPT/TikTok/Prime 等平台 |
| 网站/TG 可达性 | Website HTTPS 与 Telegram DC TCP latency/status/error |

### 报告与输出

| 能力 | 说明 |
|------|------|
| Console 报告 | 终端彩色表格输出 |
| JSON 报告 | 机器可解析的完整结构化数据 |
| HTML 报告 | 带系统信息卡片的可视化报告 |
| 报告对比 | 自动识别 benchmark/Suite；仅对兼容证据计算 delta |
| 本地历史 | add/list/show/delete、`compare --last N`、`--save-history` |
| 系统信息 | CPU/GPU/内存/磁盘/网络/OS/虚拟化全量采集 |

### 接口

| 能力 | 说明 |
|------|------|
| CLI | 标准命令行，支持全部参数和输出格式 |
| TUI | Bubble Tea 交互界面，8 套主题，实时进度 |
| MCP | JSON-RPC over stdio，4 个工具，安全默认值 |
| Go API | `RunCore()` 函数直接调用，可嵌入第三方应用 |

---

## 3. CLI 命令体系

### 命令总览

```
vmbench                                      # 启动 TUI（默认行为）
vmbench run [flags]                          # 运行硬件基准测试
vmbench suite [flags]                        # 运行 VPS 综合测评
vmbench list                                 # 列出可用 workload
vmbench sysinfo [--json]                     # 显示系统信息
vmbench compare <a.json> <b.json> [...]      # 自动识别并对比 benchmark/Suite
vmbench history add|list|show|delete|compare # 本地报告历史
vmbench nodes list|verify|update|health      # 版本化节点目录管理
vmbench mcp serve [--transport stdio]        # 启动 MCP 服务器
vmbench ecs-diff [--json]                    # 输出与 ECS 的产品差异快照
vmbench version                              # 显示版本号
```

### `run` 命令参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--iterations` | `3` | 每个 workload 的迭代次数（1-9） |
| `--filter` | （全部） | 正则表达式过滤 workload |
| `--disk-path` | 系统临时目录 | 磁盘测试使用的临时目录 |
| `--timeout` | `5m` | 单个 workload 超时时间 |
| `--mode` | `single` | 兼容参数；旧的 `multi` / `all` 会 warning 后按 single 只运行一次 catalog |
| `--scope` | `hardware` | `hardware` / `network` / `all`；网络范围需显式开启 |
| `--hardware-tool` | 平台相关 | Linux: sysbench,openssl,fio；macOS: openssl；Windows: winsat |
| `--iperf-host` | （空） | iperf3 服务器地址（逗号分隔多个） |
| `--node-catalog` | `embedded` | network/all 使用 embedded / auto / 显式 JSON path |
| `--node-revision` | （空） | 精确 revision pin |
| `--node-cache` | 用户 cache | `--node-catalog auto` 的高级 cache path override |
| `--json` | （空） | 输出 JSON 报告到文件 |
| `--html` | （空） | 输出 HTML 报告到文件 |
| `--quiet` | `false` | 静默模式，抑制进度输出 |
| `--save-history` | `false` | 原子保存到本地历史（Unix mode `0700/0600`） |
| `--history-tag` | （空） | 可选历史标签，要求 `--save-history` |

### `suite` 命令参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--iterations` | `3` | hardware workload 迭代次数（1-9） |
| `--timeout` | `5m` | 每个网络 section 的 timeout；hardware 按 workload 应用 |
| `--preset` | （全部 section） | 场景预设：`quick` / `website` / `proxy` / `mail` |
| `--only` | （全部） | 只运行指定 section（逗号分隔） |
| `--skip` | （无） | 跳过指定 section |
| `--speed-provider` | `cloudflare` | 速度测试提供商（逗号分隔） |
| `--ip-version` | `v4` | IP 版本：`v4` / `v6` / `dual` |
| `--route-presets` | `gz,bj,sh,cd,cernet,cstnet` | 广州、北京、上海、成都、教育网、科技网 |
| `--node-catalog` | `embedded` | `embedded` / `auto` / 显式 JSON path |
| `--node-revision` | （空） | pin 精确 catalog revision，不匹配时不启动 probe |
| `--node-cache` | 用户 cache | `auto` source 的 cache path override |
| `--hardware-tool` | 平台相关 | 同 run 命令 |
| `--iperf-host` | （空） | 同 run 命令 |
| `--json` | （空） | 同 run 命令 |
| `--html` | （空） | 同 run 命令 |
| `--save-history` | `false` | 保存 Suite v2 JSON 到本地历史 |
| `--history-tag` | （空） | 可选历史标签 |

### `nodes` 与 `history` 子命令

| 命令 | 作用 |
|------|------|
| `nodes list` | 按 kind/IP family/region/city/carrier 过滤并输出节点及 revision/source |
| `nodes verify` | 校验严格 schema/revision；提供 signature + Ed25519 key 时同时验签 |
| `nodes update` | 下载 manifest，验签/校验后原子更新 cache（Unix mode `0600`） |
| `nodes health` | 对筛选节点执行受限并发 HTTP HEAD、DNS 或 TCP 可用性检查 |
| `history add FILE [--tag TAG]` | 导入已有 benchmark/Suite JSON |
| `history list` | 按报告时间列出本地记录 |
| `history show ID` | 输出某条记录中的原始报告 |
| `history delete ID` | 删除指定记录 |
| `history compare --last N` | 比较最近 N 份同 report kind 的记录 |

节点选择公共参数是 `--node-catalog embedded|auto|PATH` 与 `--node-revision REV`；管理命令可用 `--node-cache PATH` 覆盖默认 cache。`embedded` 不访问网络；`auto` 只加载已经验证的 cache，失败时回退 embedded。更新必须显式提供 trust root，不内置可被远程替换的公钥。

### `--hardware-tool` 可选值

| 工具 ID | 说明 | 默认平台 |
|---------|------|---------|
| `sysbench` | CPU 单核/多核 prime、内存带宽 | Linux |
| `openssl` | AES-256-CBC、SHA256 加密吞吐 | Linux / macOS |
| `fio` | 磁盘顺序/随机 4K I/O | Linux |
| `dd` | 磁盘顺序 write/read | ❌ |
| `stream` | STREAM 内存带宽 kernels | ❌ |
| `mbw` | 内存拷贝带宽 | ❌ |
| `geekbench` | Geekbench CPU 单核/多核评分 | ❌ |
| `winsat` | Windows CPU/memory/disk 探测 | Windows |
| `all` | 启用所有可用工具 | ❌ |

### 使用示例

```bash
# 基础硬件测试（默认 3 次迭代，工具集按平台选择）
vmbench run

# 兼容模式：warning 后仍只运行一次标准 catalog
vmbench run --mode all

# 显式运行网络 workload（每项最多一次真实探测）
vmbench run --scope network --iterations 1
vmbench run --scope all --iterations 1

# 只跑 CPU 相关的 workload
vmbench run --filter 'sysbench|OpenSSL'

# 扩展工具集
vmbench run --hardware-tool sysbench,openssl,fio,dd,stream,mbw

# 快速测评（1 次迭代）并输出 JSON + HTML
vmbench run --iterations 1 --json report.json --html report.html

# 快速场景预设
vmbench suite --preset quick

# 建站场景预设，双栈
vmbench suite --preset website --ip-version dual

# 固定节点 revision，保存本次证据
vmbench suite --node-catalog auto --node-revision 2026-07-13.1 --save-history --history-tag weekly

# 只测速度和 IP 质量
vmbench suite --only speed,ip_quality

# 多个速度提供商
vmbench suite --speed-provider cloudflare,speedtest_net

# 使用 iperf3 测速
vmbench suite --speed-provider iperf3 --iperf-host 1.2.3.4

# 对比 benchmark 或 Suite 报告，以及最近 N 份同类型历史
vmbench compare report-a.json report-b.json report-c.json
vmbench history compare --last 3

# 管理、验证和健康检查节点目录
vmbench nodes list --node-catalog embedded --json
vmbench nodes verify --node-catalog /path/to/nodes.json --signature nodes.sig --public-key nodes.pub
vmbench nodes health --node-catalog auto --ip-family v6

# 查看系统信息
vmbench sysinfo
vmbench sysinfo --json
```

`run` 默认只选择 hardware scope。启用 network/all 时 CLI 会提示基础 workload 可能传输约 1.75 GB 数据；非法 regex/mode/scope/iteration/tool 返回退出码 2，没有 workload 命中或任一 workload 失败返回退出码 1。

---

## 4. 硬件测评能力

### 测评架构

vmbench 硬件测评采用**外部工具编排模式**：

```
用户请求 → catalog 注册 workload → 构建 Workload 实例
                                        ↓
                                   Runner 执行
                                        ↓
                          调用外部工具（sysbench/fio/openssl 等）
                                        ↓
                          解析标准输出 → 生成结构化结果
```

Go 代码**不执行**任何进程内的 CPU/内存/磁盘基准算法，只负责：
- 工具发现（PATH → 解析后的 vmbench 可执行文件相邻目录）
- 命令编排（参数构造、超时控制）
- 输出解析（正则提取、数值转换）
- 结果聚合（多迭代取中位数）

Runner 始终串行、隔离执行 workload，不修改全局 `GOMAXPROCS`、GC 或 OS 线程绑定。硬件 workload 使用请求的 1-9 次迭代；`bench/netio` workload 通过 `IterationLimiter` 限制为一次真实探测。

### 支持的硬件工具

#### sysbench（Linux 默认）

| 测试项 | 命令 | 输出指标 |
|--------|------|---------|
| CPU 单核 prime | `sysbench cpu --threads 1` | events/sec |
| CPU 多核 prime | `sysbench cpu --threads N` | events/sec |
| 内存顺序读带宽 | `sysbench memory --memory-oper=read --memory-access-mode=seq` | MiB/s |
| 内存顺序写带宽 | `sysbench memory --memory-oper=write --memory-access-mode=seq` | MiB/s |
| 内存随机读延迟 | `sysbench memory --memory-access-mode=rnd --memory-block-size=64B` | ops/s + ns/op |

#### openssl（Linux/macOS 默认）

| 测试项 | 命令 | 输出指标 |
|--------|------|---------|
| AES-256-CBC 加密 | `openssl speed -elapsed aes-256-cbc` | MiB/s |
| SHA256 哈希 | `openssl speed -elapsed sha256` | MiB/s |

#### fio（Linux 默认）

| 测试项 | 命令 | 输出指标 |
|--------|------|---------|
| 磁盘 4K 随机读 | `fio --rw=randread --bs=4k --iodepth=1/32` | IOPS + ns/op |
| 磁盘 4K 随机写 | `fio --rw=randwrite --bs=4k --iodepth=1/32` | IOPS + ns/op |
| 磁盘 1M 顺序读 | `fio --rw=read --bs=1M --iodepth=1/8` | MiB/s + ns/op |
| 磁盘 1M 顺序写 | `fio --rw=write --bs=1M --iodepth=1/8` | MiB/s + ns/op |

#### dd（可选）

| 测试项 | 输出指标 |
|--------|---------|
| 磁盘顺序写 | MB/s |
| 磁盘顺序读 | MB/s |

#### STREAM（可选）

| 测试项 | 输出指标 |
|--------|---------|
| Triad 带宽 | MB/s |
| Copy 带宽 | MB/s |
| Scale 带宽 | MB/s |
| Add 带宽 | MB/s |

#### mbw（可选）

| 测试项 | 输出指标 |
|--------|---------|
| 内存拷贝带宽 | MiB/s |

#### Geekbench（可选）

| 测试项 | 输出指标 |
|--------|---------|
| CPU 单核评分 | 分数（上游评分） |
| CPU 多核评分 | 分数（上游评分） |

> 注意：Geekbench 评分是上游产品的评分体系，vmbench 只是透传，不会将其纳入 vmbench 自身的评分体系。

#### WinSAT（Windows 默认）

| 测试项 | 输出指标 |
|--------|---------|
| CPU 评估 | MB/s |
| 内存评估 | MB/s |
| 磁盘评估 | MB/s |

### 工具发现策略

```
1. 检查系统 PATH → 找到则使用
2. Linux 下检查解析后的 vmbench 可执行文件相邻位置：
   `<exe-dir>/binaries/<tool>_<arch>`，其次 `<exe-dir>/<tool>_<arch>`
3. 未找到 → 写入结构化 error，不回退到进程内算法
```

当前工作目录中的 `binaries/` 不会被搜索。

---

## 5. 网络诊断能力

### 网络探测 Workload 一览

| Workload 名称 | 说明 | 协议/方法 |
|---------------|------|----------|
| `Net Download (...)` | 默认节点单线程下载测速 | HTTP |
| `Net Multi-Thread Download` | Cloudflare 多线程下载测速 | HTTP |
| `Net Upload` | Cloudflare 上传测速 | HTTP PUT |
| `Network (iperf3 → HOST)` | iperf3 带宽测试 | TCP |
| `Net Ping` | TCP ping 延迟/jitter/丢包 | TCP |
| `Net Traceroute` | 三网回程路由追踪 | 系统 traceroute 命令 |
| `Net IP Quality` | IP 信誉/DNSBL/风险评分 | HTTP API |
| `Net Streaming Unlock` | 流媒体平台解锁检测 | HTTP |
| Suite `network_info` section | 公网 IPv4/IPv6、ASN/provider/location、NAT evidence | HTTP API + 本机地址 |
| Suite `reachability` section | Website HTTPS / Telegram DC TCP | HTTPS / TCP Connect |
| Suite `mail` section | 邮件端口可达性探测 | TCP Connect |

### 路由追踪（Traceroute）

- **系统命令**：Linux/macOS 按顺序尝试 `traceroute` / `tcptraceroute` / `tracepath`，Windows 使用 `tracert`
- **失败关闭**：命令缺失、无有效 hop、全超时或所有目标失败时记录结构化错误；目标最多 4 路并发
- **节点覆盖**：版本化 catalog 当前覆盖广州、北京、上海、成都，电信/联通/移动、CERNET、CSTNET 和 IPv4/IPv6；结果保留稳定 node ID、protocol、ASN 与 revision

### 版本化 Node Catalog

Manifest 字段为 `schema_version/revision/generated_at/expires_at/nodes[]`。节点字段为 `id/name/kind/region/city/carrier/asn/ip_family/protocol/endpoint/port/url/traffic_bytes/source`，kind 支持 `download/upload/route/ping/route_ping`；download 的 `traffic_bytes` 会限制单次响应体读取量。

- 默认 `embedded`：离线且确定，不依赖远端可用性
- `auto`：优先已验证 user cache，失败回退 embedded，不在 benchmark 中隐式更新
- path：显式加载本地 JSON；strict decoder 拒绝未知字段、重复 ID 和不合法 endpoint
- revision pin：选中 snapshot 与 pin 不一致时 probe 前失败
- signed update：Ed25519 detached signature + strict schema 均通过后才原子替换 cache（Unix mode `0600`）
- health：逐节点结构化 status/method/latency/error；它是可用性检查，不参与 benchmark metric

报告把 source 归一化为 `embedded|auto|path`，不写入用户 home/cache 的真实路径；管理 CLI JSON 才在显式 `path` 字段中显示文件位置。

### 网络身份与可达性

`network_info` 只在显式 Suite 网络 section 中执行；hardware-only `run` 不会增加公网身份请求。它结合本机 virtualization 与公网 provider 结果，分别记录 IPv4/IPv6 地址、ASN/provider/location，并以 `direct/translated/unknown` 表达可验证的 NAT 证据，不声称无法证明的 cone/symmetric NAT 类型。

`reachability` 默认测试 Google、GitHub、Cloudflare HTTPS 与 Telegram DC1-DC5 TCP 443。每个结果保留 target ID/category/protocol/endpoint/status/latency，HTTP probe 另带 status code，任何受限网络失败进入独立 error。

### IP 质量检测

输出结构：

| 字段 | 说明 |
|------|------|
| IP 基本信息 | IP 地址、AS 号、ISP、地区、国家 |
| 风险摘要 | 风险评分（0-100）、标签（如 datacenter/proxy/vpn） |
| DNSBL 检测 | 各 DNSBL 服务的检查结果 |
| 端口探测 | 常见邮件端口的开放状态 |

> IP Quality 的 0-100 风险评分是**业务诊断指标**，不是 benchmark 总分。

IP Quality 采用 fail-closed：元数据、公网 IPv4 或 DNSBL 结果不完整时不生成 score，只保留结构化 detail/error；DNSBL zone 会并发查询。

### 流媒体解锁检测

检测平台包括但不限于：

| 平台 | 检测方式 |
|------|---------|
| Netflix | HTTP 区域检测 |
| YouTube | HTTP 区域/Premium 检测 |
| Disney+ | HTTP 区域检测 |
| ChatGPT | HTTP 可用性检测 |
| TikTok | HTTP 区域检测 |
| Prime Video | HTTP 区域检测 |

### 邮件端口探测

检测端口：`25`、`465`、`587`、`2525`、`110`、`143`、`993`、`995`

每个端口返回：`open` / `closed` / `filtered` 状态

---

## 6. VPS Suite 综合测评

### 九个 Section

| Section ID | 名称 | 功能 |
|------------|------|------|
| `hardware` | 硬件测评 | CPU / 内存 / 磁盘基准测试 |
| `network_info` | 网络身份 | 虚拟化、公网 IPv4/IPv6、ASN/provider、NAT evidence |
| `route` | 路由追踪 | versioned node catalog traceroute 诊断 |
| `ping` | 延迟测试 | versioned node TCP latency / jitter / loss |
| `speed` | 速度测试 | 多提供商下载/上传测速 |
| `ip_quality` | IP 质量 | IP 信誉、DNSBL、风险评分 |
| `reachability` | 网站/TG | HTTPS/TCP latency/status/error |
| `mail` | 邮件端口 | 8 个邮件端口可达性探测 |
| `media` | 流媒体解锁 | 多平台解锁状态检测 |

### 四种场景预设

| 预设 | 使用场景 | 包含的 Section |
|------|---------|---------------|
| `quick` | 快速概览 | `hardware, network_info, speed, ip_quality` |
| `website` | 建站/服务端 | `hardware, network_info, route, ping, speed, ip_quality, reachability, mail` |
| `proxy` | 代理/解锁 | `network_info, route, ping, speed, ip_quality, reachability, media` |
| `mail` | 邮件服务器 | `network_info, route, ip_quality, mail` |

### Section 优先级规则

```
1. --preset 选择默认 section 集合
2. --only 会覆盖 preset 的 section 集合
3. --skip 在最终 section 集合上继续关闭指定 section
4. --ip-version、--route-presets 等显式参数优先于 preset 默认值
```

### Speed Section 输出层级

```
speed
├── summary           # 单 provider 汇总，或多 provider 的 best_per_metric
├── groups[]          # 按 provider 聚合的分组结果
│   ├── id            # provider ID
│   ├── status        # 整体状态
│   ├── available     # 成功项数
│   ├── failed        # 失败项数
│   ├── summary       # 该 provider 的聚合值
│   └── providers[]   # 该 provider 的具体探测项
└── providers[]       # 每个具体探测项的原始结果
    ├── id            # 探测项 ID
    ├── provider      # provider 名称
    ├── kind          # download / upload
    ├── status        # ok / error
    ├── download_mbps # 下载速度
    ├── upload_mbps   # 上传速度
    ├── latency_ms    # 延迟
    ├── elapsed_ms    # 耗时
    └── message       # 错误信息
```

### Speed 提供商

| Provider ID | 说明 |
|-------------|------|
| `cloudflare` | Cloudflare 多线程下载 + 上传 |
| `speedtest_net` | Ookla Speedtest CLI JSON 输出 |
| `speedtest_cn` | speedtest.cn 兼容 CLI JSON 输出 |
| `iperf3` | 需配合 `--iperf-host` 指定目标服务器 |

默认只启用 `cloudflare`。同时选择多个 provider 时，顶层 summary 会写入 `aggregation: "best_per_metric"`；下载、上传与延迟可能分别来自不同 provider。选择 `iperf3` 但没有可用 host 时直接返回 provider/section error。

Suite 只有所有 enabled section 都为 `status=ok` 才成功。enabled 的空状态、`skipped`、`partial`、`error` 都使总体失败、发 `section.fail` 并让 CLI 返回退出码 1；disabled section 才发 `section.skip`。默认 timeout 为 5 分钟：hardware 按 workload 应用，其他网络 section 各自派生 section context，deadline/cancel 写入结构化 error。Ping 全目标失败会保留逐目标 results 并返回聚合 error。

---

## 7. 报告输出体系

### 三种输出格式

#### Console（默认）

- 终端彩色表格
- 列：Workload / Category / Time / Throughput / Latency / Detail / Error
- 自动适配终端宽度

#### JSON（`--json report.json`）

以下为 Linux 默认 hardware scope 的 schema v2 示例；macOS/Windows 的 `hardware_tools` 默认值不同。config 会记录规范化后的 mode、scope、工具选择和可选 iperf hosts；network/all 还会记录 catalog source/revision/node IDs，hardware 会清除这些网络 provenance 字段。

```json
{
  "schema_version": 2,
  "version": "v0.1.0",
  "timestamp": "2026-06-02T12:00:00Z",
  "system": {},
  "config": {
    "iterations": 3,
    "disk_path": "/tmp",
    "extensions": false,
    "mode": "single",
    "scope": "hardware",
    "hardware_tools": ["sysbench", "openssl", "fio"]
  },
  "results": {
    "workloads": [
      {
        "name": "CPU Single-Core (sysbench)",
        "category": "CPU",
        "description": "sysbench CPU --threads=1 --cpu-max-prime=20000",
        "result": {
          "iterations": 3,
          "median_ms": 5002.3,
          "samples_ms": [5002.3, 4988.1, 5015.7],
          "throughput_per_sec": 8234.56,
          "throughput_unit": "events/sec",
          "detail": "sysbench cpu --threads=1 --cpu-max-prime=20000 run"
        }
      }
    ]
  },
  "extensions": {}
}
```

hardware scope 的 `extensions` 为 `false`。network/all scope 会设为 `true`；配置了 iperf3 目标时，config 还会保留目标列表，例如：

```json
{
  "extensions": true,
  "scope": "all",
  "iperf_hosts": ["iperf.example.com:5201"]
}
```

`bytes_processed` / `ops_processed` 是可选的累计量字段。Runner 只有在 workload 通过 `ProcessedMetricReporter` 明确声明累计字节或累计操作数，并且所有成功 sample 的语义一致时才输出对应字段；events/s、IOPS、MB/s、score、latency 等速率或诊断值不会被猜测为 processed。

#### HTML（`--html report.html`）

- 系统信息卡片（CPU / 内存 / GPU / OS / 磁盘 / 网络）
- 测评结果表格
- 扩展项表格
- 警告列表
- 响应式布局
- Suite HTML 额外展示 app/catalog metadata、hardware workloads、network identity、完整 route hops、ping、speed group/provider、IP quality、reachability、mail/media 及 detail/error

### 报告对比（`compare`）

```bash
vmbench compare report-a.json report-b.json report-c.json
vmbench history compare --last 3
```

对比规则：
- **time**：越低越好（绿色 = 更快）
- **latency**：越低越好（绿色 = 更低延迟）
- **throughput**：越高越好（绿色 = 更高吞吐）

命令先识别 report kind，benchmark 与 Suite 不能混合。Benchmark Compare 忽略带 error 的 metric，将 `ms avg` 按 latency 处理，不跨不兼容 throughput 单位计算 delta，并对迭代次数、mode、scope、硬件工具/iperf host 选择和重复 workload 给出可比性警告。

Suite Compare 对齐两份或更多 Suite v1/v2 JSON 的 raw metrics。Route/Ping 结果记录实际 `probe_protocol/probe_tool`，并显式保留成功的零值 Ping 指标。只有 unit、实际 protocol/IP family、provider/probe tool、target/node identity，以及节点型证据所需的 catalog revision 都兼容时才输出 delta；HTTP status 等分类码不参与百分比 delta。不兼容时仍显示各报告值，但 delta 留空并给出 reason/warning。Route hop count 等中性证据只用于对照，不解释成性能提升。

### Suite 报告结构

Suite 报告使用独立的 schema-v2 envelope，并保留 v1 兼容字段：

```
SuiteReport
├── schema_version   # 2
├── report_kind      # suite
├── report_id        # 唯一 ID
├── app              # version / commit / build_time
├── system           # 即使 network-only 也保留
├── started_at / finished_at / duration_ms
├── status          # ok / failed / running（section 另有 partial/error/skipped）
├── message         # 状态描述
├── started_time    # 开始时间戳
├── finished_time   # 结束时间戳
├── config          # Suite 配置
├── hardware        # HardwareSection（内嵌 Report）
├── network_info    # NetworkInfoSection（IPv4/v6/ASN/NAT）
├── route           # RouteSection（路由追踪结果）
├── ping            # PingSection（延迟测试结果）
├── speed           # SpeedSection（速度测试结果）
├── ip_quality      # IPQualitySection（IP 质量结果）
├── reachability    # ReachabilitySection（website/TG）
├── mail            # MailSection（邮件端口结果）
├── media           # MediaSection（流媒体解锁结果）
└── warnings        # catalog/环境/采集 warning
```

规范化 config 记录 catalog source/revision 与最终 node IDs；route/ping 另记录实际 probe protocol/tool，避免把不同协议、工具、IP family、provider、node 或 revision 的值误当成同一测量条件。

### 本地历史

历史目录按平台 data directory 选择，也可用 `VMBENCH_HISTORY_DIR` 覆盖，以 temp + fsync + rename 原子写入。Unix 使用目录 mode `0700`、记录 mode `0600`；其他平台依赖系统 ACL。`run` / `suite --save-history` 自动存储生成报告；`history add` 可导入旧报告，list/show/delete 管理记录，`compare --last N` 只接受同 report kind。

---

## 8. TUI 交互界面

### 入口

```bash
vmbench          # 直接启动 TUI（无参数时的默认行为）
```

### 页面导航

```
Dashboard（主菜单）
├── → Hardware Benchmark → Running → Results
├── → Suite → SuiteConfig → SuiteRunning → SuiteResults
├── → Compare（加载两份 benchmark JSON；Suite Compare 走 CLI/history）
└── → System Info（查看系统信息）
```

上图为 Go Bubble Tea TUI。

### 页面功能详解

#### Dashboard

- 主菜单入口，显示所有可用功能
- 按 `t` 切换主题，选择自动保存到本地配置
- 显示当前主题名称

#### Hardware Benchmark（Running + Results）

- 选择后进入 Running 页面，实时显示每个 workload 的执行进度
- Go TUI 不提供独立 Multi-Core 入口；workload 串行执行，线程数和队列深度由外部工具参数定义
- 进度信息：当前 workload 名称、迭代进度、状态（ok/error）
- 完成后自动跳转 Results 页面

#### Results

- 支持 3 种视图模式（按 `Tab` 切换）：
  - **Cards**：卡片式展示，按 Category 分组
  - **Grouped**：紧凑分组表格
  - **Flat**：扁平列表，逐行展示
- 每项显示：名称、分类、median time、throughput、latency、detail/error
- 按 `s` 保存为 JSON 文件

#### Suite（SuiteConfig → SuiteRunning → SuiteResults）

- **SuiteConfig**：初始实际应用 Quick preset（hardware/network_info/speed/ip_quality），speed provider 默认只选 Cloudflare；与 CLI/MCP 共用 iterations/timeout/IP version/tools/providers/iperf/sections/route/catalog source/revision 归一化模型
- **SuiteRunning**：实时显示 section 执行状态（start/done/fail/skip）
- **SuiteResults**：展示各 section 结果摘要和详细数据

#### Compare

- 加载两份 benchmark JSON 报告并按 workload 并排展示
- Suite JSON 使用 `vmbench compare` 或 `history compare --last N`；当前不进入 TUI Compare 页面
- 颜色编码：绿色 = 改善，红色 = 退化

### 键盘快捷键

| 按键 | 功能 |
|------|------|
| `↑` / `k` | 上移 |
| `↓` / `j` | 下移 |
| `Enter` | 选择/确认 |
| `Tab` | 切换视图（Cards/Grouped/Flat） |
| `t` | 在 Dashboard 切换主题 |
| `s` | 保存结果 |
| `Esc` | 返回上一页 |
| `q` | 退出 |

### 8 套主题

| 主题名称 | 风格 |
|---------|------|
| `dracula` | 经典暗色紫蓝系（默认） |
| `tokyonight` | Tokyo Night 暗色系 |
| `catppuccin` | Catppuccin Mocha 暗色系 |
| `nord` | Nord 极光暗色系 |
| `gruvbox` | Gruvbox 暖色系 |
| `rose-pine` | Rosé Pine 柔和暗色系 |
| `solarized` | Solarized 经典色系 |
| `monochrome` | 纯灰度色系 |

每个主题包含：
- 背景色（Bg / Surface / Overlay）
- 前景色（Fg / Muted / Subtle）
- 强调色（Primary / Secondary / Accent）
- 语义色（Success / Warning / Danger / Info）
- 边框色（Border / BorderFocus）
- 分类色（Integer / Float / Memory / Disk / Network / System）

所有颜色使用 `AdaptiveColor`，自动适配终端的亮色/暗色模式。

### 主题配置方式

```bash
# 方式 1：在 TUI 中按 t 键切换（自动保存）
# 方式 2：环境变量
VMBENCH_THEME=nord vmbench

# 方式 3：配置文件 ~/.config/vmbench/config.json
```

### TUI 组件库（tui/comp/）

可复用 UI 组件：

| 组件 | 说明 |
|------|------|
| Card | 带边框的内容容器 |
| Progress | 进度条 |
| Spinner | 加载动画 |
| StatusPill | 状态标签 |
| KVGrid | 键值网格布局 |
| Table | 数据表格 |
| Modal | 模态对话框 |
| Tabs | 标签页导航 |
| Toast | 临时通知 |
| Banner | 大号标题 |
| Sparkline | 迷你图表 |

### 响应式布局

5 个断点自适应不同终端宽度。

---

## 9. MCP 大模型接入

### 架构

```
LLM 客户端（Claude / Cursor / Cline）
        ↕ MCP JSON-RPC over stdio
vmbench mcp serve
        ↕ 内部 API
vmbench.RunCore / suite.Run / sysinfo.Collect
```

### 启动方式

```bash
# 命令行启动
vmbench mcp serve --transport stdio

```

### 客户端配置示例

```json
{
  "mcpServers": {
    "vmbench": {
      "command": "/usr/local/bin/vmbench",
      "args": ["mcp", "serve", "--transport", "stdio"]
    }
  }
}
```

### 可用工具

#### `vmbench_capabilities`

列出 vmbench 的完整能力信息：
- 版本号
- Suite sections 列表
- 预设列表
- 硬件工具列表
- 速度提供商列表
- 可用 workload 列表

#### `vmbench_sysinfo`

采集当前主机系统信息：
- CPU、内存、GPU、磁盘、网络、OS
- 返回结构化 JSON + 文本摘要

#### `vmbench_run`

运行硬件基准测试：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `iterations` | int | 1 | 迭代次数（1-9） |
| `filter` | string | "" | workload 过滤正则 |
| `mode` | enum | "single" | 执行模式；multi/all 仅兼容并只运行一次 catalog |
| `scope` | enum | "hardware" | hardware/network/all；网络必须显式开启 |
| `hardware_tools` | enum[] | 平台相关 | Linux sysbench/OpenSSL/fio；macOS OpenSSL；Windows WinSAT |
| `iperf_hosts` | string[] | [] | iperf3 目标；仅 network/all scope 有意义 |
| `catalog_source` | string | "embedded" | embedded / auto / 显式 path；network/all 使用 |
| `catalog_revision` | string | "" | 精确 revision pin |
| `timeout_ms` | int | 300000 | 超时毫秒（最大 900000） |

#### `vmbench_suite`

运行 VPS 综合测评：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `iterations` | int | 1 | hardware workload 迭代次数（1-9） |
| `filter` | string | "" | hardware workload 过滤正则 |
| `timeout_ms` | int | 300000 | section timeout；hardware 按 workload 应用，最大 900000 |
| `preset` | enum | "" | 场景预设 |
| `only` | enum[] | [] | 指定 section |
| `skip` | enum[] | [] | 跳过 section |
| `ip_version` | enum | "v4" | IP 版本 |
| `hardware_tools` | enum[] | 平台相关 | 硬件工具 |
| `speed_providers` | enum[] | ["cloudflare"] | 速度提供商 |
| `route_presets` | enum[] | ["gz","bj","sh","cd","cernet","cstnet"] | 路由/线路预设 |
| `iperf_hosts` | string[] | [] | iperf3 服务器 |
| `catalog_source` | string | "embedded" | embedded / auto / 显式 path |
| `catalog_revision` | string | "" | 精确 revision pin |

### 安全边界

| 约束 | 说明 |
|------|------|
| 无 shell 执行 | 不接受任意 shell 命令，只调用 vmbench 内部 API |
| 参数枚举限制 | hardware_tools / section / preset / provider / route preset 限制在预定义列表 |
| Catalog fail-closed | source/revision 在运行前解析；pin 不匹配或 path 无效不启动 probe |
| 迭代上限 | `iterations` 最大 9 |
| 超时上限 | `timeout_ms` 最大 15 分钟（900000ms） |
| 互斥运行 | Server 内部互斥锁，同一时间只允许一个 benchmark |
| 默认保守 | `vmbench_run` 默认 scope=hardware，`vmbench_suite` 默认只跑 hardware |
| 网络显式开启 | run 通过 scope，suite 通过 preset 或 only 显式启用 |
| stdout 专用 | stdout 只写 JSON-RPC response，诊断信息写 stderr |
| 原始指标 | 返回原始指标和结构化错误，不输出总分/等级 |

参数缺省与显式非法值严格区分：省略 `iterations` 时默认为 1，省略 `timeout_ms` 时默认为 5 分钟；显式传入非正数、超过上限、非法 regex，或在合法枚举数组中混入未知 section/provider/tool/route preset 时，整个 tool call 以 `isError=true` 拒绝且不启动测量。

参数校验失败只返回错误文本。测量已经启动后若 workload 或 suite section 失败，tool result 仍保留完整 `structuredContent.report` 和文本摘要，同时设置 `isError=true`，调用方可以读取结构化失败细节而不是丢失报告。

CLI、TUI、MCP 复用同一 Suite normalization/validation contract，避免相同输入在不同入口产生不同 sections、IP version、provider、timeout 或 catalog provenance。MCP capabilities 同步列出 `network_info` / `reachability` 和 catalog source/revision 字段。

---

## 10. 系统信息采集

### 采集维度

| 维度 | 采集内容 | 数据来源 |
|------|---------|---------|
| **CPU** | 型号、架构、核心数、频率、缓存、特性、微架构、NUMA 节点 | gopsutil + /proc/cpuinfo |
| **内存** | 总量（字节）、类型、频率、通道数 | gopsutil + dmidecode（Linux） |
| **GPU** | 型号、显存、驱动版本 | lspci（Linux）/ system_profiler（macOS）/ PowerShell CIM（Windows） |
| **OS** | 名称、内核版本、主机名、Go 版本 | runtime + syscall |
| **磁盘** | 设备名、挂载点、文件系统类型、总容量 | gopsutil + df |
| **网络** | 接口数量、活跃接口名称 | gopsutil |
| **虚拟化** | system/role（KVM/VMware/Hyper-V/container 等 best effort） | gopsutil host + 平台 fallback |

### 跨平台策略

| 平台 | CPU | 内存 | GPU | 磁盘 | 网络 |
|------|-----|------|-----|------|------|
| **Linux** | `/proc/cpuinfo`、`/sys` | `/proc/meminfo`、`dmidecode` | `lspci` | `df` | `gopsutil` |
| **macOS** | `sysctl` | `sysctl` | `system_profiler` | `df` | `ifconfig` |
| **Windows** | PowerShell CIM | PowerShell CIM | PowerShell CIM | PowerShell CIM | `Get-NetAdapter` |

`sysinfo.Collect(ctx)` 会把调用方 context 传给每个 collector。context 已取消时立即返回 warning；平台外部命令同时继承 parent deadline，并有最长 30 秒的子超时。

### 输出示例（`vmbench sysinfo --json`）

```json
{
  "cpu": {
    "model": "AMD EPYC 7763",
    "arch": "amd64",
    "physical_cores": 8,
    "logical_cores": 16,
    "base_freq_mhz": 2450.0,
    "max_freq_mhz": 3200.0,
    "cache_sizes": {"L3": 268435456},
    "features": ["avx2", "sse4_2"],
    "micro_arch": "Zen 3",
    "numa_nodes": 1
  },
  "memory": {
    "total_bytes": 34359738368,
    "type": "DDR4",
    "freq_mhz": 3200,
    "channels": 2
  },
  "os": {
    "name": "Ubuntu 22.04 LTS",
    "kernel": "5.15.0-91-generic",
    "hostname": "vps-01",
    "go_version": "go1.26.5"
  },
  "disks": [
    {
      "device": "/dev/vda1",
      "mountpoint": "/",
      "fs_type": "ext4",
      "total_bytes": 107374182400
    }
  ],
  "network": {
    "interface_count": 2,
    "active_names": ["eth0"]
  },
  "virtualization": {
    "system": "kvm",
    "role": "guest"
  }
}
```

---

## 11. Go API 编程接口

### 直接调用 RunCore

```go
package main

import (
    "context"
    "fmt"
    "time"

    vmbench "github.com/cloudapp3/vmbench"
)

func main() {
    report := vmbench.RunCore(context.Background(), vmbench.Options{
        Iterations:    3,
        Engine:        "external",
        Mode:          "single",
        Scope:         vmbench.ScopeHardware,
        Timeout:       5 * time.Minute,
    })

    fmt.Printf("Version: %s\n", report.Version)
    fmt.Printf("Workloads: %d\n", len(report.Results.Workloads))

    for _, w := range report.Results.Workloads {
        if w.Result != nil {
            fmt.Printf("  %s: %.2f %s\n",
                w.Name,
                w.Result.ThroughputPerSec,
                w.Result.ThroughputUnit,
            )
        }
    }
}
```

### 带 Progress 回调

```go
report := vmbench.RunCore(context.Background(), vmbench.Options{
    Iterations: 3,
    OnEvent: func(evt vmbench.Event) {
        switch evt.Kind {
        case vmbench.EventSuiteStart:
            fmt.Printf("▶ 开始: %s\n", evt.Workload)
        case vmbench.EventSuiteProgress:
            fmt.Printf("  进度: %s (%d/%d) %.0f%%\n",
                evt.Workload, evt.Current, evt.Total, evt.Progress*100)
        case vmbench.EventSuiteDone:
            fmt.Printf("✔ 完成: %s (%.2fs)\n", evt.Workload, evt.Duration.Seconds())
        case vmbench.EventSuiteFail:
            fmt.Printf("✘ 失败: %s - %s\n", evt.Workload, evt.Err)
        case vmbench.EventBenchDone:
            fmt.Println("全部完成！")
        }
    },
})
```

### 调用 Suite

```go
import "github.com/cloudapp3/vmbench/suite"

report := suite.Run(context.Background(), suite.Options{
    Preset:          "quick",
    CatalogSource:   "auto",
    CatalogRevision: "2026-07-13.1",
    OnEvent: func(evt suite.Event) {
        fmt.Printf("[%s] %s: %s\n", evt.Kind, evt.Section, evt.Message)
    },
})
```

`suite.Run` 在执行前通过共享 normalization/validation 解析 catalog；pin 或 schema 不匹配会返回结构化失败，不会改用另一 revision。调用 `vmbench.NormalizeOptions` / `suite.NormalizeOptions` 可在不运行 workload 的情况下得到 CLI/TUI/MCP 一致的规范化配置。

### 采集系统信息

```go
import "github.com/cloudapp3/vmbench/sysinfo"

system, warnings := sysinfo.Collect(context.Background())
fmt.Printf("CPU: %s\n", system.CPU.Model)
fmt.Printf("Memory: %d bytes\n", system.Memory.Total)
```

### Workload 接口（自定义扩展）

```go
import "github.com/cloudapp3/vmbench/bench"

type MyWorkload struct{}

func (w *MyWorkload) Name() string        { return "Custom Test" }
func (w *MyWorkload) Category() string    { return "Integer" }
func (w *MyWorkload) Description() string { return "My custom benchmark" }

func (w *MyWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
    start := time.Now()
    // ... 执行测试逻辑 ...
    elapsed := time.Since(start)
    return elapsed, 1000000, nil
}

func (w *MyWorkload) Validate() error { return nil }

// 可选接口实现
func (w *MyWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
    return float64(processed) / elapsed.Seconds(), "ops/s"
}

func (w *MyWorkload) ProcessedKind() bench.ProcessedKind {
    return bench.ProcessedOperations // Run 返回累计操作数时才声明
}

func (w *MyWorkload) SkipWarmup() bool { return true }
```

---

## 12. 核心数据模型

### Workload 接口

```go
// 核心接口
type Workload interface {
    Name() string
    Category() string
    Description() string
    Run(ctx context.Context) (elapsed time.Duration, processed int64, err error)
    Validate() error
}

// 可选扩展接口
type CloneableWorkload interface {
    Workload
    Clone() Workload
}

type IterationLimiter interface {
    MaxIterations() int
}

type ProcessedKind uint8

const (
    ProcessedUnknown ProcessedKind = iota
    ProcessedBytes
    ProcessedOperations
)

type ProcessedMetricReporter interface {
    ProcessedKind() ProcessedKind
}

type ThroughputMeter interface {
    Throughput(processed int64, elapsed time.Duration) (float64, string)
}

type LatencyWorkload interface {
    AverageLatencyNS(processed int64, elapsed time.Duration) float64
}

type DetailReporter interface {
    Detail() string
}

type WarmupSkipper interface {
    SkipWarmup() bool
}
```

### Category 常量

| 常量 | 值 | 说明 |
|------|---|------|
| `CategoryInteger` | `"Integer"` | 整数运算类 |
| `CategoryFloat` | `"Float"` | 浮点运算类 |
| `CategoryMemory` | `"Memory"` | 内存类 |
| `CategoryExtensionDisk` | `"Extension/Disk"` | 磁盘扩展类 |
| `CategoryNetwork` | `"Network"` | 网络类 |

### RunDetail（运行时测量结果）

```go
type RunDetail struct {
    Iterations       int             // 迭代次数
    MedianTime       time.Duration   // 中位数耗时
    Throughput       float64         // 吞吐量值
    ThroughputUnit   string          // 吞吐量单位（MiB/s、events/sec、IOPS 等）
    Samples          []time.Duration // 每次迭代的原始耗时
    BytesProcessed   int64           // 可选累计字节数；语义明确时填写
    OpsProcessed     float64         // 可选累计操作数；语义明确时填写
    AverageLatencyNS float64         // 平均延迟（纳秒/操作）
    Detail           string          // 人类可读详情
    Error            string          // 结构化错误信息
}
```

### ResultEntry（报告层结果）

```go
type ResultEntry struct {
    Iterations       int       `json:"iterations,omitempty"`        // 实际迭代次数
    MedianMS         float64   `json:"median_ms"`                   // 中位数耗时（毫秒）
    SamplesMS        []float64 `json:"samples_ms,omitempty"`       // 原始耗时样本
    ThroughputPerSec float64   `json:"throughput_per_sec"`         // 吞吐量/秒
    ThroughputUnit   string    `json:"throughput_unit"`            // 吞吐量单位
    AvgNSPerAccess   float64   `json:"avg_ns_per_access,omitempty"` // 平均延迟
    BytesProcessed   int64     `json:"bytes_processed,omitempty"`  // 可选累计字节数
    OpsProcessed     float64   `json:"ops_processed,omitempty"`    // 可选累计操作数
    Detail           string    `json:"detail,omitempty"`           // 详情
    Error            string    `json:"error,omitempty"`            // 错误
}
```

### Document（完整报告）

```go
type Document struct {
    SchemaVersion int                `json:"schema_version"`       // 当前为 2
    Version       string             `json:"version"`              // vmbench 版本
    Timestamp     time.Time          `json:"timestamp"`            // 报告生成时间（UTC）
    System        sysinfo.SystemInfo `json:"system"`               // 系统信息
    Config        RunConfig          `json:"config"`               // 运行配置
    Results       ResultsSection     `json:"results"`              // 核心 workload 结果
    Extensions    ExtensionsSection  `json:"extensions,omitempty"` // 扩展 workload 结果
    Warnings      []string           `json:"warnings,omitempty"`   // 警告信息
}
```

### RunConfig（报告中的规范化配置）

```go
type RunConfig struct {
    Iterations    int      `json:"iterations"`
    Filter        string   `json:"filter,omitempty"`
    DiskPath      string   `json:"disk_path,omitempty"`
    Extensions    bool     `json:"extensions"` // hardware=false; network/all=true
    Mode          string   `json:"mode,omitempty"`
    Scope         string   `json:"scope,omitempty"`
    HardwareTools []string `json:"hardware_tools,omitempty"`
    IperfHosts    []string `json:"iperf_hosts,omitempty"`
    CatalogSource string   `json:"catalog_source,omitempty"`
    CatalogRevision string `json:"catalog_revision,omitempty"`
    NodeIDs       []string `json:"node_ids,omitempty"`
}
```

### Options（运行选项）

```go
type Options struct {
    DiskPath      string          // 磁盘测试路径
    TraceTarget   string          // 追踪目标
    Timeout       time.Duration   // 单 workload 超时
    Iterations    int             // 迭代次数（1-9）
    Filter        string          // workload 过滤正则
    OnEvent       EventHandler    // 事件回调函数
    Mode          string          // single；multi/all 仅兼容并归一化为 single
    Engine        string          // external（native/full 已废弃）
    Scope         string          // hardware / network / all（默认 hardware）
    IperfHosts    []string        // iperf3 服务器列表
    HardwareTools []string        // 硬件工具 ID 列表
    CatalogSource string          // embedded / auto / path
    CatalogRevision string        // 精确 revision pin
}
```

---

## 13. 事件系统

### Run 事件（RunCore）

| EventKind | 触发时机 | 携带数据 |
|-----------|---------|---------|
| `suite_start` | 每个 workload 首个 sample 进入前 | Workload 名称、总数；同名 workload 也逐项触发 |
| `suite_progress` | 迭代进度更新 | 当前迭代、进度百分比、状态 |
| `suite_done` | workload 成功完成 | Workload 名称、耗时 |
| `suite_skip` | workload 被跳过 | Workload 名称 |
| `suite_fail` | workload 执行失败 | Workload 名称、错误信息 |
| `bench_done` | 全部 workload 完成 | 进度 100% |
| `bench_log` | 警告/日志信息 | 消息文本 |

### Suite Section 事件

| EventKind | 触发时机 |
|-----------|---------|
| `section.start` | section 启动 |
| `section.done` | section 完成且 status=ok |
| `section.fail` | enabled section 完成但 status≠ok（含 skipped/partial/error/空状态） |
| `section.skip` | section 未启用 |
| `suite.done` | 全部 section 完成 |

### Event 结构

```go
type Event struct {
    Kind      EventKind    // 事件类型
    Suite     string       // suite 名称
    Workload  string       // workload 名称
    Category  string       // 分类
    Iteration int          // 当前迭代号
    Current   int          // 已完成样本数
    Total     int          // 总样本数
    Progress  float64      // 进度百分比（0-1）
    Metric    string       // 指标名称
    Duration  time.Duration // 耗时
    Err       error        // 错误
    Message   string       // 消息
    Status    string       // 状态
}
```

---

## 14. 构建与分发

### Go 构建

```bash
# 标准构建
go build -ldflags "-X github.com/cloudapp3/vmbench.Version=$(git describe --tags 2>/dev/null || echo dev)" \
    -o vmbench ./cmd/vmbench/

# 交叉编译
GOOS=linux   GOARCH=amd64 go build -ldflags "..." -o vmbench-linux-amd64  ./cmd/vmbench/
GOOS=linux   GOARCH=arm64 go build -ldflags "..." -o vmbench-linux-arm64  ./cmd/vmbench/
GOOS=darwin  GOARCH=amd64 go build -ldflags "..." -o vmbench-darwin-amd64 ./cmd/vmbench/
GOOS=darwin  GOARCH=arm64 go build -ldflags "..." -o vmbench-darwin-arm64 ./cmd/vmbench/
GOOS=windows GOARCH=amd64 go build -ldflags "..." -o vmbench-windows-amd64 ./cmd/vmbench/
```

### Go 依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `charmbracelet/bubbletea` | v1.3+ | TUI 框架 |
| `charmbracelet/lipgloss` | v1.1+ | TUI 样式 |
| `charmbracelet/bubbles` | v1.0+ | TUI 组件 |
| `shirou/gopsutil` | v4.25+ | 系统信息采集 |
| `klauspost/compress` | v1.18+ | 压缩算法 workload |
| `klauspost/cpuid` | v2.2+ | CPU 特性检测 |
| `pierrec/lz4` | v4.1+ | LZ4 压缩 workload |

### GoReleaser 配置

- **目标平台**：Linux（amd64/arm64）、macOS（amd64/arm64）、Windows（amd64/arm64）
- **归档格式**：tar.gz（Linux/macOS）、zip（Windows）
- **包管理**：deb、rpm
- **发布自动化**：CI + GitHub Release

### 一键安装

```bash
# 安装最新版本
curl -fsSL https://raw.githubusercontent.com/cloudapp3/vmbench/main/install.sh | bash

# 安装指定版本
curl -fsSL ... | bash -s -- --version v0.1.0

# 自定义安装目录
curl -fsSL ... | bash -s -- --dir /opt/bin
```

安装脚本功能：
- 自动检测平台和架构
- SHA-256 校验和验证
- 多策略安装目录发现
- GitHub API 版本解析

---

## 15. 跨平台支持矩阵

### 运行平台

| 平台 | 架构 | CLI | TUI | MCP | Suite |
|------|------|:---:|:---:|:---:|:-----:|
| Linux | amd64 | ✅ | ✅ | ✅ | ✅ |
| Linux | arm64 | ✅ | ✅ | ✅ | ✅ |
| macOS | amd64 | ✅ | ✅ | ✅ | ✅ |
| macOS | arm64 | ✅ | ✅ | ✅ | ✅ |
| Windows | amd64 | ✅ | ✅ | ✅ | ✅ |
| Windows | arm64 | ✅ | ✅ | ✅ | ✅ |

### 硬件工具平台支持

| 工具 | Linux amd64 | Linux arm64 | macOS | Windows |
|------|:-----------:|:-----------:|:-----:|:-------:|
| sysbench | ✅ 默认；PATH + executable-adjacent fallback | ✅ 默认；PATH + executable-adjacent fallback | 可选；PATH | — |
| openssl | ✅ 默认 | ✅ 默认 | ✅ 默认 | 可选 |
| fio | ✅ 默认；PATH + executable-adjacent fallback | ✅ 默认；PATH + executable-adjacent fallback | 可选；PATH | — |
| dd | ✅ | ✅ | ✅ | — |
| STREAM | ✅ | ✅ | ✅ | — |
| mbw | ✅ | ✅ | — | — |
| Geekbench | ✅ | ✅ | ✅ | ✅ |
| WinSAT | — | — | — | ✅ 默认 |
| iperf3 | ✅ | ✅ | ✅ | ✅ |

### 系统信息平台策略

| 维度 | Linux | macOS | Windows |
|------|-------|-------|---------|
| CPU | `/proc` + `/sys` | `sysctl` | PowerShell CIM |
| 内存 | `/proc/meminfo` + `dmidecode` | `sysctl` | PowerShell CIM |
| GPU | `lspci` | `system_profiler` | PowerShell CIM |
| 磁盘 | `df` | `df` | PowerShell CIM |
| 网络 | gopsutil | `ifconfig` | `Get-NetAdapter` |
| OS | `/proc` + `uname` | `sw_vers` | PowerShell CIM |

---

## 16. ECS 对标差异

`vmbench ecs-diff` 命令输出当前与 ECS（spiritLHLS/ecs shell 版 + oneclickvirt/ecs Go 版）的产品差异快照。

### 当前对齐与剩余差距

| 能力 | 状态 | 说明 |
|------|------|------|
| 虚拟化/公网 IP/ASN/NAT | aligned | `sysinfo.virtualization` + Suite `network_info`，NAT 采用保守证据 |
| 网站/TG 可达性 | aligned | 独立 `reachability` section，逐目标结构化结果 |
| 成都/CERNET/CSTNET/IPv6 | aligned | embedded/versioned catalog + stable node ID/revision |
| Suite/history compare | vmbench_only | 多报告 compatible-only delta 与 `--last N` |
| 长期节点运营规模 | partial | 已有 signed update/health/pin，公开运营源和覆盖规模仍需扩展 |
| 报告分享链接 | gap/P2 | 需要显式授权、默认脱敏的 upload/share adapter |
| 工具分发/多磁盘 | partial/P2 | 核心 adapter 已有，安装发现和多磁盘模式仍待增强 |

### 查看差异

```bash
# 终端文本输出
vmbench ecs-diff

# JSON 输出
vmbench ecs-diff --json
```

完整快照见 [`docs/ecs-comparison.md`](ecs-comparison.md)。

---

## 17. 完整目录结构

```
vmbench/
│
├── bench/                          # 基准测试核心
│   ├── common/                     # 通用工具
│   │   ├── gc.go                   # GC 控制（禁用 GC 进行精确测量）
│   │   ├── random.go               # 随机数据源
│   │   ├── stats.go                # 统计工具（中位数等）
│   │   └── sink.go                 # 数据汇聚工具
│   ├── diskio/                     # Legacy 进程内磁盘 workload（未注册）
│   ├── float/                      # Legacy 进程内浮点 workload（未注册）
│   ├── integer/                    # Legacy 进程内整数 workload（未注册）
│   ├── memory/                     # Legacy 进程内内存 workload（未注册）
│   ├── netio/                      # 网络探测 workload
│   │   ├── download.go             # HTTP 下载测速
│   │   ├── upload.go               # HTTP 上传测速
│   │   ├── ping.go                 # TCP ping
│   │   ├── route.go                # 路由追踪
│   │   ├── ip_quality.go           # IP 质量检测
│   │   ├── network_identity.go     # 公网 IP/ASN/NAT evidence
│   │   ├── reachability.go         # Website/Telegram 可达性
│   │   ├── media.go                # 流媒体解锁
│   │   ├── mail.go                 # 邮件端口探测
│   │   └── iperf.go                # iperf3 带宽测试
│   ├── runner.go                   # 串行隔离的多迭代执行器
│   └── workload.go                 # Workload 接口定义
│
├── catalog/                        # 外部工具注册中心
│   └── catalog.go                  # workload 定义 + 工厂函数 + 解析器
│
├── nodecatalog/                    # 版本化网络节点
│   ├── nodes.json                  # embedded 离线 snapshot
│   ├── load.go                     # embedded/auto/path + revision pin
│   ├── signature.go                # Ed25519 verify + atomic update
│   └── health.go                   # 有界并发 health check
│
├── history/                        # 原子本地报告历史
│   └── history.go                  # add/list/show/delete/latest + atomic store
│
├── cmd/vmbench/                    # CLI 入口
│   ├── main.go                     # 命令分发
│   ├── mcp.go                      # MCP serve 子命令
│   ├── ecs_diff.go                 # ECS 差异快照子命令
│   └── tui.go                      # TUI 子命令入口
│
├── mcp/                            # MCP JSON-RPC 服务器
│   └── server.go                   # stdio transport + 4 工具
│
├── report/                         # 报告生成
│   ├── types.go                    # 数据结构定义
│   ├── console.go                  # 终端表格输出
│   ├── json.go                     # JSON 文件输出
│   ├── html.go                     # HTML 可视化输出
│   └── compare.go                  # 并排对比输出
│
├── suite/                          # VPS 综合测评
│   ├── types.go                    # Section/Report 类型定义
│   ├── options.go                  # Suite 选项归一化
│   ├── config_validation.go        # CLI/TUI/MCP 共享校验 + catalog resolve
│   ├── run.go                      # Section 编排执行
│   ├── hardware.go                 # hardware section 实现
│   ├── network_info.go             # network_info section 实现
│   ├── route.go                    # route section 实现
│   ├── ping.go                     # ping section 实现
│   ├── speed.go                    # speed section 实现
│   ├── ip_quality.go               # ip_quality section 实现
│   ├── reachability.go             # reachability section 实现
│   ├── mail.go                     # mail section 实现
│   └── media.go                    # media section 实现
│
├── suitecompare/                   # Suite raw metric compare
│   └── compare.go                  # unit/protocol/provider/node/revision gate
│
├── sysinfo/                        # 系统信息采集
│   ├── types.go                    # 数据结构 + Collect 入口
│   ├── cpu.go / cpu_linux.go / cpu_darwin.go / cpu_windows.go
│   ├── memory.go / memory_linux.go / ...
│   ├── gpu.go / gpu_linux.go / ...
│   ├── os.go / os_linux.go / ...
│   ├── disk.go / disk_linux.go / ...
│   └── network.go / network_linux.go / ...
│
├── tui/                            # Bubble Tea TUI
│   ├── app.go                      # 主模型 + 页面路由
│   ├── dashboard.go                # 主菜单
│   ├── running.go                  # 实时进度页
│   ├── results.go                  # 结果展示页
│   ├── compare.go                  # 报告对比页
│   ├── suite_config.go             # Suite 配置页
│   ├── suite_running.go            # Suite 执行进度页
│   ├── suite_results.go            # Suite 结果页
│   ├── styles.go                   # Lip Gloss 样式定义
│   ├── keys.go                     # 快捷键绑定
│   ├── config.go                   # 主题持久化
│   ├── theme/                      # 8 套主题
│   │   └── theme.go                # Theme 结构 + AdaptiveColor
│   └── comp/                       # 可复用组件库
│       ├── card.go                 # 卡片容器
│       ├── progress.go             # 进度条
│       ├── spinner.go              # 加载动画
│       ├── table.go                # 数据表格
│       ├── modal.go                # 模态框
│       ├── tabs.go                 # 标签页
│       ├── toast.go                # 通知
│       ├── banner.go               # 标题
│       └── sparkline.go            # 迷你图表
│
├── docs/                           # 文档
│   ├── capabilities.md             # ← 本文档：能力全景
│   ├── product.md                  # 产品说明
│   ├── tech-stack.md               # 技术栈
│   ├── current-state.md            # 当前实现状态
│   ├── tui-design.md               # TUI 设计文档
│   ├── tui-redesign.md             # TUI 重设计
│   ├── ecs-comparison.md           # ECS 对标差异
│   ├── CHANGELOG.md                # 变更记录
│   └── README.zh-CN.md             # 中文快速参考
│
├── sh/                             # Shell 脚本
│   └── (辅助脚本)
│
├── .github/                        # GitHub 配置
│   └── workflows/
│       └── ci.yml                  # CI 流水线
│
├── .goreleaser.yml                 # GoReleaser 配置
├── install.sh                      # 一键安装脚本
├── go.mod / go.sum                 # Go 模块定义
├── run.go                          # RunCore 编排入口
├── options.go                      # Options 定义与归一化
├── config_validation.go            # run 共享校验 + catalog resolve
├── types.go                        # 包级类型别名
├── events.go                       # 事件类型定义
├── emit.go                         # 事件发射辅助
├── version.go                      # 版本号（构建时注入）
├── CONTRIBUTING.md                 # 贡献指南
├── LICENSE                         # MIT 许可证
└── README.md                       # 项目主 README
```

---

## 附录：文档导航

| 文档 | 说明 |
|------|------|
| [README.md](../README.md) | 项目主入口和快速开始 |
| [docs/product.md](product.md) | 产品规格说明 |
| [docs/tech-stack.md](tech-stack.md) | 技术架构详解 |
| [docs/current-state.md](current-state.md) | 最新实现状态 |
| [docs/tui-design.md](tui-design.md) | TUI 设计文档 |
| [docs/ecs-comparison.md](ecs-comparison.md) | ECS 对标差异 |
| [docs/CHANGELOG.md](CHANGELOG.md) | 变更记录 |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | 贡献指南 |

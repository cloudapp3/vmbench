# vmbench 分享能力设计（share）

> 状态：设计稿，未实现。本文是 `share` 能力的实现规范；实现时按 phase 更新本文状态，并同步 README / product / tech-stack / tui-design / CHANGELOG。

## 1. 背景与目标

对标 ECS（融合怪）的结论（见 [`ecs-comparison.md`](ecs-comparison.md)）：vmbench 与 ECS 功能重合度高，最大差距是"跑完即链接"的分享闭环——ECS 默认把结果上传 pastebin/h501 并返回链接，用户复制即可贴帖；vmbench 的报告只存在于本地磁盘。

`share` 的目标是补上这一环，同时守住三条底线（`docs/product.md` 已承诺）：

1. **显式授权**：任何上传必须由用户显式触发，绝无默认上传；MCP 入口 v1 不提供分享。
2. **支持脱敏**：上传前对报告做结构化脱敏，默认档案隐藏可直接滥用的本机地址。
3. **不伪造内容**：分享物是报告的忠实投影（同一 JSON 派生），不做美化、不出总分。

### 非目标

- 不做账号体系、不自建后端服务、不做结果聚合与排行榜。
- 不做静默降级：体积超限、服务商拒绝、脱敏失败都是结构化错误，绝不悄悄改内容再上传。
- v1 不做端到端加密 / 阅后即焚（可作为后续 provider 能力）。

## 2. 敏感字段盘点（基于当前 schema）

设计脱敏前先明确"什么算敏感"。当前两类报告中的敏感数据分布：

| 数据 | 位置 | 敏感度 | 说明 |
|---|---|---|---|
| 主机名 | `system.hostname`（`sysinfo/types.go`） | 高 | 可关联机主 |
| 公网 IPv4/IPv6 | `network_info.result.public_ipv4.ip` / `public_ipv6.ip` | **最高** | 直接暴露目标机 |
| 接口上的全局地址 | `network_info.result.local_global_addresses[].address` | 高 | 与公网 IP 等价 |
| NAT 证据中的地址 | `nat[].public_ip` / `nat[].local_ip`、`stun_nat` 证据内嵌地址 | 高 | 同上 |
| 文本型字段内嵌地址 | 各 section 的 `detail` / `message` / `error` / `evidence`（IP quality、media、mail、route detail 中常内嵌本机 IP/主机名） | 高 | 路径级脱敏覆盖不到，必须做值级清洗 |
| route hop IP | `route.results[].hops[].ip` | 中 | 属于运营商过渡路由器，不指向用户机器；是线路类型判定的核心证据 |
| ASN / 运营商 / 地区 | `public_ipv4.org/isp/country`、hop `asn`、`classification`、`cidr_neighbors` | 低（**证据本体**） | 这是测评的结论价值所在，任何档案下都保留 |
| catalog 节点 endpoint | route/ping `target`、`resolved_target`、speed node | 公开数据 | 来自版本化 catalog，不脱敏 |

## 3. 脱敏设计（redaction）

### 3.1 三个档案

```
--redact ips      # 默认。隐藏"可被滥用的本机地址"，保留全部结论性证据
--redact strict   # 在 ips 之上再隐藏 route hop IP，只留 hop 的 ASN 标签与 RTT
--redact none     # 原样上传（对应 ECS 行为，给明确知道自己在做什么的用户）
```

| 规则 | `ips`（默认） | `strict` |
|---|---|---|
| `system.hostname` | 置为 `hostname.redacted` | 同左 |
| 公网 IPv4 | 保留前三个 octet：`203.0.113.xxx` | 同左 |
| 公网 IPv6 | 保留 /64 前缀：`2001:db8:a::/64（后段已脱敏）` | 同左 |
| `local_global_addresses[].address` | 公网地址按上述规则；RFC1918/ULA 内网地址保留（NAT 证据需要） | 同左 |
| `nat[].public_ip` / STUN 证据内地址 | 按上述规则 | 同左 |
| route `hops[].ip` | **保留**（过渡路由器 + 证据本体） | 掩码为 `hop.redacted`，保留 `asn` 标签与 `rtt_ms` |
| ASN/运营商/地区/线路分类/邻居估计 | 保留 | 保留 |
| `ip_bgp` 注册网段/RIR/对等 | 保留（网段比 IP 粗，不构成定位） | 保留 |

### 3.2 两级实现，缺一不可

脱敏器（`share/redact.go`）分两层，顺序执行：

1. **路径级**：按上表对结构化字段直接改写。只认字段路径，不做 JSON 全文 regex。
2. **值级清洗**：先从原始报告收集"本机地址集合"（公网 v4/v6、接口全局地址、NAT 地址、hostname），再遍历所有字符串型 `detail/message/error/evidence` 字段做子串替换，替换为与档案一致的掩码形态。IP quality 的 DNSBL 明细、media detail 中的解锁 IP、route 的 detail 行都会在这一层被覆盖。

两层都作用在 **JSON 文档**上；text/HTML 分享物一律从脱敏后的 JSON 派生，保证三种格式内容一致。

### 3.3 脱敏清单（inventory）

脱敏器同时产出 inventory：命中了哪些类别、各多少处。每次 share 必须：

- `--dry-run` 时把 payload 与 inventory 一起打到 stdout（用户上传前可见真实内容）；
- 实际上传后把 inventory 打到 stderr（`uploaded: text 18.2 KiB; redacted: hostname×1, public_ipv4×1, public_ipv6×1, interface×2, embedded×14`）。

这是对"支持脱敏"承诺的可见化：用户不需要信任实现，只需要读清单。

## 4. 分享物格式

```
--format text   # 默认。终端风格等宽文本，直接贴论坛/TG，目标 < 40 KiB
--format json   # 脱敏后的完整 schema-v2 JSON（自动化消费者）
--format html   # 自包含单文件 HTML（当前 report/html.go、suite/html.go 无任何外部资源引用，
                #   脱敏后重渲染即为离线可打开的单文件）
```

- 三种格式共用同一脱敏文档，`share` 只做投影，不产生第二数据源。
- **media 折叠**：`--media summary|full`（默认 `summary`）。200+ 流媒体结果在 text 格式下折叠为"每区域 unlocked/locked/failed 计数 + 仅异常项明细"，避免 paste 被撑爆；`full` 恢复全列。json/html 不折叠（结构性数据与浏览场景能承受）。
- 体积护栏：text ≤ 512 KiB、json ≤ 512 KiB、html ≤ 1 MiB（provider 适配器可声明更低上限）。超限返回结构化错误并给出收敛建议（`--format text`、`--media summary`），**不静默截断**。

### text 版式（对标 ECS 阅读顺序）

```
头部      系统卡：OS/内核/虚拟化/CPU/内存/磁盘 + ASN/运营商/地区 + NAT/STUN 类型（脱敏标注）
硬件      workload 表：median / throughput / latency / iterations
route     每目标一行：节点、线路分类（code/label/confidence）、destination_reached、状态
ping      目标 × latency/jitter/loss/connection_state
speed     provider 分组：download/upload/latency/status（标注 best_per_metric 聚合时）
ip_quality 0-100 风险分 + DNSBL 命中 + 邮件端口结论（fail-closed 时显式"证据不全未出分"）
mail      8 端口 open/refused/timeout/error
reachability 网站/Telegram DC 状态
media     summary 折叠视图
尾部      vmbench 版本/commit、catalog source/revision、UTC 时间、耗时、
          redaction 声明（"本文由 vmbench share 生成，公网 IP 与主机名已脱敏"）
```

## 5. 目的地 provider

### 5.1 候选与取舍

| provider | 鉴权 | 保留期 | 备注 |
|---|---|---|---|
| `dpaste`（dpaste.org API） | 无 | 可设长期 | JSON API，响应含 view_url；**默认推荐** |
| `0x0`（0x0.st） | 无 | 有限（按内容衰减） | multipart 上传，实现简单；不适合长期贴 |
| `paste_rs`（paste.rs） | 无 | 服务方默认 | POST body 即 URL，最简；容量/保留以实现时验证为准 |
| `custom` | 自定义 | 自定义 | `--share-endpoint URL`，面向自建 PrivateBin/sticky 等，是合规与内网场景的保底 |

原则：

- 上限与保留期是**实现时验证的数据**，写进 provider 适配器常量表并随文档发布；本文不预设未经核实的数字。
- 无鉴权第三方 paste 都可能被墙或停服，因此 provider 是枚举 + **逗号序 fallback**（`--share-provider dpaste,0x0`），沿用 `china_isp` 节点依次尝试的既有先例；同一 payload 最多上传成功一次，fallback 只在前一个网络失败/5xx 时发生。
- MCP / 默认行为永不触发上传；`custom` 无 endpoint 时参数错误。

### 5.2 上传实现

- stdlib `net/http`，每 provider 一个适配器（`share/provider.go` 内小适配器即可），无新增第三方依赖（遵守 AGENTS.md）。
- 单次尝试 30s 超时，provider 内**不自动重试**（避免重复 paste），fallback 才换下一家。
- 响应体读取上限 1 MiB（沿用 `traffic_bytes` 的护栏哲学）；仅接受 https；解析出 URL 后立即返回。
- payload 全程在内存；`--save-payload FILE` 可选落盘保留脱敏副本（Unix `0600`，与报告导出同规格）。

## 6. CLI 设计

### 6.1 独立子命令（P1 主体）

```bash
vmbench share suite.json                      # text + ips 脱敏 → 打印链接
vmbench share suite.json --format json --redact none --save-payload clean.json
vmbench share suite.json --format html --provider dpaste,0x0
vmbench share suite.json --dry-run            # 打印脱敏后 payload + inventory，不上传
vmbench share suite.json --provider custom --share-endpoint https://paste.internal/upload
```

| flag | 默认 | 说明 |
|---|---|---|
| `--format` | `text` | `text\|json\|html` |
| `--redact` | `ips` | `ips\|strict\|none` |
| `--media` | `summary` | text 格式的 media 折叠 |
| `--provider` | `dpaste` | 枚举或逗号序 fallback |
| `--share-endpoint` | | `custom` 必填 |
| `--dry-run` | false | 输出 payload + inventory，不联网 |
| `--save-payload` | | 保存脱敏副本（0600） |

输入为 run 或 suite 的 JSON 报告（按 `report_kind` 自动识别，混入非法文件报参数错误）。

### 6.2 run / suite 直通 flag（P1b）

```bash
vmbench suite --preset proxy --json suite.json --share
vmbench run --scope all --json r.json --share --share-format html
```

- `--share` 是 bool，上传格式由 `--share-format`（默认 `text`）决定；`--redact` / `--media` / `--provider` / `--share-endpoint` 语义与子命令一致。
- 报告写出成功后才分享；section 失败的最终报告**仍然分享**（诊断证据有价值，且用户已显式选择），与现有"结构化失败"语义不冲突。
- `--share` 不隐含 `--save-history`，两者正交。

## 7. TUI 设计（P1b）

- 在 Results / SuiteResults 页新增 `h`（share）键：打开确认弹窗（非默认上传）。
- 弹窗字段与 SuiteConfig 同风格：格式 / 脱敏档案 / media 折叠 / provider；提供"预览 payload（dry-run）"动作，先看清单再上传。
- 确认后显示上传状态，成功页展示可复制 URL 与 inventory；失败显示结构化原因与 fallback 结果。
- 80×24 及以下走紧凑布局，规则同现有 Suite 视图。`docs/tui-design.md` 同步。

## 8. MCP 立场

**v1 不提供 `vmbench_share` tool。** 理由：share 的授权主体是"终端用户显式动作"，而 MCP 调用方是 agent，无法证明终端用户对"公网发布本机测评数据"知情同意；现有安全模型（`isError` 语义、枚举校验）不覆盖外发行为。若未来开放，必须满足：默认拒绝、需显式 `allow_share` 能力协商、每次调用返回 dry-run 清单确认。此立场写入 `docs/product.md` 安全边界。

## 9. 错误语义

| 场景 | 行为 |
|---|---|
| 输入不是合法 vmbench JSON | 退出码 2，参数错误 |
| 脱敏失败（理论上不发生，防御性） | 退出码 1，**绝不上传** |
| payload 超限 | 退出码 1，错误含各 provider 上限与收敛建议 |
| 全部 provider 网络失败 | 退出码 1，逐个 provider 的结构化错误；提示 `--dry-run` + `--save-payload` 手动分发 |
| fallback 成功 | stderr 记录前序失败原因，stdout 打最终链接 |
| `--redact none` | 正常执行，stdout/stderr 打印明示警告（当前档案不做任何脱敏） |

## 10. 实现落点

```text
share/
├── redact.go        # 路径级 + 值级脱敏、redaction profile、inventory
├── inventory.go     # 敏感项统计与人类可读清单
├── text.go          # text 投影（含 media 折叠）
├── html.go          # 调 report/suite 现有 HTML 渲染器，输入脱敏文档
├── provider.go      # provider 枚举、适配器、fallback、上限常量
└── *_test.go        # redact/golden/httptest
cmd/vmbench/main.go  # 新增 "share" case（现有 switch 直接扩展）
run/suite            # --share 等 flag 走 config_validation 归一化，与 CLI/TUI/MCP 契约同层
```

依赖增量：0。HTML 分享复用现有渲染器（已确认无外部资源引用，可离线打开）。

## 11. 测试计划

- `redact_test.go`：构造含已知 IP/hostname 的 suite + run 双 fixture，断言三个档案下的字段路径与 detail 内嵌值都被处理，且 ASN/运营商/分类等证据字段原样保留；断言脱敏幂等。
- `text_test.go`：golden 文件锁定 text 版式（summary/full 两种 media）；断言尾部 redaction 声明存在。
- `provider_test.go`：httptest 模拟各 provider 的表单形态、URL 解析、5xx/超时/fallback 顺序、响应体超限。
- `cli_test`：`--dry-run` 不出网；`--provider custom` 缺 endpoint 报参数错误。
- CI 不访问外部网络；真实 provider 连通性作为手动 smoke check 清单写入本文。

## 12. 分阶段交付

| Phase | 内容 |
|---|---|
| P1a | `share` 子命令：redaction（ips/strict/none）+ text/json + dpaste + custom + dry-run + inventory |
| P1b | 0x0/paste_rs 适配器；run/suite `--share` 直通；html 单文件分享；TUI `h` 键与确认弹窗 |
| P2 | `--template forum`（论坛/中文模板，与"中文输出"P1 合并交付）；`vmbench history share <id>` |
| P3 | MCP share tool（按 §8 前提重新评估）；端到端加密 provider（如 hemmelig 类） |

每个 phase 合入时同步：README（命令表 + flags + share 小节）、`docs/product.md`（使用方式与安全边界）、`docs/tech-stack.md`（share 包与 provider 表）、`docs/tui-design.md`（P1b）、`docs/CHANGELOG.md`。

## 13. 决策记录与开放问题

| 问题 | 推荐决策 | 理由 |
|---|---|---|
| 默认脱敏档案 | `ips` | ECS 默认全裸是它的历史负担；"默认安全 + 可选放开"是差异化机会，也兑现 product.md 承诺 |
| 默认格式 | `text` | 论场场景优先；json/html 面向自动化与浏览，显式选择 |
| 默认 provider | `dpaste`（fallback 由用户加） | 无鉴权 + JSON API + 可设长期保留；中国大陆可达性在实现时实测，不达标则换默认并在文档注明 |
| text 是否默认折叠 media | 是（summary） | 200+ 行会撑爆 paste 与阅读耐心；异常项才是分享时的信息密度 |
| section 失败的报告是否分享 | 是 | 用户显式选择 `--share` 即授权；失败证据正是排查所需 |
| share 事件是否入 history | v1 否 | stderr inventory 已可追溯；避免 history 语义膨胀 |
| 是否上传原始 JSON | 仅 `--format json` 显式 | 原始 JSON 是全量数据，默认面必须是脱敏投影 |

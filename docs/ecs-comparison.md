# vmbench vs ECS/GoECS 差异快照

> FEAT-001：对标 <https://github.com/spiritLHLS/ecs>，输出当前差别。
> 快照日期：2026-07-16。
> 本文是产品差异快照，不是 benchmark 结果，不产生总分。

## 来源

- ECS legacy shell：<https://github.com/spiritLHLS/ecs>
- GoECS current：<https://github.com/oneclickvirt/ecs>
- vmbench 当前仓库：`/root/github/vmbench`

## 命令

```bash
vmbench ecs-diff
vmbench ecs-diff --json
```

## 图例

| Status | 含义 |
|---|---|
| `aligned` | 核心能力已对齐 |
| `partial` | 已有基础能力，但 ECS 覆盖更广或细节更深 |
| `gap` | vmbench 当前没有对应能力 |
| `vmbench_only` | vmbench 当前具备而 ECS 不是重点的能力 |

## 差异矩阵

| 维度 | Status | vmbench 当前能力 | ECS/GoECS 当前能力 | 差异与下一步 |
|---|---|---|---|---|
| 项目形态 | `partial` | Go CLI + Bubble Tea TUI，Console/JSON/HTML 输出，强调原始指标与结构化报告 | shell 版 + 当前 GoECS，提供交互菜单、参数模式、非 root、离线包与 Docker 运行 | ECS 的部署入口更丰富；vmbench 保留 Go/TUI 路线，后续补安装脚本、Docker 或离线包 |
| 系统信息 | `aligned` | 跨平台硬件 sysinfo + virtualization system/role；Suite `network_info` 输出公网 IPv4/IPv6、ASN/provider/location 与保守 NAT 证据 | 基础信息、IP、ASN、虚拟化、NAT、IPv4/IPv6 与安全上下文 | 核心证据已对齐；vmbench 不伪装成完整 cone-NAT 分类，只输出 direct/translated/unknown |
| 硬件基准 | `partial` | 默认 sysbench/fio/openssl；可选 dd、STREAM、mbw、Geekbench、WinSAT；缺工具结构化 error，不做进程内 fallback | sysbench、geekbench、winsat、stream、mbw、dd、fio 等 | 核心工具覆盖已接近 ECS；后续差距在自动安装、离线包、多盘/多模式细节 |
| 路由诊断 | `aligned` | 版本化 catalog 覆盖广州/北京/上海/成都、三网、CERNET、CSTNET、IPv4/IPv6，并保留稳定 node ID/revision/hops/error | BackTrace/NextTrace 覆盖电信、联通、移动、教育网、科技网、多地区与 IPv4/IPv6 | 核心线路维度对齐；ECS 生态节点仍更广，vmbench 强项是 revision 可追踪 |
| Ping 延迟 | `partial` | 同一 versioned catalog 上的 TCP latency/jitter/loss，支持 v4/v6/dual 和 node identity | 更多节点和运营商维度的 pingtest 组合 | 证据协议与节点追踪更严格，长期运营节点规模仍小于 ECS 生态 |
| 速度测试 | `partial` | Cloudflare、Speedtest.net、Speedtest.cn、iperf3 provider，download 节点进入 embedded/auto/path catalog，输出 groups/providers/summary | Speedtest.net/cn 多节点、节点自动更新、指定运营商/地区、结果上传 | 已支持 signed catalog update/health/revision pin；仍缺公开运营更新源和分享上传链路 |
| IP 质量 | `partial` | IP 基础信息、DNSBL、邮件端口和 0-100 风险诊断 | 更多 IP 数据库，覆盖 IPv4/IPv6、邮件、欺诈/安全、地区与 ASN 诊断 | 第三方数据库覆盖深度不足；按 provider 接入更多数据库 |
| 流媒体解锁 | `partial` | Netflix、YouTube、Disney+、ChatGPT、TikTok、Prime 等常见探测 | UnlockTests/TikTok 等更完整地区化解锁脚本 | 有基础模型；后续补区域化平台清单 |
| 网站/TG 可达性 | `aligned` | 独立 `reachability` section，逐项输出 website HTTPS、Telegram DC TCP、latency/status/HTTP status/error | 常见网站访问、Twitter 地区、Telegram DC 等网络可用性检查 | 核心可达性证据已对齐；Twitter 地区等区域化目标仍可按数据 catalog 扩展 |
| 报告与分享 | `partial` | Console/JSON/Suite v2/完整 HTML、原子本地 history（Unix `0700/0600`）、`--save-history` 与 compatible-only compare | 终端结果、goecs.txt/test_result.txt、分享链接和上传通道 | vmbench 本地复现和机器可读更强；ECS 分享传播更强，后续增加显式 consent/redaction 的 upload/share |
| 交互体验 | `vmbench_only` | 现代 TUI、主题、Suite 配置页、运行页、结果页和 Compare 页 | 交互菜单成熟，但 GUI 不是新版 GoECS 重点 | vmbench 终端 UI 是差异化优势，继续落在 TUI Suite |
| 结果对比 | `vmbench_only` | 自动识别 benchmark/Suite；支持两份以上和 history `--last N`，仅在 unit/protocol/provider/node/catalog revision 兼容时计算 delta | 重点是一键测评与分享，非多份结构化报告的兼容性对比 | 继续保持证据可比性优势，不用综合评分替代原始 delta |
| 评分策略 | `aligned` | 不输出 benchmark 总分、等级或 category score；IP 风险诊断独立保留 | 以各工具/模块的测量输出为主，可包含上游工具自己的结果 | 对标时不把 ECS 单项结果归一化成 vmbench 总分 |

## 建议优先级

### P1

1. 运营公开签名 catalog：建立 revision 发布、健康度、退役和流量预算流程，而不是继续扩展硬编码节点。
2. 扩充 IP 数据 provider 与区域化 reachability/media 目标，同时保持 fail-closed 和 provider provenance。

### P2

1. 增加显式授权、默认脱敏的可选结果 upload/share：补齐 ECS 的传播能力，不破坏本地 JSON/HTML 可复现输出。
2. 强化硬件工具分发与平台适配：补自动安装/离线包、多盘/多模式细节；缺工具时继续结构化记录错误。

## 边界

- 不复制 GPL 项目代码，只参考功能设计。
- 不重新引入 benchmark 综合评分。
- 网络类探测在受限环境失败时，继续结构化记录 `detail/error`。

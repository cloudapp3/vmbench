# VMBench Changelog

## v0.13.1（2026-09-10）

### sysinfo 硬件证据展示接线

- **背景**：v0.13.0 落库的硬件证据（超售信号、主网卡、缓存层级）只进 JSON，屏幕上看不到。本版本把三组证据接入全部展示面，取值收敛在 `sysinfo/present.go`，无新采集。
- **共享取值**：`OversellSignals()` 增补 `State` 原始证据词并新增 `OversellSignalsText()`（`balloon=present (!) / ksm=disabled` 一行文本，`(!)` 标记买家风险）；`NetworkInfo.PrimaryNIC()` 渲染 `virtio_net (1af4:1000)`；复用 `FormatCacheLine`。
- **五个展示面**：benchmark console 系统头三个条件行（checkup console 经 hardware 段复用自动继承）；benchmark HTML sys-cards 下一行 muted 证据行；checkup HTML 系统信息区（超售/网卡 metric + badge 分级：Risk=warn、Off=ok reassurance，CPU metric small 追加缓存行）；TUI dashboard 系统卡与详情卡；TUI compare 报告卡（两机对比超售差异）。
- **零证据即零渲染**：裸机无 balloon/KSM 证据 → 整行消失（不显示假安心）；无设备 backed 网卡、无缓存证据同样跳过。CPU `stepping` 仅入 JSON 不接显示（裸数字买家无从解读）；内存运行态 used/available 暂不接显示（采集时点语义需专门文案，待后续版本）。
- **测试**：`sysinfo/present_test.go`（State 与 On/Risk 一致性、PrimaryNIC 五形态、缓存排序与分数渲染）、`report/render_test.go`（console/HTML 正反向）、`tui/dashboard_render_test.go`（dashboard/compare 卡正反向）。

## v0.13.0（2026-09-10）

### 报告脱敏：本机公网 IP 默认替换为文档保留段地址

- **背景**：体检报告在多处原样记录本机公网 IP（结构化身份字段、IP quality 自由文本证据、BGP/CIDR 网段、反向 DNSBL 标签、内嵌 hardware Document），而报告会以 JSON/HTML 落盘、写入 history、经 TUI 展示并整体返回给 MCP 客户端——分享即泄露。
- **新包 `redact/`**：泛型 JSON 字节级替换——`Marshal → 三次有序替换 → Unmarshal 回原类型`。Pass A 网段/范围（仅当网段数学上包含某本机 IP 才替换，v4 长度保持 /24 下限、v6 /64 下限）、Pass B 反向 DNSBL 标签（`d.c.b.a.` 带尾点）、Pass C 带边界保护的裸 IP（URL、`::ffff:` 映射、相邻部分前缀均可正确处理）。泄漏面是长尾，字节级对未来新增字段自动覆盖。
- **掩码形态**：IPv4 → `203.0.113.1,2,…`（RFC 5737 TEST-NET-3），IPv6 → `2001:db8::1,2,…`（RFC 3849），同一 IP 在一份报告内映射一致；文档保留段自身不脱敏 ⇒ 幂等。内网 IP、hostname、路由 hop、远端节点 IP 保留（不在本机地址集合内）。
- **双入口收口**：`checkup.Run` 与 `vmbench.RunCore` 各自在产出报告前脱敏一次，天然覆盖 CLI JSON/HTML/console、`--save-history`、TUI 卡片与导出、MCP structuredContent、内嵌 run Document；嵌套 RunCore 传 `none` 由外层统一处理。checkup 侧 fail-closed：脱敏失败时丢弃网络身份与 IP quality 证据并告警，绝不返回明文。
- **开关**：默认脱敏；CLI `--redact none` 逃生舱（stderr 打分享警告）；TUI 与 MCP 恒为脱敏。非法值退出码 2；en/zh-CN 双语 i18n。Go API `Options.Redact` 零值即默认脱敏（fail-safe）。
- **测试**：`redact/redact_test.go` 覆盖映射一致性、三类 pass 形态、边界保护、幂等、往返完整性；`checkup/redact_test.go` 用含全部泄漏面的 fixture 断言脱敏后无明文、指标无损；CLI 校验测试（`--redact bogus` → 2）。

### 体检 route 展示：ECS 式具体线路

- **线路成为第一公民**：route 部分三个输出面（TUI 结果卡 / console / HTML）统一展示回程具体线路（如 `电信CN2GIA [精品线路]`）。取值优先保守分类 `classification.label`（清理对齐空格），分类缺失或 inconclusive 时回退翻译 `observed_asns`（含 AS4809+AS4134→CN2GT、单 AS4809→CN2GIA 消歧，沿用上游 ECS 标签表），两者皆无显示本地化的「未知线路」。
- **色调分级共享**：新增 `checkup.RouteLineText` / `RouteLineTone` 供三个输出面共用——精品/优质线路为成功色、163/4837/CMI 等普通干线为中性色、证据不足为警示色；HTML route 摘要表 badge 按分级着色，逐跳表中命中中国骨干网 ASN 的行加高亮 badge。
- **console 主表收敛**：route 表从 9 列收敛为 `目标/解析IP/线路/置信度/状态` 5 列；未到达以 `partial (unreached)` 并入状态列，probe/hops 等探测出处仍在 HTML 与 JSON 证据中保留。
- **测试修复**：catalog 两个工具预检测试补 `XDG_CACHE_HOME` 隔离（同 v0.11.0 对 `cmd/vmbench` 的修复）——开发机已 fetch 过 pinned fio 时 `resolveTool` 缓存回退会掩盖 missing 证据，导致 `TestMissingHardwareToolsForFilterOnlyChecksMatchingAdapters` 误报。

### Ping：TCP 全静默目标自动 ICMP echo fallback

- **背景**：catalog 中 `*.endpoint.nxtrace.org` 等 traceroute 端点对 TCP SYN 静默丢包（ACL drop 而非 reject），ping 部分长期出现成片 `error no_response`——目标是活的，只是不回 TCP。独立出口复现 + ICMP 反证确认这是目标策略而非路径问题。
- **fallback 语义**：10 次 TCP 探测全部静默（0 open、0 RST）且未取消时，追加一次系统 `ping`（`bench/netio/ping_icmp.go`；Linux `-c/-w`、macOS `-c/-t`/`ping6`、Windows `-n/-w`，沿用 traceroute 的 shell-out 模式，无需 root）。ICMP 有回包则该行改记 `status=ok`、`probe_protocol=icmp-echo`、`probe_tool=system-ping`，延迟/抖动/loss 来自 ICMP 证据，`connection_state` 留空；ping 缺失或 ICMP 也无响应时维持原 TCP error 不受影响。
- **跨 locale 解析**：逐次 RTT 只取带 `ttl=` 标记的回包行中的 `time=10.7 ms` / `时间=338ms` / `time<1ms`，天然跳过 rtt min/avg/max/mdev 与 Windows 往返均值等汇总行（含中文 Windows）。
- **展示**：TUI ping 卡对 icmp-echo 行追加 `icmp` 标记并按警示色（同 refused/mixed）着色——TCP 死、ICMP 活属于降级路径；HTML 报告经既有 `probe … via …` 列自然呈现；compare 逐行沿用 `probe_protocol`，section 级标签更新为 `tcp-connect+icmp-echo`。
- **范围**：RST/refused、mixed、DNS 失败等路径不触发 fallback；`vmbench-rs` 尚未同步（见 `docs/rust-port.md`）。

### Node catalog `auto`：每次自动拉取 + 静默回退（借鉴 ECS 分发模式）

- **背景**：`auto` 此前从不联网，只读本地缓存，而缓存只有手动 `vmbench nodes update` 才会写入——对多数用户 auto 等价于 embedded，节点数据随版本定死。上游（speedtest.cn-CN-ID 等）每日更新，快照却停在发布日。
- **新语义**：`auto` 每次 load 都 best-effort 拉取 manifest（`nodecatalog/remote.go`）：镜像链顺序尝试（raw.githubusercontent → jsDelivr → gh-proxy → spiritlhl CDN），共享 5s 超时、非最后镜像 2s 软帽；每个候选必须通过 HTTPS fetch → strict schema → revision pin 三关。成功即用（source 记为 `remote`）并把原文原子写入缓存作离线兜底。
- **静默回退**：一切拉取失败（DNS/超时/5xx/镜像不可达/schema 无效/pin 不匹配）回退 cache→embedded，不产生任何 warning——离线不该唠叨；仅成功后缓存写失败与 manifest 过期会告警。pin 不匹配的远程 revision 不覆盖已钉住的缓存。
- **信任模型**：与 ECS 融合怪同模式——信任根是 HTTPS（TLS）+ 严格 schema decoder，不涉及签名密钥；镜像链只是分发端点，前缀代理即使被替换内容，影响面也仅限测试目标指向（无代码执行、无凭据泄露）。镜像链为单个 string var（`DefaultManifestURLs`），可用 `-ldflags -X` 整体替换，增删镜像无需发版。需要端到端强保证时仍可用 `nodes update`：显式提供 Ed25519 trust root 与 detached signature，验签通过才原子写入缓存。
- **数据保鲜**：仓库 `nodecatalog/nodes.json` 即 live manifest（raw URL 直接服务仓库文件），维护者用 `scripts/gen_isp_nodes.go` 再生成、审查、bump revision 后直接提交；embedded 快照随 `go:embed` 同一提交更新。无需密钥仪式。
- **测试**：`nodecatalog/remote_test.go` 九个用例覆盖拉取写缓存、断网静默回退（cache/embedded）、schema 无效静默跳过、镜像链降级、pin 保留、过期、写失败、并发安全；`TestMain` 保证测试二进制永不触网。

### sysinfo 硬件证据数据层（additive，展示面后续接线）

- **内存运行态**：`MemoryInfo` 增补 `used_bytes` / `available_bytes` / `used_percent`（Linux/macOS/Windows 采集，`omitempty`）——是 best-effort 运行时状态而非容量，渲染层须把零值当 unknown 而非空。
- **CPU/网卡证据**：`CPUInfo` 增补 `stepping`；`NetworkInfo` 增补 `primary_driver` / `primary_pci`（首个物理网卡的驱动与 PCI ID，如 `virtio_net` / `1af4:1000`，无设备 backed 接口时留空）。
- **展示辅助（`sysinfo/present.go`）**：`PlatformDiagnostics.OversellSignals()` 把已有的 balloon/KSM 证据映射为买家视角超售信号（On=能力开启、Risk=对买家意味着超售暴露，证据未知则省略）；`FormatCacheLine` 以固定 L1d/L1i/L2/L3 顺序渲染缓存行。均为纯函数，本版本仅入库未接显示（v0.13.1 接线）。

## v0.12.0（2026-09-10）

### 派生评估层：`vmbench score`（政策反转）

- **政策反转**：vmbench 此前承诺"不输出总分/等级"（原始指标优先）。本版本在保持原始指标为唯一事实来源的前提下，新增**确定性派生评估层**：`vmbench score <report.json|->` 基于版本化基线生成 assessment（综合 index/rating、五维度明细、web/build/proxy/storage 场景适配、覆盖率披露）。`Evaluate` 是纯函数——同一报告 + 同一基线 ⇒ 字节级相同输出；评分路径无网络、无时钟。
- **覆盖率不撒谎**：期望指标集按报告自身 config（hardware_tools）∩ requires_kind ∩ platform 计算，optional 指标缺失不拉低覆盖率；CPU 维度无数据或性能维度 <2 有数据时综合分置空并给出 warning；部分维度缺失时 reweight 并在 `basis`/`excluded` 显式披露。legacy suite 报告与 checkup 同样可评。
- **基线数据**：内嵌 `score/baselines.json`（schema_version 1，revision `2026-09.1`）：log 曲线归一化吞吐、固定阈值带归一化延迟/丢包，严格单位纪律（MiB/s ≠ MB/s）；`--baseline` 换外部基线、`--baseline-rev` 钉版本（不匹配即失败）。锚点为知情占位值，待真实 VPS 语料校准。设计细节见 `docs/score-design.md`。
- **一票否决**：场景 profile 的 veto 触发时仅封顶评级至 C（不动 index）；fio Q1 延迟与 Q32 IOPS 各有专属否决线。Ping 平均延迟是地理属性，不入综合分，仅用于 proxy 场景否决。

### 数据层（additive，schema v2 兼容）

- **fio 尾延迟**：新增 `latency_p99_ns` 字段（`ResultEntry` / `RunDetail`，omitempty），fio workload 解析 clat percentile p99；报告各展示面（JSON/HTML/console/TUI/compare）在 p99>0 时并列显示。
- **CPU steal 探针**：新增 workload `CPU Steal (/proc/stat)`（Linux-only，无外部工具依赖，默认 5s 采样，`--filter 'Steal'` 可选/排除），fail-closed：计数器不前进、回退或格式变化即报错。
- **MCP 政策串**：capabilities policy 从"不输出总分/等级"改为"派生评估走 `vmbench score` CLI，MCP 本身不评分"。

## v0.11.0（2026-09-09）

### "suite" 全面更名为 "checkup"（BREAKING）

- **报告种类与组合概念更名**：v0.8.0 合并进根命令的组合测评概念由 "suite" 更名为 **checkup**（中文「体检」）。报告种类、TUI 页面（CheckupResults）、事件（`checkup_start` → `checkup_done` 等）、MCP capabilities 键（`checkup_sections` / `checkup_presets`）与全部双语文档同步更名；tagline 的 "benchmark suite" 属工具集含义，改为 "benchmark toolkit"（中文「工具集」），与报告种类无关。
- **`report_kind` 线格式**：体检报告现在写 `"report_kind": "checkup"`；读取侧兼容旧值 `"suite"`——`history add` / `compare` / 历史记录继续接受 v0.10.0 及更早版本的报告，历史记录里的旧 `kind: "suite"` 读取时归一化为 checkup，旧 run 记录不受影响。
- **MCP `vmbench_suite` 弃用别名删除**（兑现 v0.8.0 公告）：`vmbench_run` 是唯一基准工具；旧客户端调用 `vmbench_suite` 会得到 unknown tool 错误。
- **Go API（BREAKING）**：包路径 `suite/` → `checkup/`、`suitecompare/` → `checkupcompare/`，导出符号同步更名（`checkup.Run` / `checkup.Options` / `checkup.CheckupReport` / `history.KindCheckup` / `vmbench.EventCheckupStart` 等），旧 `Suite*` 标识符不再存在。
- **顺带修复**：`install.sh` 安装完成的示例命令仍指向 v0.8.0 已删除的 `vmbench suite` 子命令（执行会以 exit 2 退出），改为根命令用法；`cmd/vmbench` 工具预检测试补上 `XDG_CACHE_HOME` 重定向，开发机已 fetch 过静态工具时测试不再误报。

## v0.10.0（2026-09-09）

### `vmbench uninstall`：二进制自卸载

- **新增 `vmbench uninstall` 子命令**：打印删除计划（历史报告条数、已获取的静态工具、TUI 偏好、数据目录、二进制本体）→ 终端确认 → 按「目录 → 二进制」顺序删除；任一项失败即保留二进制，卸载可安全重跑。`--dry-run` 预览、`--yes` 跳过确认、`--json` 输出结构化计划/结果，en/zh-CN 双语文案。
- **职责边界与 `install.sh` 委托协议配套**：二进制负责文件清理（数据/配置/工具缓存/本体），shell 侧负责 `# vmbench user install` PATH 条目与 systemd/launchd 服务停止；`install.sh --uninstall` 探测到子命令即委托并在委托前停服务，无终端时（如 `curl | bash`）非交互直接执行。自定义 `VMBENCH_HISTORY_DIR` 与 `VMBENCH_CONFIG` 重定向位置一律保留。
- **安全护栏**：系统根目录（`/`、`/etc`、`/usr` 等）与 `$HOME` 拒绝删除；symlink、非目录、非常规文件拒绝；删除前逐项重验（TOCTOU）；dpkg/rpm 管理的二进制警告改用包管理器卸载；手动 service unit 检测提示。`tools fetch` 的静态工具缓存（`~/.cache/vmbench`）纳入清理（shell fallback 此前不清理该目录）。
- **新增顶层 `uninstall` 包**（Plan/Execute/guards，`history.DefaultRoot`、`tui.ConfigPaths` 为配套导出）。

### 文档

- README 移除「自定义目录」`--dir /opt/bin` 示例，对齐 vmflow 安装引导：显式安装只展示 `--system`，`--dir PATH` 保留在 flags 说明中并注明「显式目录不改启动文件，需自行确保在 `PATH`」。`docs/capabilities.md` 同步、`vmbench-rs/install.sh` 示例改用 `/usr/local/bin`；默认目录策略不变（root `/usr/local/bin`，用户 `~/.local/bin`/`~/bin`）。

## v0.9.0（2026-09-09）

### TUI 配置页改为 ECS 式勾选清单（BREAKING）

- 统一"运行评测"配置页重构为垂直勾选清单：首行「开始评测」光标默认停留，进入页面直接回车即跑；9 个测试项一行一项、默认全勾，`空格`/`回车` 勾选、`1-9` 数字键快切，开始行实时显示启用项数与按历史均值估算的总时长。
- **默认行为变化（BREAKING）**：默认全勾意味着 TUI 直接回车产出 suite 全量报告（此前默认"仅硬件" run 报告）；缩小范围取消勾选即可。
- 高级参数（迭代次数 / IP 版本 / 超时 / 节点目录）收进底部可展开的「高级设置」行，`←→` 循环改值；硬件工具、workload 过滤、speed/route/media/IP 来源等细节卡片移除，自动采用与 CLI/MCP 相同的文档化默认值。
- 移除随之失效的 suite 汇总/渲染死代码与旧测试（净约 1000 行）。

### `vmbench tools`：opt-in 静态工具二进制分发

- **新增 `vmbench tools` 子命令**（`status` / `fetch`）：解决"系统没装 fio/sysbench → 磁盘/CPU/内存 workload 全部结构化报错"的开箱体验。`fetch` 从本仓库稳定 `tools` release tag 的独立资产下载 pin 版静态构建（fio 3.39、sysbench 1.0.20，Linux amd64/arm64），流式下载按 `toolbin` 包编译期 SHA-256 pin 校验（信任根 = TLS + 源码 pin，独立于每次 release 的 checksums.txt），不匹配 fail-closed 不落盘；原子安装到用户缓存目录 `~/.cache/vmbench/binaries/`（`--dest` 可覆盖，`--url` 支持镜像），不改动系统软件包。`status` 列出全部硬件工具的解析路径。
- **工具解析顺序扩展**：PATH → 可执行文件相邻 `binaries/`（管理员放置，优先）→ 用户缓存目录（fetch 产物）。政策不变：主 release 包仍不内置第三方二进制、不搜索当前工作目录、缺工具仍进结构化 `error` 不回退进程内算法。
- **预检提示**：CLI 预检在 apt 提示后新增 `vmbench tools fetch ...` 一行提示（en/zh-CN）。
- **构建管道**：`scripts/build-tools.sh`（Alpine musl 静态构建 + 静态断言 + sha256 输出）与 `.github/workflows/tools-release.yml`（手动触发，原生 amd64/arm64 runner 构建，资产覆盖上传到 `tools` tag）。重 pin 需重建资产并同步更新 `toolbin` 哈希。GPLv2 源码提供义务与许可证表见新增 `docs/THIRD-PARTY.md`。
- 顺带修正历史遗留：仓库 `binaries/sysbench_x64` 为 musl 动态链接，在 glibc 系统不可执行；官方静态资产即为替代。

## v0.8.0（2026-09-08）

### run/suite 合并进根命令（BREAKING）

- **跑测评不再需要二级命令**：`run` / `suite` 子命令删除，全部基准 flag（合并后 23 个，名称不变）提升到根级。迁移示例：
  - `vmbench run` → `vmbench`
  - `vmbench run --filter 'SHA|AES' --json r.json` → `vmbench --filter 'SHA|AES' --json r.json`
  - `vmbench suite --preset quick` → `vmbench --preset quick`
  - `vmbench suite --only ping,mail` → `vmbench --only ping,mail`
- **默认只跑 hardware**：非交互调用不带 `--preset` / `--only` / `--skip` 时只跑硬件基准（兼容旧 `vmbench run`）；网络 section 需要 preset 或 `--only` 显式选择。`vmbench --lang zh-CN`（唯一显式 flag 是 `--lang`）打开对应语言的 TUI 而不开跑基准。
- **报告种类规则**（一条规则三处复用，CLI / TUI / MCP 共用）：解析 preset/only/skip 后，生效 section 恰好只有 `hardware` → 走 run 路径（`vmbench.RunCore` → run 报告 / KindRun），否则走 suite 路径（`suite.Run` → suite 报告 / KindSuite）。run 报告与旧 `vmbench run` 字节级兼容，history / compare 可与旧 run 记录配对。**语义变化**：旧 `vmbench suite --only hardware` 产出 KindSuite，新 `vmbench --only hardware` 产出 KindRun——依赖 report kind 的 history / compare 流程需要注意；`--preset` 等多 section 组合不受影响。
- **旧命令入口**：`vmbench run` / `vmbench suite` 现在输出迁移提示（指向根级用法与 `--help`）并以 exit 2 退出，不做 flag 转发。根命令新增拒绝多余位置参数（旧 run/suite 静默忽略）。
- **TUI 统一配置页**：Dashboard 的 run/suite 两个入口合并为一个"运行评测"，进入单一配置页——preset 胶囊首位是"仅硬件"（与 CLI 默认一致）+ Custom + 4 个 suite preset，9 个 section 开关与硬件工具 / workload 过滤 / speed / route / media / IP 来源等细节卡片按开关状态按需展开，焦点自动吸附可见字段；启动时按报告种类规则分流。**顺带修复 bug**：旧硬件配置页启动后从不切换页面，进度视图与取消弹窗实际不可达。
- **TUI 单一运行页**：Suite 运行页并入统一运行页，按 runKind 分流渲染 workload 网格（run）或 section 网格（suite）；取消弹窗按 kind 选文案；结果页保持分开（run 报告与 suite 报告形状不同）。TUI 偏好文件删除死字段 `last_mode` / `last_engine`（旧 config.json 的多余 key 读取时被忽略）。
- **MCP 合并**：`vmbench_run` 暴露完整基准面（preset / only / skip / 硬件工具 / catalog 选项等 17 参数并集），用同一报告种类规则分流；`vmbench_suite` 保留为弃用别名——路由到同一 handler，schema 完全一致，Title 标记 Deprecated，计划 v0.9.0 删除。

### install.sh 对齐 vmflow 安装器（PATH 处理 / --system / --uninstall）

- **PATH 处理**：安装目录自动选择改为复用已有安装（`~/.local/bin`、`~/bin`、`/usr/local/bin`）→ root 用 `/usr/local/bin` → 普通用户优先已在 `PATH` 中的 `~/.local/bin`/`~/bin`；自动装到主目录且该目录不在 `PATH` 时，向 `.zshrc`/`.bashrc`/`.profile` 幂等追加 `# vmbench user install` + `export PATH=...` 块并打印 reload 命令。此前装到不在 PATH 的目录（如 `--dir /opt/bin`）只会打印绝对路径提示，随后必然 `command not found`。显式 `--dir`/`VMBENCH_INSTALL_DIR` 绝不改启动文件，只打印 PATH 警告与精确的 export 提示；`--no-modify-path`/`VMBENCH_NO_MODIFY_PATH` 可整体关闭启动文件修改。
- **`--system`**：系统级安装到 `/usr/local/bin`（或显式 `--dir`）。root 直接安装；非 root 先验证 sudo（`VMBENCH_SUDO` 可指定绝对路径）再下载，sudo 仅用于目标目录检查（test/mkdir 固定绝对路径）与 `/usr/bin/install` 写入。
- **`--uninstall`**：优先委托 `vmbench uninstall` 子命令（向前兼容）；否则 shell 兜底——定位二进制（--dir → PATH → 常见目录）、stop/verify 手动创建的 `vmbench` systemd/launchd unit（stop 失败即 fail-closed，不动任何文件）、删除二进制与平台数据目录（Linux `${XDG_DATA_HOME:-~/.local/share}/vmbench`，macOS `~/Library/Application Support/vmbench`，含本地历史报告）、TUI 偏好配置目录（Linux `${XDG_CONFIG_HOME:-~/.config}/vmbench`，macOS 与数据目录同级；拒绝符号链接/非目录路径），并从启动文件精确移除自己写入的 PATH 块（awk 逐块匹配，用户手写行与其他安装的块保留）。终端交互确认；`curl | bash` 管道下跳过确认。
- **加固**（与 vmflow 同款）：`tar --no-same-owner`（root 解压不恢复归档 uid/gid）；目标路径为符号链接/非普通文件时拒绝覆盖；`--version`/`--dir` 值校验（含 `=` 形式与 env 变量，全部在任何下载动作之前）。
- README / zh-CN README 同步：Quick Start 换成 `--print-install-dir` + `export PATH` 一行式；Install 节重写目录选择与新 flags；新增 Uninstall 节。install_test.go 从 2 个测试扩到 21 个（fake curl / fake uid / fake sudo fixture；PATH、--system、目录复用、参数校验、卸载全覆盖）。

## v0.7.0（2026-09-08）

### CLI 命令面精简（BREAKING）

- **`run` 回归纯硬件基准**：删除 `--scope` / `--iperf-host` / `--node-catalog` / `--node-revision` / `--node-cache`。`vmbench run` 只编排外部工具硬件 workload，网络诊断（route / ping / speed / IP 质量 / mail / media 等）全部由 `vmbench suite` 提供；`suite` 侧对应参数不受影响。迁移：`vmbench run --scope network|all` → `vmbench suite --only ...` 或 `--preset`。
- **删除 `--mode`**（run）：纯 legacy 参数，删除前已归一化为 `single` 并只发兼容 warning。
- **删除 suite 的 9 个 `--no-*` 开关**（`--no-hardware/--no-network-info/--no-route/--no-ping/--no-speed/--no-ip-quality/--no-reachability/--no-mail/--no-media`）：与 `--skip` 语义完全重叠。迁移：`--no X` → `--skip X`。
- **MCP `vmbench_run`** 同步删除 `mode` / `scope` / `iperf_hosts` / `catalog_source` / `catalog_revision` / `catalog_cache_path` 参数（`vmbench_suite` 不变）；参数解码不拒绝未知字段，旧客户端传入已删参数会被忽略并得到默认的硬件基准行为。
- 报告兼容：benchmark JSON schema v2 的 `mode` / `scope` / `iperf_hosts` / `catalog_source` / `catalog_revision` / `node_ids` 字段保留（`compare` / `history` 继续解析旧网络报告并参与可比性警告）；新 `run` 报告固定 `scope="hardware"`、`extensions=false`，不再输出 `mode` 与网络 provenance 字段。
- flag 计数：`suite` 33 → 24，`run` 17 → 12；README/Common Flags 表与 docs 全量同步。

### 自升级命令

- **`vmbench update`**：从 GitHub Releases 自升级——查询最新 release、与当前版本比较、下载对应 OS/arch 归档、按 release `checksums.txt` 做 SHA-256 校验、解出二进制后以临时文件 + rename 原地替换（Windows 先移 `.old`）。`--check` 仅检查，`--version TAG` 固定/降级版本，`--json` 输出结构化状态，`--dest` 指定目标路径；`GITHUB_TOKEN`/`GH_TOKEN` 作为 API bearer token。校验模型与 `install.sh` 一致（checksums over TLS）。新增 `selfupdate/` 包（纯标准库）与 en/zh-CN 成对 i18n；README 与 docs 同步更新。

## v0.6.0（2026-09-07）

### TUI 人性化重构（滚动 / 帮助 / 鼠标 / 配置页 / 对比重做 / 详情页）

- **全页滚动**（`tui/scroll.go`）：中央裁剪实现，每页内容按 offset 切片，`PgUp/PgDn`、`Home/End`、鼠标滚轮统一驱动；页面切换与 resize 自动归零；每行做 ANSI 感知宽度守卫（`i18n.TruncateStyled`），80 列终端长行不再软折行撑破视口。footer 在内容可滚时显示 `n/m` 位置提示，光标移动后最小滚动保持焦点行可见。此前 Suite 结果 9 张卡片在 24 行终端下溢出屏幕且无法查看。
- **帮助页与提示单一事实源**（`tui/help.go`）：`?` 打开当前页优先的按键参考页；footer 提示与帮助页共用 `helpFor()` 注册表；文本输入聚焦（SuiteConfig Advanced / RunConfig Custom 过滤 / Picker 手输）时抑制 `?`/`q` 全局键，避免按键被吞进输入框或误退出。删除死代码 `tui/keys.go`、`tui/styles.go` 与 results.go 中不可达的保存提示分支。
- **鼠标**（`tui/mouse.go`）：滚轮全页滚动；Dashboard 菜单行点击 = 选中 + 确认，主题行点击切换主题。此前启用了 cell-motion 模式却零处理，滚轮事件被吞。
- **RunConfig 新页**（`tui/run_config.go`）：硬件跑分先配置再运行——iterations（1-9）、硬件工具多选、过滤 chips（All/CPU/Disk/Memory/Custom 正则手输），实时 "N workloads planned" 计数与异步缺失工具预检（warning 非阻塞，对齐 CLI）。修复幽灵行 bug：Running 页预填的 workload 列表与 runner 实际执行集合同源（工具×过滤镜像），不再出现永远 waiting 的行；此前硬编码 `iterations=3` 且不过滤。
- **SuiteConfig 摘要卡**（`tui/suite_summary.go`）：启用 section 数、节点目录规模、计划 workload 数、预计总时长（优先近 10 条历史记录的 per-section 墙钟均值，无历史回退静态粗估，hardware 随 iterations 缩放）；统计经 `catalogStatsMsg`/`historyStatsMsg` 异步加载，View 无 IO。新增 `1-9` 数字键快切 section（Advanced 输入态忽略）。
- **Running 反馈**：ETA（首个 workload 完成后按墙钟均值外推剩余，不再误用 MedianTime）、run 级采样计数（消费此前被丢弃的 `EventSuiteProgress` 事件）、当前 workload 迭代迷你条 `▰▱ n/m` 与已耗时；Suite 页 section 行同样显示已耗时。
- **Compare 重做**（`tui/compare_picker.go`）：入口先进历史选择器（最新在前、重验证、上限 50 条），`spc` 标记 A/B（第三条顶掉最旧）、`c` 对比、`v` 查看单条（run→Results、suite→SuiteResults，Esc 返回）、`m` 手输路径模式（`-compare-a/-b` flag 预填）。kind 路由：两条 run → 既有 delta 表；两条 suite → 在 TUI 内直接渲染 `suitecompare` textgrid（此前必须退出到 CLI）；混合 kind toast 拒绝。Compare 页 View 不再每帧同步读盘（IO 全部移入 Cmd→Msg），flag 双路径仍直达对比页。
- **ResultDetail 新页**（`tui/result_detail.go`）：Results flat 视图 `d` 进入单 workload 证据页——指标 KVGrid、结构化错误红卡、每次迭代采样、适配器原始输出（逐行截宽保换行，超长截断 40 行）；Esc 返回且光标保留。另修复 flat 视图光标无法移动的映射错误。
- 配套：i18n 新增约 90 组 key（en/zh-CN 成对，parity 测试强制）；渲染边界测试扩展到全部新页面（80x24 双语）；`charmbracelet/x/ansi` 转直接依赖；Suite Results 文案补齐 i18n；`docs/tui-design.md`/`README`/`docs/product.md` 同步更新。

## v0.5.0（2026-09-06）

### 多语言界面（en / zh-CN）

- 新增 `i18n` 包（[go-i18n v2](https://github.com/nicksnyder/go-i18n) + 内嵌 TOML 目录），语言目录自动发现：新增语言 = 在 `i18n/messages/` 下新增同名目录。语言选择优先级 `--lang` flag > `VMBENCH_LANG` 环境变量 > `~/.config/vmbench/config.json` 的 `lang` 字段 > 系统 locale（`LC_ALL`/`LC_MESSAGES`/`LANG`，`zh*` 自动归一到 `zh-CN`）> 英文；未知取值回退英文并提示一次。所有子命令（run/suite/tui/list/nodes/sysinfo/compare/history）支持 `--lang`。
- 覆盖面：CLI 用法/参数帮助/校验错误/进度/预检提示、`sysinfo` 控制台标签、TUI 全部页面（菜单、按键提示、弹窗、表格、Suite 配置）、console 与 HTML 报告标签（含 `<html lang>`）。测试守卫：en/zh-CN 目录 key 集双向一致、双 locale 的 TUI 80x24 渲染边界。
- 数据层不翻译：JSON 字段名、状态枚举 token（ok/fail/partial/...）、suite section ID、workload 名称（与 `--filter` 正则匹配耦合）、适配器错误信息与 suite section 计算消息保持英文，翻译只发生在渲染层。
- CJK 宽度修复：TUI 截断改为按显示宽度（此前 6 个中文字符可穿透 10 列上限）；CLI 进度/表格填充改用 cell 对齐的 `PadCells`；tabwriter（按 rune 计数）在报告表格中替换为 `textgrid`（显示宽度对齐）。
- 已知限制（一期）：TUI 语言在启动时固定（无运行时切换键）；MCP 工具描述保持英文；`NormalizeOptions` 等数据面错误保持英文。

## v0.4.1（2026-09-05）

### 修复

- 测速下载请求补齐浏览器 User-Agent（复用 reachability 探测同一常量），部分按 UA 拦截裸客户端的 speedtest 节点不再整体失败；HTTP 200 但响应体为空时输出结构化错误 `endpoint returned no data`，不再产出 0 MiB/s 的假吞吐结果进入 compare。
- `cidr_neighbors`：宣告 CIDR 与本地 /24 完全相同时跳过第二次 bgp.tools 查询（此前同数据请求两次，且 HTML 报告会把同一条目按 subnet/announced 渲染两遍）；console/HTML 既有去重显示与 `ok/partial/error` 状态判定不变。
- 修复 1 vCPU 主机上 sysbench 多核 workload 被命名为 "CPU Single-Core (sysbench)"、导致同一次 run 出现重复 workload 名（结果按名索引互相覆盖）的问题；单/多核命名改为显式标记，不再按 `threads==1` 推断。

## v0.4.0（2026-09-05）

### 第三批：TUI 配置补齐 + IPv6 子网 / CIDR 活跃邻居

- TUI SuiteConfig 新增 `Media Sets`（默认 `all`，与地区互斥切换）与 `IP Quality Sources`（默认 `builtin`，`securitycheck` opt-in）两个多选字段；`china_isp` / `speedtest_isp` 三网 provider 已随动态 provider 列表可选。字段进入宽/紧凑两种布局并通过 80x24 约束，CLI/TUI/MCP 的 Suite 配置契约保持一致。
- `network_info` 新增 `ipv6_subnet`：通过 basics（Apache-2.0）的 RA / ip 命令 / 配置文件三级 fallback 探测本机公网 IPv6 的 on-link 前缀长度（实测 /112），只报原始值不做分配建议。
- `network_info` 新增 `cidr_neighbors`：按 ecs 同源方法（basics baseinfo 解析 bgp.tools 前缀 PNG 像素比例）估算本机 /24 子网与宣告 CIDR 的活跃邻居数/总数；bgp.tools 对部分网络返回 403 或非 PNG 响应时结构化降级为 error 证据。
- 新增直接依赖 `oneclickvirt/basics`（Apache-2.0，仅 `network/baseinfo` 与 `network/ipv6` 子包）。

### 对齐 ecs 的第二批能力（NAT 类型 / BGP 归属 / 逐跳 ASN / 平台诊断）

- `network_info` 新增 STUN NAT 类型完整检测：接入 Apache-2.0 的 [gostun](https://github.com/oneclickvirt/gostun)，输出 Full Cone / Restricted Cone / Port Restricted Cone / Symmetric 分类、mapping/filtering 行为、端口保持与 hairpin 回连能力、逐服务器证据（`stun_nat` 字段）；UDP 受限环境记录结构化错误，原有保守 NAT 启发式证据保留。
- `network_info` 新增 IP 的 BGP/RDAP 归属视图（`ip_bgp` 字段）：通过 backtrace/bgptools 输出宣告 ASN、注册网段/范围、RIR/注册日期/geofeed，以及上游/对等/IXP 关系（RIPEstat + PeeringDB），Tier 1 上游以公开 transit-free 名单标注数量；查询失败结构化降级。
- `route` 结果逐跳新增 `asn` 标注：IPv4 按运营商骨干网段精确匹配（含 223.118.32.0/21 CMIN2 特例），IPv6 使用内嵌的 AS prefix 快照（数据来自 oneclickvirt/backtrace v0.0.21，Apache-2.0，见 `bench/netio/asnprefixes/README.md`）；HTML hop 表新增 ASN 列。
- `sysinfo` 新增 Platform Diagnostics（自研 /proc+/sys 读取，无新依赖）：uptime、load 1/5/15、时区（/etc/localtime）、swap 用量、virtio 气球驱动、KSM（含 pages_shared）、TCP 拥塞控制/队列规则与 rmem/wmem 三元组、嵌套虚拟化 CPU flags（vmx/svm/masked）、HugePages、启动盘；Linux 实测、其他平台留空不报错，`vmbench sysinfo` 控制台与 Suite JSON 的 `system.platform` 同步输出。

## v0.3.0（2026-09-04）

### 对齐 ecs 的四大能力（流媒体 / IP 质量 / 回程线路 / 三网测速）

- 流媒体解锁切换到 Apache-2.0 的 [UnlockTests](https://github.com/oneclickvirt/UnlockTests) 库：`media` section 默认全平台 200+ 服务（原先内置 6 项手写探测已删除），新增 `--media-set`（`all|globe|tw|hk|jp|kr|na|sa|eu|afr|sea|oce|ai` 或逗号组合）与 suite/MCP 的 `media_set` 字段。结果新增 `raw_status` / `unlock_type` / `ip_version`，`Restricted` 计入 available 并在 summary 单列 `restricted`；全量双栈默认约 2-4 分钟。TUI 媒体卡修复此前 `yes/ok/unlock` 判定失效的问题并限制渲染行数。
- `route` section 新增回程线路类型判定：接入 Apache-2.0 的 [backtrace](https://github.com/oneclickvirt/backtrace) 库，基于已采集的系统 traceroute hop 证据（注入缓存 TraceFunc，无需 raw socket / root）输出每目标 `classification{code,label,confidence,rank,evidence}`（163 / 9929 / 4837 / CN2GIA / CN2GT / CTGNET / CMIN2 / CMI）与 `observed_asns`。分类失败只降级不失败 section，compare 仅作证据不参与 delta。
- `ip_quality` 扩展数据源：新增 ipapi.is 归属交叉验证（company/ASN 与 ip-api 不一致时记入 evidence；2026-09 起该 API 匿名档不再提供风险字段，故仅作归属证据、不影响 0-100 评分），结果新增 `sources[]` 状态表；新增 opt-in 外部源 `securitycheck`（`--ip-quality-source builtin,securitycheck`），复用 oneclickvirt/securityCheck 闭源二进制（PATH 或可执行文件旁 `binaries/`），输出 18 库视角的结构化字段与原文证据，二进制缺失时记录 `unavailable` 不影响 section 成败。
- 三网测速：新增 `china_isp` provider（node catalog 新 kind `isp_download`，12 个 speedtest.cn 直连节点按运营商分组顺序下载，50 MiB/节点预算，同运营商节点 fallback；节点数据来自 MIT 的 speedtest.cn-CN-ID，可用 `go run scripts/gen_isp_nodes.go` 刷新）与 `speedtest_isp` provider（Ookla `speedtest` CLI `-s` 按运营商 server ID 分组，ID 来自 MIT 的 speedtest.net-CN-ID）。embedded catalog revision 升至 `2026-09-04.1`。
- 新增 Go 依赖：`oneclickvirt/UnlockTests`、`oneclickvirt/backtrace`（Apache-2.0，与 MIT 兼容）；未复制任何 GPL（oneclickvirt/ecs 编排层）或无许可（securityCheck）代码。

## v0.2.0（2026-09-04）

### Go v0.2 可比较 VPS 证据

- 删除未被任何入口引用的 legacy 进程内 workload 包 `bench/integer|float|memory|diskio` 及仅被其使用的 `bench/common` GC/随机源/sink 工具，只保留仍被 runner 使用的统计函数；硬件测量继续完全依赖外部工具。
- 网络响应读取统一加上限：ip-api.com（4 KiB）、ipify（256 B）、流媒体探测（2 MiB）改用 `io.LimitReader`，防止恶意或异常端点无上限响应体耗尽内存；流媒体探测的读取错误不再被吞掉。
- `sh/build.sh` 与文档中的硬编码 `/root/temp` 输出路径改为 `${VMBENCH_OUTPUT_DIR:-${TMPDIR:-/tmp}}`，其余构建示例直接输出到仓库根目录（已被 .gitignore 覆盖）。
- CI 新增 `cross-build` 矩阵：与 GoReleaser 发布目标一致的 linux/darwin amd64+arm64、windows amd64 五个平台各执行 CGO 禁用的 `go build` + `go vet`，GoReleaser snapshot 现依赖该矩阵通过。

- 移除容易过时的 `vmbench ecs-diff` / `ecs-compare` 静态差异快照命令及其独立对标文档；实际能力以当前 CLI、结构化报告和产品文档为准。
- 修正 TCP Ping 对拒绝连接的误判：TCP RST/refused 现在作为目标响应计入 RTT/received，不再误算 100% 丢包；逐目标新增 `connection_state=open|refused|mixed|no_response`。
- Mail 端口改为顺序探测，避免对共享探测目标的并发突发导致随机假阴性；Suite 与 IP Quality 统一输出 `open|refused|timeout|error`，DNS 超时保持为探测错误，Compare 只使用 `open` 连接延迟。Route 新增 `resolved_target`、`destination_reached` 和 `status=ok|partial|error`，有 hop 但未到目标不再显示为成功；Compare 只接受显式到达目标的 Route 指标，旧报告缺少到达证据时不计算 delta。
- TUI Running 保留 `partial` 独立状态并按严格 Suite 规则计入非成功数量；IP Quality fail-closed 时结果卡继续显示 section error、风险摘要与 Port 25 证据。
- `run` 和启用 hardware 的 `suite` 新增外部工具预检提示；预检复用 workload Name/Category filter，只检查本次实际可能运行的 adapter，Linux 同时输出已知 Debian/Ubuntu 安装命令。Suite CLI 默认在 stderr 实时输出 section 生命周期，新增 `--quiet` 抑制进度。
- CLI JSON/HTML 改为同目录临时文件、fsync、rename 原子写入，Unix mode 收紧为 `0600`。Linux dd read 使用 `iflag=direct` 避免页缓存虚高，其他平台无法保证 uncached read 时 fail-closed；`sh/build.sh` 设置 `CGO_ENABLED=0`，与 GoReleaser 构建保持一致。
- 新增版本化 `nodecatalog/`：Manifest 记录 schema/revision/生成与过期时间；节点记录稳定 ID、地区/城市、运营商、ASN、IP family、protocol、endpoint、source 和流量预算，download 的 `traffic_bytes` 实际限制响应体读取量。默认 embedded 离线快照，并支持 `embedded` / `auto` / 显式 path 与精确 revision pin。
- 新增 `vmbench nodes list|verify|update|health`。远程更新必须由调用方提供 Ed25519 trust root 和 detached signature；验证精确 manifest 字节及严格 schema 后才原子替换缓存（Unix mode `0600`），篡改或 revision 不匹配时 fail-closed。
- embedded route/ping 节点扩展到成都、CERNET、CSTNET 与 IPv6；报告保留实际 catalog source/revision、节点 identity、`probe_protocol` 和 `probe_tool`，IPv6 route 按 family 选址，节点及探测方式变化可追踪。成功 Ping 的零值 latency/jitter/loss 也显式保留。
- sysinfo 新增本机 virtualization system/role；Suite 新增 `network_info`，输出公网 IPv4/IPv6、ASN/provider/location 和保守的 `direct/translated/unknown` NAT 证据。hardware-only `run` 不因此增加公网请求。
- Suite 新增 `reachability`，对 website HTTPS 与 Telegram DC TCP 目标逐项记录 protocol、endpoint、latency、HTTP status、status 和 error。
- Suite JSON 升级为 schema-v2 envelope：加入 `report_kind`、唯一 `report_id`、app build、system、UTC timestamps/duration、规范化 config 和 catalog provenance；保留 v1 version/Unix time 字段兼容旧 consumer。
- Suite HTML 补齐 hardware workload、network info、完整 route hops、ping、provider-level speed、IP quality、reachability、mail/media 及 warning/error 明细，network-only/disabled/failed section 保持 nil-safe。
- `vmbench compare` 自动识别 benchmark/Suite JSON，并支持两份以上报告。Suite Compare 只有 unit、实际 protocol/IP family、provider/probe tool、target/node identity 和所需 catalog revision 兼容时才计算 delta；HTTP status 等分类码不参与百分比 delta；不兼容时保留值并输出 incompatibility reason。
- 新增原子本地 `history`：`add/list/show/delete/compare --last N`；Unix 使用目录 `0700` / 文件 `0600`，`run` / `suite --save-history [--history-tag TAG]` 可直接保存报告，最近 N 份必须为同一 report kind 才比较。
- CLI、TUI、MCP 使用同一 Suite 配置校验/归一化契约，各入口按场景暴露字段子集；TUI 覆盖 iterations、timeout、sections、IP version、hardware tools、speed providers、iperf hosts、route selection、`catalog_source` 和 `catalog_revision`。
- Go TUI 修复 80 列横向溢出，并在低于 40 行时为 SuiteConfig/Running/Results 使用当前字段或逐 section 单行摘要；九 section 与日志/取消提示的最坏组合可在 `80x24` 内渲染。
- 继续只输出原始 time/throughput/latency/detail/error；未重新引入 benchmark 总分、等级或 category score。IP Quality 的 0-100 风险评分仍仅是业务诊断。

### Go 主线测量可信度与安全默认值

- Go 工具链基线升级到 `1.26.6`，CI 与正式 Release 在测试、vet 后运行 `govulncheck`，阻止带可达标准库漏洞的构建发布。
- Runner 改为始终串行、隔离执行 workload，不再并发不同 benchmark，也不再修改进程级 `GOMAXPROCS`、GC 或 OS 线程绑定状态。
- `vmbench run` 默认范围改为 `hardware`；新增 `--scope hardware|network|all`，只有显式启用 network/all 才注册网络 workload，并提示基础网络测试可能传输约 1.75 GB 数据。
- 所有 `bench/netio` workload 最多执行一次真实探测；报告记录实际 `iterations=1`，硬件 workload 继续按请求的 1-9 次迭代聚合。
- 删除 `mode=all` 的第二轮重复 pass；旧的 `--mode multi/all` 仅为 CLI 兼容保留，输出 warning 并只运行一次标准外部工具 catalog。
- CLI 严格拒绝非法 regex、mode、scope、iteration、section、provider 和 tool；`run` 没有匹配 workload 或任一 workload 失败时返回退出码 1。
- Go API 遇到非法 filter regex 时使用永不匹配表达式，不再回退为全量执行。
- Runner 新增逐 workload 的 start/done 回调：首个 sample 前立即发射 `suite_start`，该 workload 返回后立即发射 `suite_done` / `suite_fail`，不再等整批结束；同名 workload 也不会合并事件。

### Go 报告与 Suite 状态

- benchmark JSON 升级到 schema v2，根节点新增 `schema_version`；config 新增 `scope` / `iperf_hosts`，hardware scope 写入 `extensions=false`，network/all 写入 `true`。未实际启用 network/speed 时会清除未使用的 iperf host，避免污染 provenance。
- 每项结果新增实际 `iterations` 与 `samples_ms`；`bytes_processed` / `ops_processed` 改为显式可选累计量，只有 workload 声明 bytes/operations 且成功 sample 语义一致时才输出，不再把 rate/score/latency 猜成 processed。
- Compare 忽略带 error 的 metric，将 `ms avg` 按越低越好处理，不再跨不兼容 throughput 单位计算 delta，并对迭代次数、mode、scope、工具/iperf host 选择与重复 workload 输出可比性警告。
- Suite 成功判定收紧为所有 enabled section 必须 `status=ok`；空状态、`skipped`、`partial`、`error` 均使 `HasFailures()` / finalize 失败并发 `section.fail`，只有 disabled section 发 `section.skip`。
- route/ping/speed/ip_quality/mail/media 各自派生默认 5 分钟的 section timeout context，deadline/cancel 覆盖为结构化 error；hardware 继续按 workload 应用 timeout，不额外套 section deadline。
- Net Ping 全部目标失败时返回聚合 error 但保留逐目标 results；部分成功继续保留全部明细。iperf3 provider 缺少可用 host 时直接返回 error。
- Suite speed provider 默认值收敛为仅 Cloudflare；多 provider 顶层汇总标记 `aggregation=best_per_metric`，避免误解为单一节点的一次测量。
- Suite HTML 对禁用或失败的 speed/IP quality/media section 做 nil-safe 渲染；Go TUI 的 SuiteConfig 初始 Quick preset 现在实际启用 Quick sections，并移除误导性的独立 Multi-Core 入口。

### Go 外部工具与网络可靠性

- 平台默认硬件工具改为：Linux `sysbench,openssl,fio`，macOS `openssl`，Windows `winsat`；其他 adapter 仍可显式选择。
- 本地 Linux fallback 只从解析后的 vmbench 可执行文件相邻 `binaries/` 或可执行文件同目录加载，不再信任当前工作目录下的同名文件。
- sysbench/fio/OpenSSL/WinSAT 在解析不到主指标时 fail-closed；fio 使用唯一临时文件、`--unlink=1`、defer 清理，并按 Linux/macOS/Windows 选择对应 AIO engine。
- Go traceroute 改为系统 `traceroute` / `tcptraceroute` / `tracepath` / `tracert`，最多 4 路并发；命令缺失、无有效 hop、全超时或全部目标失败均结构化报错。
- IP Quality 改为 fail-closed：元数据、公网 IPv4、DNSBL 或 Port 25 结论不完整时不生成 0-100 score；DNSBL zone 改为并发查询。
- Cloudflare upload 改为流式零数据请求体，不再为 50 MiB payload 分配等量内存；Suite 的 Cloudflare 速率换算修正为十进制 Mbps。
- Linux CPU `base_frequency` 修正 kHz 到 MHz 的换算；sysinfo 外部命令统一增加 30 秒超时。
- `sysinfo.Collect(ctx)` 现在把调用方 context 传给所有平台 collector；已取消 context 立即返回 warning，外部命令可被调用方 deadline 提前终止。

### 硬件测评细化

- Linux 默认集中的 `sysbench` 内存 workload 拆分为 read bandwidth、write bandwidth 与 random read latency，继续只输出原始吞吐/延迟/detail/error。
- Linux 默认集中的 `fio` 磁盘 workload 拆分为 4K random read/write Q1/Q32 与 1M sequential read/write Q1/Q8，分别输出 IOPS、MiB/s 与可解析的平均延迟。
- Runner 在每次 iteration 后采集外部工具解析出的吞吐和延迟，并对样本取中位数，避免多次迭代时只使用最后一次外部工具解析值。

### 文档梳理

- 新增 `docs/current-state.md` 作为最新技术与产品状态的总览入口。
- `docs/README.zh-CN.md`、`docs/product.md`、`docs/tech-stack.md` 增加最新状态导览与交叉链接。

### Benchmark runner 修正（早期阶段，当前行为以上述 Go 主线条目为准）

- 此前曾将 `vmbench run --mode all` 修正为先执行 single pass、再执行 multi pass；当前开发版本已进一步删除第二轮 pass，仅保留兼容 warning 与一次标准 catalog 执行。
- 此前并行 workload 的 progress event 曾改为 runner 内串行发射以避免回调竞争；当前开发版本已进一步将 workload 调度整体改为串行隔离。
- 网络探测类 workload 跳过合成 warm-up，避免 ping/iperf 等真实网络探测被重复执行；`Net Ping` 缓存命中时保留真实 elapsed/processed。
- Cloudflare upload / multi-download 增加 HTTP 状态码与 body copy 错误检查。
- 进程内顺序磁盘 workload 不再吞掉非 EOF 读错误，并校验读取字节数。

### MCP Server

- 新增 `vmbench mcp serve --transport stdio`，通过 Model Context Protocol 暴露给大模型客户端调用。
- 首批 tools：`vmbench_capabilities`、`vmbench_sysinfo`、`vmbench_run`、`vmbench_suite`。
- MCP 默认安全策略：`run` 默认 hardware scope、`suite` 默认只跑 hardware、`iterations` 默认 1 且最大 9、`timeout_ms` 最大 15 分钟。
- MCP 输入限制在内置枚举，不接受任意 shell 命令；stdout 只输出 JSON-RPC，诊断写 stderr。
- MCP 返回原始指标和结构化诊断，不引入 benchmark 总分、等级或 category score。
- MCP 现在严格拒绝非法 regex、显式非正/越界的 iterations 与 timeout，以及混入未知项的 section/provider/tool/route preset 数组，不再静默规范化后继续运行。
- MCP 测量失败时保留完整 `structuredContent.report` 并设置 `isError=true`；参数校验失败则返回错误文本且不启动测量。


### GitHub 发布准备

- 修正 `.gitignore`：提交 docs / sh / install 脚本，忽略 `dist/`、本地构建产物和可选第三方 `binaries/`。
- README 增加 CI / Go Reference / MIT badge、中文说明入口、GitHub Releases 安装方式和贡献入口。
- 新增 `CONTRIBUTING.md`、`install.sh`、`docs/README.zh-CN.md`。
- GoReleaser 发布包增加中文 README 与 CHANGELOG，package 描述调整为 VPS benchmark suite。
- 第三方 benchmark 二进制不进入官方源码和 release 包；如需 Linux 本地 fallback，只能放在解析后的 vmbench 可执行文件相邻 `binaries/` 或同目录，当前工作目录不会被搜索。

### 硬件跑分外部工具化

- 新增 `--hardware-tool sysbench,openssl,fio,dd,stream,mbw,geekbench,winsat`，可用 `all` 启用全部 adapter。
- hardware section 默认注册外部工具 workload；当前默认值按平台选择：Linux `sysbench,openssl,fio`，macOS `openssl`，Windows `winsat`。
- `vmbench suite` 的 hardware 不再使用进程内 CPU/内存/磁盘 benchmark，也不在工具缺失时 fallback。
- 新增 `sysbench memory` 外部内存带宽 workload。
- 新增 `dd` 磁盘顺序写/读 workload。
- 新增 `STREAM` / `mbw` 内存带宽 workload。
- 新增可选 `Geekbench` CPU upstream score workload，不默认跑，不作为 vmbench 总分。
- 新增可选 `WinSAT` CPU / memory / disk workload。
- 外部命令 workload 跳过 runner 的 warm-up 二次执行，避免同一外部工具被额外跑一遍。
- TUI Dashboard 移除 Native / Full 硬件跑分入口，保留 External 与 Suite 入口。

### TUI 重新设计

- 新 8 主题系统(`tui/theme/`):dracula / tokyonight / catppuccin / nord / gruvbox / rose-pine / solarized / monochrome,基于 `lipgloss.AdaptiveColor` 自动深浅适配
- 主题切换:Dashboard `[t]` 键循环、`VMBENCH_THEME` env、`~/.config/vmbench/config.json` 持久化；公开 TUI 不再提供 `--theme` 参数
- 新组件库 `tui/comp/`:Card / Progress / Spinner / StatusPill / KVGrid / Bar / Sparkline / Header / Footer / Tabs / Toast / Banner / Modal / Table / Layout
- 自适应布局:5 个断点(<80 / <120 / <160 / <200 / ≥200),响应式列宽与卡片网格
- Dashboard:ASCII 横幅 logo + 左 nav + 右 sysinfo 卡片 + 主题徽章
- Running:卡片网格按 category 分组,bubbles spinner + bubbles progress + event log viewport,取消改用居中 modal
- Results:Cards / Grouped / Flat 三 tab,save 成功/失败 toast 反馈
- Compare:两栏 sysinfo 卡片 + delta 表格(`▲ 绿`/`▼ 红`/`=`)
- 新 SuiteConfig 页:preset radio + sections 多选 + speed providers 多选 + route presets 多选
- 新 SuiteRunning 页:section 卡片网格 + 总进度 + spinner + log
- 新 SuiteResults 页：九个 section 使用定制或结构化卡片（Hardware / NetworkInfo / Route / Ping / Speed / IPQuality / Reachability / Mail / Media）
- Suite 入口加入 Dashboard 菜单
- 渲染快照工具 `cmd/vmbench-rendertest/`(build tag `rendertest`),便于无终端环境下生成视觉截图

### Suite 事件钩子

- `suite/options.go` 加 `EventHandler` 与 `Event{Kind, Section, Status, Message, Time}`
- `suite/run.go` 每 section 发 `EventSectionStart/Done/Fail/Skip`,跑完发 `EventSuiteDone`
- TUI suite_running 通过 channel 订阅刷新

### 移除不成熟评分体系

- 删除 `score/` 包
- 删除 benchmark 总分、等级、category score
- JSON/HTML/Console/TUI 改为展示原始指标
- Compare 改为比较 time / throughput / latency

### Suite 增强

- 新增 `ping` section
- 新增 `mail` section
- 新增 `--preset quick|website|proxy|mail`
- 新增 `--speed-provider cloudflare,speedtest_net,speedtest_cn,iperf3`
- 新增 `--only`
- 新增 `--skip`
- 新增 `--ip-version v4|v6|dual`
- Suite JSON/Console/HTML 输出记录本次使用的 preset 和启用 section
- Speed section 按 provider 分组展示 Cloudflare / Speedtest.net / Speedtest.cn / iperf3 的原始测量值和错误
- Speed JSON 增加 `groups[]` 聚合层，保留 `providers[]` 原始层

### 报告调整

- `report.ResultEntry` 保留原始测量字段；当前 schema v2 字段为：
  - `iterations`
  - `median_ms`
  - `samples_ms`
  - `throughput_per_sec`
  - `throughput_unit`
  - `bytes_processed`
  - `ops_processed`
  - `avg_ns_per_access`
  - `detail`
  - `error`

## 历史说明

早期版本曾实验过基于 baseline 的 synthetic score，但由于基线和权重尚不成熟，已移除。后续如需重新引入，应先建立公开、可复现的参考基准和版本化评分 schema。

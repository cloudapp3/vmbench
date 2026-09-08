# VMBench Changelog

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

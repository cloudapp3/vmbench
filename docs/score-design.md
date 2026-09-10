# vmbench 评估能力设计（score）

> 状态：**已实现**（v0.12.0）。本文是 `vmbench score`（确定性评估）能力的实现规范；实现按 phase 更新本文状态，并同步 README / product / tech-stack / capabilities / CHANGELOG。

## 1. 背景与目标

vmbench 此前的产品立场是"不输出总分/等级"（README、capabilities、MCP capabilities 均有承诺）。经项目所有者决策，本设计在保留原始指标第一公民地位的前提下，**反转该政策**，引入确定性评估层：

- **确定性规则引擎（`vmbench score`）**：消费报告 JSON，按版本化基线归一化、加权，输出维度评级与场景适配度。数学计算全部由代码完成，保证精确与一致；无网络，可复现可审计。

两条底线：

1. **评分确定性**：`score.Evaluate` 是纯函数（无网络、无时钟、无文件系统副作用），同输入 + 同基线 revision ⇒ 字节级相同的输出。
2. **覆盖率不撒谎**：稀疏报告显式披露覆盖率/置信度；缺失维度 reweight-with-disclosure（输出 `basis`/`excluded`）；证据不足时 composite 置 null，绝不给单维报告硬凑总分。

对最初参考稿的三处修正（论证保留备查）：

- 吞吐类指标不用线性 clamp 归一化（EPYC 与老 Xeon 同撞 100 分，高端分辨率浪费），改 log 曲线。
- 延迟类指标不用任意 L_min/L_max 锚点，改固定阈值带。
- 取消 P_steal × P_loss 连乘惩罚：邻居争抢本身已压低实测 sysbench/fio/speed 成绩，连乘等于双重计罚；改为独立 `stability` 维度，单次计入。

### 非目标

- run/checkup 报告本身不内嵌评分字段；评估永远是报告的派生物（输入报告 + 基线 ⇒ 独立 assessment 文档）。
- 不做 LLM 语义诊断（经项目所有者决策移除，评估完全确定性）。
- 本期不做：MCP score 工具、assessment HTML、TUI 评估查看器、`history` 存储 assessment（均记为 deferred）。

## 2. 词汇与兼容

历史上 vmbench 删除过一个 score 包，`AGENTS.md` 禁止重新出现旧字段名 `RawScore`、`ScoreGrade`、`CategoryScores`、`MultiScore`。本设计启用全新词汇，Go 类型、JSON 字段、文档统一使用：

| 概念 | 词汇 | JSON |
|---|---|---|
| 评估文档 | `Assessment` | `report_kind: "assessment"` |
| 0-100 数值 | index | `index` |
| 等级字母 | rating ∈ S/A/B/C/D | `rating` |
| 维度 | dimension | `dimensions[]`（id/index/rating/confidence/coverage_pct） |
| 场景适配 | profile | `profiles[]`（id/index/rating/fit/veto） |
| 场景结论 | fit ∈ recommended/conditional/unsuitable/insufficient_evidence | `fit` |

旧字段名禁令继续有效；本文档词汇表是唯一合法来源。

## 3. 输入与数据路径

`score.Evaluate(data []byte, opts Options)` 接受三种输入，kind 判定复用 `history.Inspect`（含 legacy `suite` → checkup）：

- **run 报告**（`report.Document`，schema v2，无 report_kind）：`results.workloads[]` 与 `extensions.workloads[]`，每行取 `result.throughput_per_sec` / `throughput_unit` / `avg_ns_per_access` / `latency_p99_ns` / `median_ms` / `samples_ms[]` / `error`；覆盖率上下文取 `config.hardware_tools[]`、`config.iterations`；系统上下文取 `system.cpu`、`system.memory.total_bytes`、`system.virtualization`、`system.os`。
- **checkup 报告**（`CheckupReport`）：硬件部分取内嵌的 `hardware.report`；网络部分取 `speed.result.summary.{download_mbps,upload_mbps}`（仅 status ok/partial）、`ping.results[]` 中位聚合（`avg_latency_ms`/`jitter_ms`/`packet_loss`，按 status=ok 行取中位数）。
- 稀疏输入是常态：workload 由用户勾选，缺行记 `missing`，`error` 非空或 `result==nil` 的行记 missing 并把错误文本进 warnings——绝不当作 0 分。

刻意排除：ping `avg_latency_ms` 是地理属性不入 composite，仅用于 proxy veto；`Geekbench CPU`（上游私有权度）与 `ip_quality.score`（业务诊断）均不入确定性评分。

## 4. 基线表 schema 与版本化

`score/baselines.json`（`//go:embed`，仿 nodecatalog）：

```json
{
  "schema_version": 1,
  "revision": "2026-09.1",
  "grades": {"s": 90, "a": 80, "b": 65, "c": 50},
  "metrics": { "<metric-id>": { "dimension": "...", "weight": 0.40,
      "source": {"workload": "<exact workload name>", "field": "throughput_per_sec", "fallback_field": "latency_p99_ns"},
      "units": ["events/sec"], "class": "log", "floor": 500, "ceiling": 10000,
      "bands": {"excellent": 60, "good": 90, "fair": 150, "cutoff": 400},
      "requires_tool": "sysbench", "requires_kind": "any", "platform": "",
      "optional": false, "fallback_for": "" } },
  "dimensions": {"cpu": {"weight": 0.30}, "...": {}},
  "profiles": {"web": {"weights": {"cpu": 0.32, "...": 0}, "vetoes": [{"metric": "...", "op": "gt", "value": 2000000, "unit": "ns", "cap": "C"}]}}
}
```

- 加载：严格 Decode（`DisallowUnknownFields`、单 JSON 文档、1 MiB 上限）+ `Validate()`（metric id 唯一；维度/ profile 权重和为 1；引用存在；log 类 floor>0 且 <ceiling；band 类 exc<good<fair<cut）。
- 来源：内嵌（默认）或 `--baseline PATH`；`--baseline-rev REV` 指定 revision 时严格相等校验，不匹配硬错误（fail-closed）。基线 `revision` 与来源写入输出的 `baseline` 字段。
- 锚点数值是知情占位，随 revision 迭代校准，不改代码。

## 5. 归一化模型

- **吞吐类（class=log）**：`n(v) = clamp(100·ln(v/floor)/ln(ceiling/floor), 0, 100)`。单调、平滑、两端有锚点。
- **延迟/比率类（class=band）**：固定阈值 exc/good/fair/cut（越小越好），分段线性过点 (exc,100) (good,70) (fair,40) (cut,0)；`v ≤ exc → 100`，`v > cut → 0`。
- **单位纪律**：指标只匹配 `units` 列表内的行（`MiB/s` ≠ `MB/s`），同名异单位 → 缺项 + warning；optional fallback 指标自带独立锚点，不做跨单位换算。

## 6. 维度 / 指标 / 权重全表（revision 2026-09.1）

维度权重：cpu .30、memory .20、disk .30、network .12（requires_kind: checkup）、stability .08。等级线 S≥90 / A≥80 / B≥65 / C≥50 / D<50。CPU 单核权重占维度约 70%（VPS 场景单核决定 Web 响应/编译/代理性能）。

| metric id | 来源 workload（精确名）| 字段/单位 | class | 锚点 | 维度内权重 |
|---|---|---|---|---|---|
| cpu.sysbench.single | CPU Single-Core (sysbench) | events/sec | log | 500 / 10000 | .40 |
| cpu.sysbench.multi | CPU Multi-Core (sysbench) | events/sec | log | 2000 / 150000 | .30 |
| cpu.openssl.aes | OpenSSL AES-256-CBC | MiB/s | log | 200 / 8000 | .15 |
| cpu.openssl.sha | OpenSSL SHA256 | MiB/s | log | 150 / 5000 | .15 |
| cpu.winsat | WinSAT CPU | MB/s | log | 5 / 120 | 1.0（platform: windows）|
| mem.read.bw | Memory Read Bandwidth (sysbench) | MiB/s | log | 2000 / 40000 | .35 |
| mem.write.bw | Memory Write Bandwidth (sysbench) | MiB/s | log | 1500 / 35000 | .25 |
| mem.rand.lat | Memory Random Read Latency (sysbench) | ns | band | 60/90/150/400 | .40 |
| mem.stream | Memory Bandwidth (STREAM) | MB/s | log | 2000 / 60000 | optional, fallback_for mem.read.bw |
| mem.mbw | Memory Bandwidth (mbw) | MiB/s | log | 2000 / 40000 | optional, fallback_for mem.read.bw |
| mem.winsat | WinSAT Memory | MB/s | log | 5 / 150 | 1.0（platform: windows）|
| disk.rand4k.read.q32 | Disk 4K Random Read Q32 (fio) | IOPS | log | 3000 / 400000 | .25 |
| disk.rand4k.write.q32 | Disk 4K Random Write Q32 (fio) | IOPS | log | 2000 / 300000 | .15 |
| disk.rand4k.read.q1.lat | Disk 4K Random Read Q1 (fio) | latency_p99_ns（fallback avg_ns_per_access）| band | 100µs/300µs/1ms/10ms | .25 |
| disk.rand4k.read.q1.iops | Disk 4K Random Read Q1 (fio) | IOPS | log | 500 / 30000 | .10 |
| disk.rand4k.write.q1.lat | Disk 4K Random Write Q1 (fio) | latency_p99_ns（fallback avg）| band | 150µs/500µs/2ms/15ms | .10 |
| disk.seq1m.read.q8 | Disk 1M Sequential Read Q8 (fio) | MiB/s | log | 100 / 15000 | .10 |
| disk.seq1m.write.q8 | Disk 1M Sequential Write Q8 (fio) | MiB/s | log | 80 / 12000 | .05 |
| disk.dd.read / disk.dd.write | Disk Read (dd) / Disk Write (dd) | MiB/s | log | 50 / 5000 | optional fallback |
| net.download | speed.result.summary.download_mbps | Mbps | log | 50 / 5000 | .40 |
| net.upload | speed.result.summary.upload_mbps | Mbps | log | 30 / 3000 | .30 |
| net.ping.jitter | ping.results[].jitter_ms 中位 | ms | band | 1/3/8/50 | .30 |
| stability.cpu.steal | CPU Steal (/proc/stat) | % | band | 0.5/1.5/3/8 | .50 |
| stability.samples.cv | cpu+disk workloads samples_ms 的 CV 中位 | ratio | band | 0.05/0.15/0.30/0.60 | .30 |
| stability.net.loss | ping.results[].packet_loss 中位 | fraction | band | 0/0.005/0.02/0.10 | .20 |

Q1 与 Q32 全程独立成指标，互相不替代（数量级差 1-2 个数量级）。

## 7. 稳定性维度设计

取代参考稿的乘法惩罚。理由：邻居争抢的后果已经体现在 sysbench/fio/speed 的实测值里（被压低的成绩），再乘惩罚系数是双重计罚，且让评分不可解释。`stability` 维度（权重 .08）单次计入三类信号：CPU steal（调度层面的直接证据）、跨迭代样本变异系数（实测成绩的抖动）、ping 丢包（网络层不稳）。steal/CV 是超售判断最直接的证据来源。

## 8. 场景 profile 与 QD 专属 veto

veto 只把 profile rating 封顶到 C（输出 veto 详情），不改 index。fit 判定：recommended（无 veto 且 index≥65）/ conditional / unsuitable（veto 触发）/ insufficient_evidence（profile 关键指标缺失）。

| profile | cpu/mem/disk/net/stab | veto（封顶 C） |
|---|---|---|
| web | .32/.22/.26/.13/.07 | disk.rand4k.read.q1.lat > 2ms（Q1 专属）；stability.net.loss > 2%；cpu.sysbench.single < 800 |
| build | .48/.28/.19/0/.05 | cpu.sysbench.multi < 3000；disk.seq1m.write.q8 < 50 MiB/s |
| proxy | .28/.12/.07/.38/.15 | ping avg_latency_ms 中位 > 150ms（输出地理告警）；stability.net.loss > 1%；cpu.sysbench.single < 600 |
| storage | .08/.17/.55/.13/.07 | disk.rand4k.read.q32 < 15000 IOPS（Q32 专属）；disk.seq1m.read.q8 < 100 MiB/s |

## 9. 覆盖率 / 置信度 / 综合合成

- 期望指标集 = `requires_tool` ∈ 报告 `config.hardware_tools`（无要求则不计）∧ `requires_kind` 匹配输入 kind ∧ `platform` 匹配报告 OS 族；`optional` 指标永不拉低覆盖率；`fallback_for` 指标可替代缺失主指标（用自己的锚点，note 标注）。
- 维度 index = 已有指标 normalized 的权重重归一化均值；composite index = 覆盖率 >0 的维度按权重重归一化均值，实际参与/排除的维度显式写入 `composite.basis` / `composite.excluded`。
- composite 置 null 的兜底：cpu 维度无数据，或性能维度（cpu/memory/disk/network）有数据者 <2。
- `composite.status`：期望维度全覆盖 ≥80% → complete，否则 partial。
- 置信度 = coverage_pct/100 × iteration_factor（iterations≥3→1.0、2→0.85、1→0.7）；composite 置信度为维度置信度的权重重归一化均值。

## 10. 输出 schema

```go
type Assessment struct {
    SchemaVersion int                   `json:"schema_version"`      // 1
    Kind          string                `json:"report_kind"`         // "assessment"
    Generator     string                `json:"generator"`
    Source        SourceRef             `json:"source"`              // 输入 kind/id/时间
    Baseline      BaselineRef           `json:"baseline"`            // revision + 来源
    Composite     *Composite            `json:"composite,omitempty"` // nil = 证据不足
    Dimensions    []DimensionAssessment `json:"dimensions"`
    Profiles      []ProfileAssessment   `json:"profiles"`
    Coverage      Coverage              `json:"coverage"`
    Warnings      []string              `json:"warnings,omitempty"`
}
// Composite{index, rating, status, basis[], excluded[]}
// DimensionAssessment{id, index, rating, confidence, coverage_pct, metrics[]{id,workload,value,unit,normalized,class,note}, missing[]}
// ProfileAssessment{id, index, rating, fit, veto{metric,value,limit,cap}, missing[]}
// Coverage{metric_pct, present[], missing[], dimensions_present[], dimensions_missing[], confidence}
```

**不含任何时间戳**（Generator 版本除外）：确定性底线 ⇒ 同输入同基线字节级相同输出。全部可选字段 `omitempty`。

## 11. CLI：`vmbench score`

```
Usage: vmbench score <report.json|-> [flags]

  --json              assessment JSON 输出到 stdout
  --out PATH          写 assessment JSON 到文件（0600）
  --baseline PATH     基线 JSON 文件（默认内嵌）
  --baseline-rev REV  要求精确 revision（fail-closed）
  --lang en|zh-CN
```

退出码：0 成功（含 partial）、2 用法错、1 运行错（文件不可读/JSON 非法/报告不识别/基线解码失败/revision 不匹配）。控制台渲染（tabwriter）：composite/coverage 概览 → 维度表 → 指标明细 → profiles → warnings；标签走 i18n，数据值英文。

## 12. 测试计划

- score：fixture 构造器（run/checkup/legacy suite）；归一化单调/钳位/边界值；稀疏、全错误行、单位错配、p99 优先于 avg（note）、reweight 披露、Q1/Q32 veto 各自触发、覆盖率/置信度算术、**确定性（两次 Evaluate → json.Marshal 字节相同）**；基线严格解码与 Validate 失败矩阵。
- catalog：fio percentile fixture（读/写向）、缺失 percentile 静默置 0；steal 假 readStat 数学、Δtotal==0 报错、平台门控。
- bench/report：LatencyPercentileWorkload 中位数管道；`latency_p99_ns` omitempty 往返。
- cmd/vmbench：usage 行、退出码矩阵、--json 形状、--baseline-rev 不匹配 → 1。

## 13. 实施阶段

- ✅ P0 政策反转 + 本文档（AGENTS.md、capabilities §1）。
- ✅ P1 数据层：fio p99（catalog → bench → report → 展示点）+ `CPU Steal (/proc/stat)` 探针（linux-only）。
- ✅ P2 `score/` 包 + baselines.json + `vmbench score` CLI + i18n + 文档同步（README/tech-stack/capabilities/CHANGELOG/MCP 政策串）。
- ✅ P3 收尾：本文档状态 → 已实现；product/README.zh-CN 指针；deferred 项记录；原计划的 LLM `analyze` 诊断经项目所有者决策移除。

Deferred（记录在案，暂不做）：MCP `vmbench_score` 工具、assessment HTML 渲染、TUI 评估查看器、`history` 新 kind `assessment`、基线真实语料校准（发版前收集 VPS 报告）。

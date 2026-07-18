package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const ecsDiffSnapshotDate = "2026-07-16"

type ecsDiffSource struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type ecsDiffLegendItem struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

type ecsDiffRow struct {
	Area       string `json:"area"`
	Status     string `json:"status"`
	VMBench    string `json:"vmbench"`
	ECS        string `json:"ecs"`
	Difference string `json:"difference"`
	Next       string `json:"next"`
}

type ecsDiffNextAction struct {
	Priority string `json:"priority"`
	Item     string `json:"item"`
	Reason   string `json:"reason"`
}

type ecsDiffSnapshot struct {
	SnapshotDate string              `json:"snapshot_date"`
	Requirement  string              `json:"requirement"`
	Sources      []ecsDiffSource     `json:"sources"`
	Legend       []ecsDiffLegendItem `json:"legend"`
	Rows         []ecsDiffRow        `json:"rows"`
	NextActions  []ecsDiffNextAction `json:"next_actions"`
	Notes        []string            `json:"notes"`
}

func runECSDiff(args []string) int {
	fs := flag.NewFlagSet("ecs-diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "output the comparison snapshot as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.Join([]string{
			"Usage: vmbench ecs-diff [--json]",
			"",
			"Show a maintained snapshot of the current differences between vmbench and ECS/GoECS.",
			"The snapshot is product guidance only: it does not run benchmarks and does not create a benchmark score.",
			"",
			"Flags:",
		}, "\n"))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	snapshot := currentECSDiffSnapshot()
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeECSDiffText(os.Stdout, snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func currentECSDiffSnapshot() ecsDiffSnapshot {
	return ecsDiffSnapshot{
		SnapshotDate: ecsDiffSnapshotDate,
		Requirement:  "P1/v0.2 可比较的 VPS 证据，对标 ECS/GoECS 当前能力",
		Sources: []ecsDiffSource{
			{
				Name:        "ECS legacy shell",
				URL:         "https://github.com/spiritLHLS/ecs",
				Description: "原 shell 融合怪仓库，README 指向 Go 版本并以维护为主。",
			},
			{
				Name:        "GoECS current",
				URL:         "https://github.com/oneclickvirt/ecs",
				Description: "当前 Go 版 ECS，覆盖 VPS 融合测评、多场景命令与 Docker/离线运行。",
			},
		},
		Legend: []ecsDiffLegendItem{
			{Status: "aligned", Description: "核心能力已对齐"},
			{Status: "partial", Description: "已有基础能力，但 ECS 覆盖更广或细节更深"},
			{Status: "gap", Description: "vmbench 当前没有对应能力"},
			{Status: "vmbench_only", Description: "vmbench 当前具备而 ECS 不是重点的能力"},
		},
		Rows: []ecsDiffRow{
			{
				Area:       "项目形态",
				Status:     "partial",
				VMBench:    "Go CLI + Bubble Tea TUI，Console/JSON/HTML 输出，强调原始指标与结构化报告。",
				ECS:        "shell 版 + 当前 GoECS，提供交互菜单、参数模式、非 root、离线包与 Docker 运行。",
				Difference: "ECS 的部署/运行入口更丰富；vmbench 的结构化报告和 TUI 更强。",
				Next:       "保留 Go/TUI 路线，补齐安装脚本、Docker 或离线包属于后续分发增强。",
			},
			{
				Area:       "系统信息",
				Status:     "partial",
				VMBench:    "采集 OS、CPU、GPU、内存、磁盘、网络接口和虚拟化；network_info 输出公网 IPv4/IPv6、ASN、provider 证据及 NAT 三态 heuristic。",
				ECS:        "除基础信息外，还覆盖 IP、ASN、虚拟化、NAT、IPv4/IPv6 与安全上下文。",
				Difference: "核心 VPS 身份证据已对齐；ECS 的安全上下文和第三方数据库覆盖仍更广。",
				Next:       "继续增加可选 provider，同时保留 provider 级错误和非权威 NAT 语义。",
			},
			{
				Area:       "硬件基准",
				Status:     "partial",
				VMBench:    "默认 sysbench/fio/openssl；可选 dd、STREAM、mbw、Geekbench、WinSAT；缺工具结构化 error，不做进程内 fallback。",
				ECS:        "覆盖 sysbench、geekbench、winsat、stream、mbw、dd、fio 等多种工具。",
				Difference: "核心工具覆盖已接近 ECS；后续差距在自动安装、离线包、多盘/多模式细节。",
				Next:       "补自动安装/离线包、多盘/多模式细节，仍只展示原始指标。",
			},
			{
				Area:       "路由诊断",
				Status:     "partial",
				VMBench:    "版本化 catalog 提供广州/北京/上海/成都、三网、CERNET、CSTNET 与 IPv4/IPv6 route targets，结果保留稳定 node ID、协议和来源。",
				ECS:        "BackTrace/NextTrace 覆盖电信、联通、移动、教育网、科技网、多地区与 IPv4/IPv6。",
				Difference: "核心线路类型已覆盖；ECS 的地区规模、线路识别和 NextTrace 展示更深。",
				Next:       "通过签名 catalog 迭代可验证节点，不加入无法健康检查的推测 endpoint。",
			},
			{
				Area:       "Ping 延迟",
				Status:     "partial",
				VMBench:    "ping section 基于同一版本化 catalog 输出 latency/jitter/loss，覆盖成都、教育网、科技网并支持 v4/v6/dual。",
				ECS:        "提供更多节点和运营商维度的 pingtest 组合。",
				Difference: "输出模型、双栈与节点追踪已对齐；ECS 的公共节点规模仍更大。",
				Next:       "根据 nodes health 结果扩充稳定节点，并继续结构化记录部分/全部失败。",
			},
			{
				Area:       "速度测试",
				Status:     "partial",
				VMBench:    "Cloudflare、Speedtest.net、Speedtest.cn、iperf3 provider，输出 groups/providers/summary。",
				ECS:        "Speedtest.net/cn 多节点、节点自动更新、可指定运营商/地区和结果上传。",
				Difference: "vmbench 已有离线快照、revision pin、Ed25519 签名更新和健康检查；仍缺结果分享上传及更细的 Speedtest 节点选择。",
				Next:       "增加可选且可脱敏的 upload/share，并扩展 provider 自身节点选择。",
			},
			{
				Area:       "IP 质量",
				Status:     "partial",
				VMBench:    "ip_quality section 有 IP 基础信息、DNSBL、邮件端口和 0-100 风险诊断。",
				ECS:        "聚合更多 IP 数据库，覆盖 IPv4/IPv6、邮件、欺诈/安全、地区与 ASN 诊断。",
				Difference: "vmbench 已有业务风险诊断，但第三方数据库覆盖深度不足。",
				Next:       "按 provider 接入更多数据库，保留风险评分与 benchmark 指标隔离。",
			},
			{
				Area:       "流媒体解锁",
				Status:     "partial",
				VMBench:    "media section 覆盖 Netflix、YouTube、Disney+、ChatGPT、TikTok、Prime 等常见探测。",
				ECS:        "集成 UnlockTests/TikTok 等更完整地区化解锁脚本。",
				Difference: "vmbench 有基础模型；ECS 的地区和平台覆盖更全。",
				Next:       "补齐区域化平台清单，并继续以 detail/error 保留探测失败原因。",
			},
			{
				Area:       "网站/TG 可达性",
				Status:     "partial",
				VMBench:    "reachability section 检查 Google、GitHub、Cloudflare HTTPS 与 Telegram DC1-5 TCP，保留协议、endpoint、延迟、HTTP 状态和错误。",
				ECS:        "提供常见网站访问、Twitter 地区、Telegram DC 等网络可用性检查。",
				Difference: "网站和 Telegram 核心可达性已覆盖；Twitter 地区等细分诊断仍少于 ECS。",
				Next:       "按独立 provider 扩展地区诊断，不把可达性转换为综合评分。",
			},
			{
				Area:       "报告与分享",
				Status:     "partial",
				VMBench:    "Suite schema v2、Console/JSON/完整 HTML、本地 history 和 run/suite 自动 compare；节点条件不兼容时拒绝 delta。",
				ECS:        "终端结果、goecs.txt/test_result.txt、分享链接和上传通道。",
				Difference: "vmbench 的机器可读、证据 provenance 和本地对比更强；ECS 的分享传播更强。",
				Next:       "增加可选 upload/share adapter；默认仍本地输出，避免强依赖外部服务。",
			},
			{
				Area:       "交互体验",
				Status:     "vmbench_only",
				VMBench:    "现代 TUI、主题、Suite 配置页、运行页、结果页和 Compare 页。",
				ECS:        "交互菜单成熟，但 README 明确 GUI 暂不适配新版 GoECS。",
				Difference: "vmbench 在终端 UI 表达上是差异化优势。",
				Next:       "继续把 ECS 可借鉴能力落在 TUI Suite，而不是回退到纯脚本菜单。",
			},
			{
				Area:       "结果对比",
				Status:     "vmbench_only",
				VMBench:    "compare 自动识别 run/suite，并基于原始指标判断变化；history 支持本地时间序列。",
				ECS:        "重点是一键测评与分享，非两份结构化报告对比。",
				Difference: "这是 vmbench 的保留优势，符合当前产品方向。",
				Next:       "继续扩展历史趋势展示，同时严格校验单位、协议、provider、目标和节点目录 revision。",
			},
			{
				Area:       "评分策略",
				Status:     "aligned",
				VMBench:    "不输出 benchmark 总分、等级或 category score；IP 风险诊断独立保留。",
				ECS:        "以各工具/模块的测量输出为主，可包含上游工具自己的结果。",
				Difference: "对标时不应把 ECS 的单项结果归一化成 vmbench 总分。",
				Next:       "继续保持原始指标 + 结构化报告 + 对比分析。",
			},
		},
		NextActions: []ecsDiffNextAction{
			{
				Priority: "P2",
				Item:     "增加可选且可脱敏的结果分享",
				Reason:   "补齐 ECS 的传播能力，同时默认保持本地、私有和可复现。",
			},
			{
				Priority: "P2",
				Item:     "扩展 provider 与地区诊断",
				Reason:   "P1 证据模型已稳定，可在不改变 schema 语义的前提下增加 IP、媒体和可达性覆盖。",
			},
			{
				Priority: "P2",
				Item:     "强化硬件工具分发与平台适配",
				Reason:   "补自动安装/离线包、多盘/多模式细节；缺工具时继续结构化记录错误。",
			},
		},
		Notes: []string{
			"该快照用于产品差异输出，不代表运行时 benchmark 结果。",
			"P1/v0.2 已落地节点目录、网络身份/可达性、Suite Compare、History 与完整 HTML。",
			"新增能力时继续遵守 vmbench 原始指标原则，不重新引入综合评分。",
		},
	}
}

func writeECSDiffText(w io.Writer, snapshot ecsDiffSnapshot) error {
	if _, err := fmt.Fprintf(w, "VMBench vs ECS 差异快照（%s）\n", snapshot.SnapshotDate); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "需求: %s\n\n", snapshot.Requirement); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "来源:"); err != nil {
		return err
	}
	for _, source := range snapshot.Sources {
		if _, err := fmt.Fprintf(w, "- %s: %s (%s)\n", source.Name, source.URL, source.Description); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\n图例:"); err != nil {
		return err
	}
	for _, item := range snapshot.Legend {
		if _, err := fmt.Fprintf(w, "- %s: %s\n", item.Status, item.Description); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\n差异:"); err != nil {
		return err
	}
	for i, row := range snapshot.Rows {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%02d. [%s] %s\n", i+1, row.Status, row.Area); err != nil {
			return err
		}
		lines := []struct {
			label string
			value string
		}{
			{"vmbench", row.VMBench},
			{"ECS/GoECS", row.ECS},
			{"差异", row.Difference},
			{"下一步", row.Next},
		}
		for _, line := range lines {
			if _, err := fmt.Fprintf(w, "    %-9s: %s\n", line.label, line.value); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w, "\n建议优先级:"); err != nil {
		return err
	}
	for _, action := range snapshot.NextActions {
		if _, err := fmt.Fprintf(w, "- %s %s：%s\n", action.Priority, action.Item, action.Reason); err != nil {
			return err
		}
	}
	if len(snapshot.Notes) > 0 {
		if _, err := fmt.Fprintln(w, "\n备注:"); err != nil {
			return err
		}
		for _, note := range snapshot.Notes {
			if _, err := fmt.Fprintf(w, "- %s\n", note); err != nil {
				return err
			}
		}
	}
	return nil
}

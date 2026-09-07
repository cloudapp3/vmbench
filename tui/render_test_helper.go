//go:build rendertest

package tui

import (
	"time"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/history"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func WithSysInfoForRender(m Model, info sysinfo.SystemInfo, width, height int) Model {
	m.sysInfo = info
	m.width = width
	m.height = height
	return m
}

func WithPageForRender(m Model, name string) Model {
	switch name {
	case "dashboard":
		m.page = pageDashboard
	case "running":
		m.page = pageRunning
		m.workloads = []workloadState{
			{name: "CPU Single-Core (sysbench)", category: "CPU", status: "done", metric: "532 events/sec"},
			{name: "OpenSSL AES-256-CBC", category: "CPU", status: "done", metric: "1.4 GB/s"},
			{name: "Memory Read Bandwidth (sysbench)", category: "Memory", status: "running"},
			{name: "Disk 1M Sequential Read Q1 (fio)", category: "Disk", status: "waiting"},
			{name: "Disk 4K Random Read Q1 (fio)", category: "Disk", status: "waiting"},
		}
		m.phase = "Single-Core"
		m.engine = "external"
	case "results":
		m.page = pageResults
	case "compare":
		m.page = pageCompare
	case "suite-config":
		m.page = pageSuiteConfig
	case "suite-running":
		m.page = pageSuiteRunning
		m.suiteSections = []suiteSection{
			{id: suite.SectionHardware, label: "Hardware", status: "done", message: "ok"},
			{id: suite.SectionNetworkInfo, label: "Network Info", status: "done", message: "ok"},
			{id: suite.SectionRoute, label: "Route", status: "done", message: "ok"},
			{id: suite.SectionPing, label: "Ping", status: "running"},
			{id: suite.SectionSpeed, label: "Speed", status: "waiting"},
			{id: suite.SectionIPQuality, label: "IP Quality", status: "waiting"},
			{id: suite.SectionReachability, label: "Reachability", status: "waiting"},
			{id: suite.SectionMail, label: "Mail Ports", status: "skip"},
		}
	case "suite-results":
		m.page = pageSuiteResults
	case "help":
		m.helpFrom = pageDashboard
		m.page = pageHelp
	case "run-config":
		m.page = pageRunConfig
	case "compare-picker":
		m.page = pageComparePicker
		m.picker.records = []history.Record{
			{ID: "run-20260907-101010", Kind: history.KindRun, Tag: "baseline", ReportTime: time.Date(2026, 9, 7, 10, 10, 10, 0, time.UTC)},
			{ID: "suite-20260906-220000", Kind: history.KindSuite, Tag: "evening", ReportTime: time.Date(2026, 9, 6, 22, 0, 0, 0, time.UTC)},
			{ID: "run-20260905-090000", Kind: history.KindRun, ReportTime: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)},
		}
		m.picker.a = 0
		m.picker.b = 2
	case "result-detail":
		m.page = pageResultDetail
		m.resultsDetail = 0
		if m.report == nil {
			report := vmbench.Report(gbreport.Document{Results: gbreport.ResultsSection{
				Workloads: []gbreport.WorkloadEntry{
					{
						Name:     "Disk 4K Random Read Q1 (fio)",
						Category: "Disk",
						Result: &gbreport.ResultEntry{
							Iterations:       3,
							MedianMS:         3000,
							SamplesMS:        []float64{2990, 3005, 3008},
							ThroughputPerSec: 18200,
							ThroughputUnit:   "IOPS",
							AvgNSPerAccess:   54000,
							Detail:           "fio-3.36\nread: IOPS=18200, BW=71MiB/s\n  lat (usec): min=48, max=2100, avg=54",
						},
					},
					{
						Name:     "Memory Write Bandwidth (sysbench)",
						Category: "Memory",
						Result:   &gbreport.ResultEntry{Iterations: 3, MedianMS: 1200, Error: "sysbench: command not found"},
					},
				},
			}})
			m.report = &report
		}
	}
	return m
}

func WithReportForRender(m Model, r *vmbench.Report) Model {
	m.report = r
	return m
}

func WithSuiteReportForRender(m Model, r *suite.SuiteReport) Model {
	m.suiteReport = r
	return m
}

func RenderViewForTest(m Model) string {
	return m.View()
}

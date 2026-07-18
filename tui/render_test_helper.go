//go:build rendertest

package tui

import (
	"github.com/cloudapp3/vmbench"
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

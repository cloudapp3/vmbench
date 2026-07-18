package vmbench

import (
	gbbench "github.com/cloudapp3/vmbench/bench"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// Workload is the benchmark workload interface.
type Workload = gbbench.Workload

// BenchResult is one measured workload result.
type BenchResult = gbbench.BenchResult

// RunDetail stores one workload measurement detail.
type RunDetail = gbbench.RunDetail

// ProgressEvent reports benchmark progress.
type ProgressEvent = gbbench.ProgressEvent

// ResultEntry is one workload result entry.
type ResultEntry = gbreport.ResultEntry

// WorkloadEntry is one workload row in the bench report.
type WorkloadEntry = gbreport.WorkloadEntry

// ResultsSection stores core measured workload results.
type ResultsSection = gbreport.ResultsSection

// ExtensionsSection stores optional extension workload results.
type ExtensionsSection = gbreport.ExtensionsSection

// SystemInfo is the collected hardware and OS inventory.
type SystemInfo = sysinfo.SystemInfo

// Report is the bench report returned by RunCore.
type Report = gbreport.Document

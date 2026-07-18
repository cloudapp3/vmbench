//go:build rendertest

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudapp3/vmbench"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/sysinfo"
	"github.com/cloudapp3/vmbench/tui"
	"github.com/cloudapp3/vmbench/tui/theme"
)

func main() {
	page := "dashboard"
	width := 140
	reportPath := ""
	suiteReportPath := ""
	compareA := ""
	compareB := ""
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--page":
			i++
			page = os.Args[i]
		case "--width":
			i++
			fmt.Sscanf(os.Args[i], "%d", &width)
		case "--report":
			i++
			reportPath = os.Args[i]
		case "--suite-report":
			i++
			suiteReportPath = os.Args[i]
		case "--compare-a":
			i++
			compareA = os.Args[i]
		case "--compare-b":
			i++
			compareB = os.Args[i]
		}
	}
	theme.InitThemeFromEnv("")

	info, _ := sysinfo.Collect(context.Background())

	m := tui.NewModel(compareA, compareB)
	m = tui.WithSysInfoForRender(m, info, width, 50)
	m = tui.WithPageForRender(m, page)

	if reportPath != "" {
		data, err := os.ReadFile(reportPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var doc gbreport.Document
		if err := json.Unmarshal(data, &doc); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		report := vmbench.Report(doc)
		m = tui.WithReportForRender(m, &report)
	}

	if suiteReportPath != "" {
		data, err := os.ReadFile(suiteReportPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var sr suite.SuiteReport
		if err := json.Unmarshal(data, &sr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		m = tui.WithSuiteReportForRender(m, &sr)
	}

	fmt.Println(tui.RenderViewForTest(m))
}

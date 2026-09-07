package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
)

func detailReport() *gbreport.Document {
	doc := &gbreport.Document{}
	doc.Results.Workloads = []gbreport.WorkloadEntry{
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
			Result: &gbreport.ResultEntry{
				Iterations: 3,
				MedianMS:   1200,
				Error:      "sysbench: command not found",
			},
		},
	}
	return doc
}

func TestResultDetailOpensFromFlatTab(t *testing.T) {
	m := scrollTestModel(t, pageResults, func(m *Model) {
		m.report = detailReport()
	})
	m.resultsTab = 2 // flat

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.resultsCur != 1 {
		t.Fatalf("flat cursor should move (previously dead), cur = %d", m.resultsCur)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(Model)
	if m.page != pageResultDetail || m.resultsDetail != 1 {
		t.Fatalf("d should open detail of row 1, page = %d detail = %d", m.page, m.resultsDetail)
	}

	view := m.View()
	if !strings.Contains(view, "sysbench: command not found") {
		t.Fatalf("detail should show the error entry:\n%s", view)
	}
	if !strings.Contains(view, i18n.T("tui.detail.errorTitle")) {
		t.Fatalf("detail should show error card title")
	}
	assertRenderBounds(t, view, 80, 24)

	// esc returns and the cursor is preserved.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.page != pageResults || m.resultsCur != 1 {
		t.Fatalf("esc should return to results with cursor kept, page = %d cur = %d", m.page, m.resultsCur)
	}
}

func TestResultDetailRendersRawOutputAndSamples(t *testing.T) {
	m := scrollTestModel(t, pageResultDetail, func(m *Model) {
		m.report = detailReport()
		m.resultsDetail = 0
	})
	// Content assertions run on the unclipped body; the visible window is
	// only 18 lines and the raw-output card sits below the fold.
	content := pageContent(m)
	for _, want := range []string{
		"fio-3.36",
		"IOPS=18200",
		i18n.T("tui.detail.detailTitle"),
		i18n.T("tui.detail.samples"),
		"#1 2990.0ms",
		"71MiB/s",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("detail content missing %q:\n%s", want, content)
		}
	}
	// ...and scrolling brings it into view.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	um := updated.(Model)
	if !strings.Contains(um.View(), "fio-3.36") {
		t.Fatalf("End should scroll the raw output card into view:\n%s", um.View())
	}
	assertRenderBounds(t, m.View(), 80, 24)
}

func TestResultDetailLongLinesClipped(t *testing.T) {
	long := strings.Repeat("x", 300)
	doc := detailReport()
	doc.Results.Workloads[0].Result.Detail = long + "\nsecond line"
	m := scrollTestModel(t, pageResultDetail, func(m *Model) {
		m.report = doc
		m.resultsDetail = 0
	})
	content := pageContent(m)
	if strings.Contains(content, long) {
		t.Fatal("300-cell line must be clipped, not rendered raw")
	}
	if !strings.Contains(content, "second line") {
		t.Fatal("following detail lines must survive clipping")
	}
	assertRenderBounds(t, m.View(), 80, 24)
}

func TestResultDetailZhCNBounds(t *testing.T) {
	i18n.SetLang("zh-CN")
	t.Cleanup(func() { i18n.SetLang("en") })
	m := scrollTestModel(t, pageResultDetail, func(m *Model) {
		m.report = detailReport()
		m.resultsDetail = 0
	})
	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	if !strings.Contains(view, i18n.T("tui.detail.title")) {
		t.Fatalf("zh-CN detail title missing:\n%s", view)
	}
}

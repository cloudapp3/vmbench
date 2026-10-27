package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/tui/theme"
)

// isQuitCmd reports whether cmd is the tea.Quit command (func values are not
// comparable, so invoke it and check the message).
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestCtrlCQuitsFromEveryPage pins the global ctrl+c binding: raw mode clears
// ISIG, so ^C reaches Update as a plain key that no page binds. Before the
// global case existed it was silently swallowed everywhere. The running page
// is the exception: an in-flight run routes ctrl+c through the cancel
// confirmation instead of quitting.
func TestCtrlCQuitsFromEveryPage(t *testing.T) {
	pages := append([]page{pageHelp}, helpPageOrder...)
	for _, p := range pages {
		m := scrollTestModel(t, p, nil)
		cancelled := false
		m.cancel = func() { cancelled = true }

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if p == pageRunning {
			um, ok := updated.(Model)
			if !ok || !um.confirm {
				t.Fatalf("ctrl+c on the running page should open the cancel confirmation, confirm=%v", um.confirm)
			}
			if isQuitCmd(cmd) {
				t.Fatal("ctrl+c on the running page must not quit before confirming")
			}
			if cancelled {
				t.Fatal("ctrl+c on the running page must not cancel before confirming")
			}
			continue
		}
		if !isQuitCmd(cmd) {
			t.Fatalf("ctrl+c on page %d should quit, got cmd %v", p, cmd)
		}
		if um, ok := updated.(Model); !ok || um.page != p {
			t.Fatalf("ctrl+c on page %d should leave the model untouched for teardown", p)
		}
		if !cancelled {
			t.Fatalf("ctrl+c on page %d should cancel the run context", p)
		}
	}
}

// TestCtrlCOnRunningWithoutCancelFunc: reaching the running page without a
// run in flight (nil cancel) still quits instead of hanging on a modal.
func TestCtrlCOnRunningWithoutCancelFunc(t *testing.T) {
	m := scrollTestModel(t, pageRunning, nil)
	if m.cancel != nil {
		t.Fatal("pre-test sanity: model should have no cancel func")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatal("ctrl+c with no in-flight run should quit")
	}
	if um, ok := updated.(Model); !ok || um.confirm {
		t.Fatal("no in-flight run means no confirmation needed")
	}
}

// TestCtrlCBypassesConfirmModal: during the running-page cancel confirmation
// ctrl+c exits outright instead of feeding the y/n prompt.
func TestCtrlCBypassesConfirmModal(t *testing.T) {
	m := runningTestModel(t)
	m.confirm = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatal("ctrl+c should quit even with the confirm modal open")
	}
}

// TestCtrlCWithoutCancelFunc: pages reached before any run starts have a nil
// cancel func; ctrl+c must still quit without panicking.
func TestCtrlCWithoutCancelFunc(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	if m.cancel != nil {
		t.Fatal("pre-test sanity: dashboard model should have no cancel func")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCmd(cmd) {
		t.Fatal("ctrl+c should quit even when no run is in flight")
	}
}

// TestNetIdentityDoneMsgUpdatesModel: the startup identity probe lands after
// the first render; Update must store the identities and clear loading. The
// offline variant (nil identities) clears loading and leaves rows hidden.
func TestNetIdentityDoneMsgUpdatesModel(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	m.netIdent = netIdentState{loading: true}

	updated, cmd := m.Update(netIdentityDoneMsg{
		v4: &netio.PublicIPIdentity{IP: "203.0.113.10", ASN: 64500, Org: "Example Net"},
	})
	if cmd != nil {
		t.Fatal("identity landing should not trigger a cmd")
	}
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if um.netIdent.loading {
		t.Fatal("loading flag should clear after the probe lands")
	}
	if um.netIdent.v4 == nil || um.netIdent.v4.ASN != 64500 {
		t.Fatalf("v4 identity = %+v, want stored identity", um.netIdent.v4)
	}
	if um.netIdent.v6 != nil {
		t.Fatalf("v6 identity = %+v, want nil", um.netIdent.v6)
	}

	updated, _ = um.Update(netIdentityDoneMsg{})
	if um2, ok := updated.(Model); !ok || um2.netIdent.loading || um2.netIdent.v4 != nil || um2.netIdent.v6 != nil {
		t.Fatalf("offline landing should leave identities nil and loading cleared: %+v", um2.netIdent)
	}
}

// TestQMeansBackOnSubpages pins the q semantics: every subpage treats q like
// esc (back one level); only the dashboard keeps q = quit, and the running
// page routes it into the cancel confirmation.
func TestQMeansBackOnSubpages(t *testing.T) {
	cases := []struct {
		page page
		want page
	}{
		{pageConfig, pageDashboard},
		{pageHistory, pageDashboard},
		{pageComparePicker, pageDashboard},
		{pageResults, pageDashboard},
		{pageResultDetail, pageResults},
		{pageCompare, pageComparePicker},
		{pageCheckupResults, pageDashboard},
		{pageHelp, pageDashboard},
	}
	for _, tc := range cases {
		m := scrollTestModel(t, tc.page, nil)
		m.helpFrom = pageDashboard
		updated, cmd := m.Update(keyMsg('q'))
		um, ok := updated.(Model)
		if !ok {
			t.Fatalf("update returned %T", updated)
		}
		if um.page != tc.want {
			t.Fatalf("q on page %d should return to page %d, got %d", tc.page, tc.want, um.page)
		}
		if isQuitCmd(cmd) {
			t.Fatalf("q on page %d must not quit the program", tc.page)
		}
	}

	m := scrollTestModel(t, pageDashboard, nil)
	_, cmd := m.Update(keyMsg('q'))
	if !isQuitCmd(cmd) {
		t.Fatal("q on the dashboard should quit")
	}

	rm := runningTestModel(t)
	updated, cmd := rm.Update(keyMsg('q'))
	if um := updated.(Model); !um.confirm || isQuitCmd(cmd) {
		t.Fatalf("q on the running page should open the cancel confirmation, confirm=%v", um.confirm)
	}
}

// TestQOnRunningConfirmsThenReturnsToDashboard: y on the cancel modal lands
// on the dashboard with the run cancelled.
func TestQOnRunningConfirmsThenReturnsToDashboard(t *testing.T) {
	m := runningTestModel(t)
	cancelled := false
	m.cancel = func() { cancelled = true }

	updated, _ := m.Update(keyMsg('q'))
	m = updated.(Model)
	updated, _ = m.Update(keyMsg('y'))
	um := updated.(Model)
	if !cancelled || um.confirm || um.page != pageDashboard {
		t.Fatalf("confirm should cancel and return to dashboard: cancelled=%v confirm=%v page=%d", cancelled, um.confirm, um.page)
	}
}

// TestGlobalThemeAndLangKeysWorkEverywhere: t/l cycle from any page, exactly
// like the dashboard; only raw-text entry is exempt.
func TestGlobalThemeAndLangKeysWorkEverywhere(t *testing.T) {
	t.Setenv("VMBENCH_LANG", "en")
	defer i18n.SetLang("en")
	for _, p := range append([]page{pageHelp}, helpPageOrder...) {
		m := scrollTestModel(t, p, nil)
		before := theme.Active.Name
		if _, _ = m.Update(keyMsg('t')); theme.Active.Name == before {
			t.Fatalf("t on page %d should cycle the theme", p)
		}
		updated, _ := m.Update(keyMsg('l'))
		um := updated.(Model)
		if um.langExplicit != "en" || i18n.Lang() != "en" {
			t.Fatalf("l on page %d should cycle the language, got %q/%q", p, um.langExplicit, i18n.Lang())
		}
	}

	// Text entry keeps every rune.
	m := scrollTestModel(t, pageComparePicker, nil)
	updated, _ := m.Update(keyMsg('m'))
	m = updated.(Model)
	updated, _ = m.Update(keyMsg('l'))
	um := updated.(Model)
	if um.langExplicit != "" || !strings.Contains(um.picker.pathA.Value(), "l") {
		t.Fatalf("l during text entry must reach the input, got langExplicit=%q path=%q", um.langExplicit, um.picker.pathA.Value())
	}
}

// TestTooSmallThreshold pins the 50x16 floor: 49x15 is blocked, 50x16 renders.
func TestTooSmallThreshold(t *testing.T) {
	m := scrollTestModel(t, pageDashboard, nil)
	m.width, m.height = 49, 15
	if view := m.View(); !strings.Contains(view, i18n.T("tui.tooSmallHint")) {
		t.Fatal("49x15 should be blocked by the too-small guard")
	}
	m.width, m.height = 50, 16
	view := m.View()
	if strings.Contains(view, i18n.T("tui.tooSmallHint")) {
		t.Fatal("50x16 should render instead of the too-small guard")
	}
}

// runnableReport builds a minimal run-shaped report for history round trips.
func runnableReport() vmbench.Report {
	report := vmbench.Report(gbreport.Document{Timestamp: time.Now()})
	report.Results.Workloads = []gbreport.WorkloadEntry{{
		Name: "CPU Single-Core (sysbench)", Category: "CPU",
		Result: &gbreport.ResultEntry{MedianMS: 1010, ThroughputPerSec: 532, ThroughputUnit: "events/sec"},
	}}
	return report
}

// TestBenchmarkDoneSavesHistoryAndJumps: a finished run both lands on the
// results page and is persisted to the history store.
func TestBenchmarkDoneSavesHistoryAndJumps(t *testing.T) {
	fixtureStore(t, 0, 0)
	m := scrollTestModel(t, pageRunning, nil)
	m.runKind = "run"

	updated, cmd := m.Update(benchmarkDoneMsg{report: runnableReport()})
	um := updated.(Model)
	if um.page != pageResults {
		t.Fatalf("finished run should jump to results, page = %d", um.page)
	}
	saved, ok := cmd().(historySavedMsg)
	if !ok || saved.err != nil {
		t.Fatalf("done should persist the report, got %#v", saved)
	}
	records := mustListHistory(t)
	if len(records) != 1 || records[0].Kind != history.KindRun {
		t.Fatalf("store should hold one run record, got %+v", records)
	}
}

// TestCancelledBenchmarkDoneSavesWithoutJump: after the user cancelled back
// to the dashboard, the late done message stays put, saves the partial
// report, and toasts about it.
func TestCancelledBenchmarkDoneSavesWithoutJump(t *testing.T) {
	fixtureStore(t, 0, 0)
	m := scrollTestModel(t, pageDashboard, nil)

	updated, cmd := m.Update(benchmarkDoneMsg{report: runnableReport()})
	um := updated.(Model)
	if um.page != pageDashboard {
		t.Fatalf("late done message must not yank the user to results, page = %d", um.page)
	}
	if !um.toast.Active() || !strings.Contains(um.toast.Message, i18n.T("tui.toast.partialSaved")) {
		t.Fatalf("late done message should toast the partial save, got %+v", um.toast)
	}
	// The batch is [saveHistoryCmd, toastTick]; only execute the save half —
	// the tick would sleep.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) < 1 {
		t.Fatalf("done away from the running page should batch save+toast, got %T", cmd())
	}
	if saved, ok := batch[0]().(historySavedMsg); !ok || saved.err != nil {
		t.Fatalf("save half failed: %#v", saved)
	}
	if records := mustListHistory(t); len(records) != 1 {
		t.Fatalf("partial report should be stored, got %d records", len(records))
	}
}

// TestCheckupDoneSavesHistory: the checkup path persists too.
func TestCheckupDoneSavesHistory(t *testing.T) {
	fixtureStore(t, 0, 0)
	m := scrollTestModel(t, pageRunning, nil)
	m.runKind = "checkup"

	report := checkup.CheckupReport{SchemaVersion: 2, ReportKind: "checkup"}
	report.Config.Preset = "quick"
	updated, cmd := m.Update(checkupDoneMsg{report: report})
	um := updated.(Model)
	if um.page != pageCheckupResults {
		t.Fatalf("finished checkup should jump to its results, page = %d", um.page)
	}
	if saved, ok := cmd().(historySavedMsg); !ok || saved.err != nil {
		t.Fatalf("checkup done should persist the report, got %#v", saved)
	}
	records := mustListHistory(t)
	if len(records) != 1 || records[0].Kind != history.KindCheckup {
		t.Fatalf("store should hold one checkup record, got %+v", records)
	}
}

func mustListHistory(t *testing.T) []history.Record {
	t.Helper()
	store, err := history.Open("")
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

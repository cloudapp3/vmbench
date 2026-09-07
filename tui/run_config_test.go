package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/catalog"
	"github.com/cloudapp3/vmbench/i18n"
)

func TestRunConfigDefaultsMatchCLI(t *testing.T) {
	s := newRunConfigState()

	if s.iterations != 3 {
		t.Fatalf("default iterations = %d, want 3", s.iterations)
	}
	got := s.buildOptions()
	want := (vmbench.Options{
		Mode:          "single",
		Engine:        "external",
		Scope:         vmbench.ScopeHardware,
		Iterations:    3,
		HardwareTools: catalog.DefaultHardwareTools(),
	})
	if got.Iterations != want.Iterations || got.Scope != want.Scope ||
		got.Mode != want.Mode || got.Engine != want.Engine || got.Filter != "" {
		t.Fatalf("buildOptions = %+v, want base %+v", got, want)
	}
	if strings.Join(got.HardwareTools, ",") != strings.Join(want.HardwareTools, ",") {
		t.Fatalf("default tools = %v, want %v", got.HardwareTools, want.HardwareTools)
	}

	if norm, err := vmbench.NormalizeOptions(got); err != nil {
		t.Fatalf("default options must normalize: %v", err)
	} else if norm.Iterations != 3 {
		t.Fatalf("normalized iterations = %d", norm.Iterations)
	}
}

func TestRunConfigNavigationAndToggles(t *testing.T) {
	m := scrollTestModel(t, pageRunConfig, nil)

	// Iterations: right twice → 5.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRight})
	um := updated.(Model)
	if um.runConfig.iterations != 5 {
		t.Fatalf("iterations = %d, want 5", um.runConfig.iterations)
	}

	// Down to tools; move inner cursor; toggle off sysbench.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyDown})
	um = updated.(Model)
	if um.runConfig.field != fieldRunTools {
		t.Fatalf("field = %d, want tools", um.runConfig.field)
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeySpace})
	um = updated.(Model)
	if um.runConfig.tools[catalog.HardwareToolSysbench] {
		t.Fatal("space should deselect sysbench (cursor at 0)")
	}
	// plannedWorkloads drops sysbench rows.
	if planned := um.runConfig.plannedWorkloads(); len(planned) == 0 {
		t.Fatal("planned workloads should not be empty with openssl+fio")
	} else {
		for _, d := range planned {
			if strings.Contains(d.Name, "sysbench") {
				t.Fatalf("sysbench workload still planned: %s", d.Name)
			}
		}
	}

	// Filter chips: down to filter, right to CPU.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyDown})
	um = updated.(Model)
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRight})
	um = updated.(Model)
	if um.runConfig.filterChip != 1 || um.runConfig.buildOptions().Filter != "CPU" {
		t.Fatalf("filter chip = %d expr = %q, want 1/CPU", um.runConfig.filterChip, um.runConfig.buildOptions().Filter)
	}
	for _, d := range um.runConfig.plannedWorkloads() {
		if d.Category != "CPU" {
			t.Fatalf("CPU filter planned non-CPU workload %q (%s)", d.Name, d.Category)
		}
	}
}

func TestRunConfigCustomFilterTextEntry(t *testing.T) {
	m := scrollTestModel(t, pageRunConfig, nil)
	// Jump to filter field and cycle to Custom chip.
	m.runConfig.field = fieldRunFilter
	m.runConfig.filterChip = 4

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fio")})
	um := updated.(Model)
	if um.runConfig.filterText != "fio" {
		t.Fatalf("filterText = %q, want fio", um.runConfig.filterText)
	}

	// "q" must be typed, not quit.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	um = updated.(Model)
	if um.runConfig.filterText != "fioq" {
		t.Fatalf("filterText = %q, want fioq", um.runConfig.filterText)
	}

	// "?" must not open help while typing; it is typed into the field.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	um = updated.(Model)
	if um.page != pageRunConfig {
		t.Fatalf("? leaked during text entry, page = %d", um.page)
	}
	if um.runConfig.filterText != "fioq?" {
		t.Fatalf("filterText = %q, want fioq?", um.runConfig.filterText)
	}

	// Backspace removes one rune.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	um = updated.(Model)
	if um.runConfig.filterText != "fioq" {
		t.Fatalf("filterText = %q after backspace, want fioq", um.runConfig.filterText)
	}

	// plannedWorkloads honors the regex.
	um.runConfig.filterText = "fio"
	planned := um.runConfig.plannedWorkloads()
	if len(planned) == 0 {
		t.Fatal("fio filter should plan fio workloads")
	}
	for _, d := range planned {
		if !strings.Contains(d.Name, "fio") {
			t.Fatalf("fio filter planned non-fio workload %q", d.Name)
		}
	}
}

func TestRunConfigStartEmitsHardwareStart(t *testing.T) {
	m := scrollTestModel(t, pageRunConfig, nil)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a cmd emitting hardwareStartMsg")
	}
	msg := cmd()
	start, ok := msg.(hardwareStartMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want hardwareStartMsg", msg)
	}
	if start.opts.Scope != vmbench.ScopeHardware || start.opts.Iterations != 3 {
		t.Fatalf("start opts = %+v", start.opts)
	}
	if _, isModel := updated.(Model); !isModel {
		t.Fatalf("update returned %T", updated)
	}
}

func TestRunConfigInvalidRegexShowsToast(t *testing.T) {
	m := scrollTestModel(t, pageRunConfig, nil)
	m.runConfig.field = fieldRunFilter
	m.runConfig.filterChip = 4
	m.runConfig.filterText = "("

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	if cmd == nil {
		t.Fatal("invalid regex should surface a toast cmd")
	}
	if !um.toast.Active() {
		t.Fatal("invalid regex should show a toast")
	}
}

func TestRunConfigPreflightAndRender(t *testing.T) {
	m := scrollTestModel(t, pageRunConfig, nil)

	updated, _ := m.Update(missingToolsMsg{missing: []string{"mbw"}})
	um := updated.(Model)
	if !um.runConfig.missingOK || len(um.runConfig.missing) != 1 {
		t.Fatalf("preflight state = %+v", um.runConfig)
	}
	view := um.View()
	if !strings.Contains(view, "mbw") {
		t.Fatalf("preflight card should list missing tool:\n%s", view)
	}

	// Every field focus must fit 80x24 in both locales.
	for f := fieldRunIterations; f <= fieldRunStart; f++ {
		mm := scrollTestModel(t, pageRunConfig, nil)
		mm.runConfig.field = f
		mm.runConfig.missingOK = true
		assertRenderBounds(t, mm.View(), 80, 24)
	}

	i18n.SetLang("zh-CN")
	t.Cleanup(func() { i18n.SetLang("en") })
	for f := fieldRunIterations; f <= fieldRunStart; f++ {
		mm := scrollTestModel(t, pageRunConfig, nil)
		mm.runConfig.field = f
		mm.runConfig.missingOK = true
		assertRenderBounds(t, mm.View(), 80, 24)
	}
}

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudapp3/vmbench/bench/netio"
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
// global case existed it was silently swallowed everywhere.
func TestCtrlCQuitsFromEveryPage(t *testing.T) {
	pages := append([]page{pageHelp}, helpPageOrder...)
	for _, p := range pages {
		m := scrollTestModel(t, p, nil)
		cancelled := false
		m.cancel = func() { cancelled = true }

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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

package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func withMarket(f fakes) {
	f.mk.repos = []Repo{
		{Repo: "o/tdd-guard", List: "testing", Description: "Test-driven development guard that blocks code written before its failing test.", Topics: []string{"tdd", "claude-code"}, Stars: 31234, Items: 12, Followed: true},
		{Repo: "o/lint-all", List: "testing", Description: "Linting skills.", Stars: 842, Items: 3},
		{Repo: "x/review-kit", List: "review", Description: "Review agents.", Topics: []string{"review"}, Stars: 120, Items: 7},
		{Repo: "o/tdd-guard", List: "review", Description: "Test-driven development guard that blocks code written before its failing test.", Stars: 31234, Items: 12, Followed: true},
		{Repo: "me/mine", List: "Other", Stars: 5, Items: 1, Followed: true},
	}
}

func marketModel(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withMarket)
	press(t, m, "down", "4")
	return m, f
}

func TestMarketplaceLoadsAsynchronouslyOnFirstVisit(t *testing.T) {
	m, f := newTestWith(t, withMarket)
	if !strings.Contains(view(m), "MARKETPLACE (…)") || f.mk.listCalls != 0 {
		t.Fatalf("unloaded:\n%s", view(m))
	}
	press(t, m, "down")
	_, cmd := m.Update(key("4"))
	if v := view(m); !strings.Contains(v, "loading the marketplace…") {
		t.Fatalf("loading:\n%s", v)
	}
	if cmd == nil || f.mk.listCalls != 0 {
		t.Fatalf("the list is read by a command, not inline: cmd %v calls %d", cmd, f.mk.listCalls)
	}
	drain(t, m, cmd)
	if v := view(m); !strings.Contains(v, "MARKETPLACE (4)") || strings.Contains(v, "loading the marketplace") || f.mk.listCalls != 1 {
		t.Errorf("loaded:\n%s", v)
	}
	press(t, m, "1", "4")
	if f.mk.listCalls != 1 {
		t.Errorf("a loaded tab is not read again: %d", f.mk.listCalls)
	}
}

func TestMarketplaceTable(t *testing.T) {
	m, _ := marketModel(t)
	v := view(m)
	for _, want := range []string{"REPO", "LIST", "ITEMS", "★", "── testing (2)", "── review (2)", "── Other (1)", "o/tdd-guard", "★31.2k", "★842", "4 repos · 2 followed · 2 lists"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "✓") {
		t.Errorf("followed repos carry a check:\n%s", v)
	}
}

func TestMarketplaceDetail(t *testing.T) {
	m, _ := marketModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	v := view(m)
	for _, want := range []string{"Test-driven development guard", "tdd", "claude-code", "testing, review", "followed yes"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail missing %q:\n%s", want, v)
		}
	}
}

func TestMarketplaceSelectionSkipsDividers(t *testing.T) {
	m, _ := marketModel(t)
	var seen []string
	for range 6 {
		seen = append(seen, m.selectedRepo().Repo+"/"+m.selectedRepo().List)
		press(t, m, "down")
	}
	want := []string{"o/tdd-guard/testing", "o/lint-all/testing", "x/review-kit/review", "o/tdd-guard/review", "me/mine/Other", "me/mine/Other"}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Errorf("down visits %v, want %v", seen, want)
	}
	press(t, m, "pgup")
	if r := m.selectedRepo(); r == nil || r.Repo != "o/tdd-guard" || r.List != "testing" {
		t.Errorf("pgup lands on a repo: %+v", r)
	}
	press(t, m, "pgdown")
	if r := m.selectedRepo(); r == nil || r.Repo != "me/mine" {
		t.Errorf("pgdown lands on the last repo: %+v", r)
	}
}

func TestMarketplaceFilterHidesEmptyGroups(t *testing.T) {
	m, _ := marketModel(t)
	press(t, m, "/")
	typeText(t, m, "review")
	v := view(m)
	if !strings.Contains(v, "── review (2)") || strings.Contains(v, "── testing") || strings.Contains(v, "── Other") {
		t.Errorf("only the matching group stays:\n%s", v)
	}
	if !strings.Contains(v, "filter: review") {
		t.Errorf("filter text in the title:\n%s", v)
	}
	press(t, m, "enter")
	if !strings.Contains(view(m), "4 repos · 2 followed · 2 lists · filter: review") {
		t.Errorf("stats strip names the filter:\n%s", view(m))
	}
	if r := m.selectedRepo(); r == nil || r.Repo != "x/review-kit" {
		t.Errorf("selection %+v", r)
	}
	press(t, m, "esc")
	if m.mk.filter.value() != "" || !strings.Contains(view(m), "── testing (2)") {
		t.Errorf("esc clears the filter:\n%s", view(m))
	}
}

func TestMarketplaceFilterNothingMatches(t *testing.T) {
	m, f := marketModel(t)
	press(t, m, "/")
	typeText(t, m, "zzz")
	if v := view(m); !strings.Contains(v, "no repo matches the filter") {
		t.Errorf("empty filter:\n%s", v)
	}
	press(t, m, "enter", "space")
	if len(f.mk.followed) != 0 {
		t.Errorf("space with no repo does nothing: %v", f.mk.followed)
	}
}

func TestMarketplaceSpaceTypesInFilter(t *testing.T) {
	m, f := marketModel(t)
	press(t, m, "/")
	typeText(t, m, "a")
	press(t, m, " ", "b")
	if m.mk.filter.in.Value() != "a b" || len(f.mk.followed) != 0 {
		t.Errorf("filter %q, follows %v", m.mk.filter.in.Value(), f.mk.followed)
	}
	press(t, m, "s", "1", "q")
	if m.tab != tabMarketplace || m.inst != nil || f.mk.syncs != 0 {
		t.Errorf("global keys stay inert while typing: tab %v inst %v syncs %d", m.tab, m.inst, f.mk.syncs)
	}
}

func TestMarketplaceFollowAndUnfollow(t *testing.T) {
	m, f := marketModel(t)
	press(t, m, "down", " ")
	if len(f.mk.followed) != 1 || f.mk.followed[0] != "+o/lint-all" {
		t.Fatalf("follow calls %v", f.mk.followed)
	}
	if m.status != "following o/lint-all — press s to sync" || !m.statusOK {
		t.Errorf("status %q ok %v", m.status, m.statusOK)
	}
	if v := view(m); !strings.Contains(v, "3 followed") {
		t.Errorf("stats follow the change:\n%s", v)
	}
	press(t, m, " ")
	if f.mk.followed[1] != "-o/lint-all" || m.status != "unfollowed o/lint-all — press s to sync" {
		t.Errorf("unfollow: %v status %q", f.mk.followed, m.status)
	}
	if !strings.Contains(view(m), "2 followed") {
		t.Errorf("stats:\n%s", view(m))
	}
}

func TestMarketplaceFollowUpdatesEveryListOfTheRepo(t *testing.T) {
	m, _ := marketModel(t)
	press(t, m, " ")
	for _, r := range m.mk.rows {
		if r.Repo == "o/tdd-guard" && r.Followed {
			t.Errorf("both entries of o/tdd-guard are unfollowed: %+v", m.mk.rows)
		}
	}
}

func TestMarketplaceFollowErrorGoesToStatus(t *testing.T) {
	m, f := marketModel(t)
	f.mk.followErr = errors.New("repo not found")
	press(t, m, "down", " ")
	if m.statusOK || !strings.Contains(m.status, "repo not found") {
		t.Errorf("status %q", m.status)
	}
	if v := view(m); !strings.Contains(v, "repo not found") || !strings.Contains(v, "2 followed") {
		t.Errorf("view:\n%s", v)
	}
}

func TestMarketplaceListError(t *testing.T) {
	m, f := newTestWith(t, func(f fakes) { withMarket(f); f.mk.listErr = errors.New("offline") })
	press(t, m, "down", "4")
	v := view(m)
	if !strings.Contains(v, "could not load the marketplace: offline") || !strings.Contains(v, "MARKETPLACE (…)") {
		t.Errorf("error in the body, count unknown:\n%s", v)
	}
	press(t, m, "2")
	if !strings.Contains(view(m), "PACKS") {
		t.Errorf("other tabs work:\n%s", view(m))
	}
	f.mk.listErr = nil
	press(t, m, "4")
	if v := view(m); f.mk.listCalls != 2 || !strings.Contains(v, "MARKETPLACE (4)") || strings.Contains(v, "offline") {
		t.Errorf("revisit retries:\n%s", v)
	}
}

func TestMarketplaceSyncStreamsAndSaysToRestart(t *testing.T) {
	m, f := marketModel(t)
	f.mk.lines = []string{"fetching…", "done"}
	press(t, m, "s")
	v := view(m)
	if f.mk.syncs != 1 || m.inst == nil || !m.inst.done || m.inst.title != "sync" || !strings.Contains(v, "ipm catalog sync") || !strings.Contains(v, "fetching…") {
		t.Fatalf("sync modal:\n%s", v)
	}
	if m.status != "catalog synced — restart ipm tui to search the new items" || !m.statusOK {
		t.Errorf("status %q ok %v", m.status, m.statusOK)
	}
	if f.mk.listCalls != 2 {
		t.Errorf("the list is reloaded once after a sync: %d", f.mk.listCalls)
	}
	press(t, m, "esc")
	if m.inst != nil || !strings.Contains(view(m), syncedMsg) {
		t.Errorf("the status outlives the key that closes the modal:\n%s", view(m))
	}
}

func TestMarketplaceSyncFailureKeepsTheList(t *testing.T) {
	m, f := marketModel(t)
	f.mk.syncErr = errors.New("pnpm failed")
	press(t, m, "s")
	if !strings.Contains(view(m), "✗ pnpm failed") || strings.Contains(m.status, "catalog synced") || f.mk.listCalls != 1 {
		t.Errorf("failed sync:\n%s\nstatus %q calls %d", view(m), m.status, f.mk.listCalls)
	}
}

func TestMarketplaceSyncRefusedWhileBusy(t *testing.T) {
	m, f := marketModel(t)
	f.b.gate = make(chan struct{})
	cmd := m.startBuild()
	if cmd == nil || !m.busy() {
		t.Fatal("build should be streaming")
	}
	if c := m.marketKey(key("s")); c != nil || f.mk.syncs != 0 || m.status != busyMsg {
		t.Errorf("syncs %d status %q", f.mk.syncs, m.status)
	}
	close(f.b.gate)
	drain(t, m, cmd)
}

func TestMarketplaceHelp(t *testing.T) {
	m, _ := marketModel(t)
	press(t, m, "?")
	v := view(m)
	for _, want := range []string{"space", "follow", "sync"} {
		if !strings.Contains(v, want) {
			t.Errorf("help missing %q:\n%s", want, v)
		}
	}
}

func TestMarketplaceFooter(t *testing.T) {
	m, _ := marketModel(t)
	if v := view(m); !strings.Contains(v, "space follow") || !strings.Contains(v, "s sync") {
		t.Errorf("footer:\n%s", v)
	}
}

func TestMarketplaceUnfollowLastFilteredRowKeepsSelectionInRange(t *testing.T) {
	m, _ := marketModel(t)
	press(t, m, "/", "✓", "enter", "down", "down")
	if r := m.selectedRepo(); r == nil || r.Repo != "me/mine" {
		t.Fatalf("selection %+v", r)
	}
	press(t, m, " ")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_ = view(m) // must not panic
	if m.mk.sel != len(m.visibleRepos())-1 {
		t.Errorf("sel %d of %d", m.mk.sel, len(m.visibleRepos()))
	}
}

func TestMarketplaceNilDepDoesNotPanic(t *testing.T) {
	m, _ := newTest(t)
	m.d.Marketplace = nil
	press(t, m, "down", "4")
	if !strings.Contains(view(m), "could not load the marketplace: no marketplace available") {
		t.Errorf("view:\n%s", view(m))
	}
}

func TestMarketplaceLoadingAndErrorFooterShowOnlyWorkingKeys(t *testing.T) {
	m, _ := newTestWith(t, withMarket)
	press(t, m, "down")
	m.Update(key("4"))
	if v := view(m); strings.Contains(v, "space follow") || !strings.Contains(v, "s sync") {
		t.Errorf("loading footer:\n%s", v)
	}
	m, _ = newTestWith(t, func(f fakes) { f.mk.listErr = errors.New("x") })
	press(t, m, "down", "4")
	if v := view(m); strings.Contains(v, "space follow") {
		t.Errorf("error footer:\n%s", v)
	}
}

func TestFinishingARunDropsAStaleStatus(t *testing.T) {
	m, _ := newTest(t)
	m.d.Builder.(*fakeBuilder).lines = []string{"ok"}
	m.status, m.statusOK = "stale", true
	drain(t, m, m.startBuild())
	if m.status != "" {
		t.Errorf("finishing a run drops the old status: %q", m.status)
	}
}

func TestSyncWithoutMarketplaceSaysSo(t *testing.T) {
	m, _ := newTest(t)
	m.d.Marketplace = nil
	press(t, m, "down", "4", "s")
	if m.inst != nil || !strings.Contains(m.status, "no marketplace available") {
		t.Errorf("inst %v status %q", m.inst != nil, m.status)
	}
}

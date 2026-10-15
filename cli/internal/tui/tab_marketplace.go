package tui

import (
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// marketTab is the Marketplace tab's state: the curated repos, selection and
// filter.
type marketTab struct {
	rows    []Repo
	err     error // the last load's error
	loaded  bool
	loading bool // the first load, or a retry, is in flight
	filter  filterBox

	sel, offset int // sel indexes the visible repos, not the divider rows
}

// marketLoaded carries a marketplace load back to the model.
type marketLoaded struct {
	repos []Repo
	err   error
}

// syncedMsg is the status after a successful catalog sync.
const syncedMsg = "catalog synced — restart ipm tui to search the new items"

var marketCols = []column{
	{Title: "✓", Min: 1, Max: 1},
	{Title: "REPO", Min: 12, Max: 36},
	{Title: "LIST", Min: 6},
	{Title: "★", Min: 2, Max: 7, Right: true},
	{Title: "ITEMS", Min: 5, Max: 6, Right: true},
}

// cells are a repo's visible column text.
func (r Repo) cells() []string {
	check, star := "", ""
	if r.Followed {
		check = "✓"
	}
	if r.Stars > 0 {
		star = "★" + stars(r.Stars)
	}
	return []string{check, r.Repo, r.List, star, strconv.Itoa(r.Items)}
}

// loadMarket reads the marketplace in the background: the real one fetches
// the curated lists over HTTP.
func (m *Model) loadMarket() tea.Cmd {
	mk := m.d.Marketplace
	if mk == nil {
		return func() tea.Msg { return marketLoaded{err: errors.New("no marketplace available")} }
	}
	return func() tea.Msg {
		repos, err := mk.List()
		return marketLoaded{repos, err}
	}
}

// marketVisit starts the load on the tab's first visit and retries after an
// error; a load already in flight is left alone.
func (m *Model) marketVisit() tea.Cmd {
	mt := &m.mk
	if mt.loading || (mt.loaded && mt.err == nil) {
		return nil
	}
	mt.loading, mt.err = true, nil
	return m.loadMarket()
}

// marketReady applies a finished load, keeping the selection on the same
// repo of the same list when it is still there.
func (m *Model) marketReady(msg marketLoaded) {
	mt := &m.mk
	keep := m.selectedRepo()
	mt.loaded, mt.loading, mt.err = true, false, msg.err
	if msg.err != nil {
		mt.rows = nil
		m.counts[tabMarketplace] = notLoaded
		return
	}
	mt.rows = msg.repos
	m.counts[tabMarketplace] = len(m.marketStats().repos)
	mt.sel = 0
	for i, r := range m.visibleRepos() {
		if keep != nil && r.Repo == keep.Repo && r.List == keep.List {
			mt.sel = i
		}
	}
	m.clampMarket()
}

// visibleRepos are the rows the filter keeps, in list order.
func (m *Model) visibleRepos() []Repo {
	var out []Repo
	for _, r := range m.mk.rows {
		if m.mk.filter.match(r.cells()) {
			out = append(out, r)
		}
	}
	return out
}

// selectedRepo is the repo under the cursor, nil when there is none.
func (m *Model) selectedRepo() *Repo {
	vis := m.visibleRepos()
	if m.mk.sel < 0 || m.mk.sel >= len(vis) {
		return nil
	}
	return &vis[m.mk.sel]
}

func (m *Model) clampMarket() {
	m.mk.sel = max(0, min(m.mk.sel, len(m.visibleRepos())-1))
}

// marketKey handles a key press on the Marketplace tab.
func (m *Model) marketKey(msg tea.KeyPressMsg) tea.Cmd {
	mt, k := &m.mk, msg.String()
	if mt.filter.typing {
		if mt.filter.key(msg) {
			mt.sel, mt.offset = 0, 0
			m.clampMarket()
		}
		return nil
	}
	page := max(1, m.height-12)
	switch k {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
	case "/":
		mt.filter.focus()
	case "down", "j":
		mt.sel = max(0, min(mt.sel+1, len(m.visibleRepos())-1))
	case "up", "k":
		mt.sel = max(0, mt.sel-1)
	case "pgdown":
		mt.sel = max(0, min(mt.sel+page, len(m.visibleRepos())-1))
	case "pgup":
		mt.sel = max(0, mt.sel-page)
	case "esc":
		if mt.filter.value() != "" {
			mt.filter.clear()
			mt.sel, mt.offset = 0, 0
			m.clampMarket()
		}
	case "space", " ":
		m.toggleFollow()
	case "s":
		return m.startSync()
	}
	return nil
}

// toggleFollow follows the selected repo, or unfollows it when it is
// followed, and shows the change in every row of the repo.
func (m *Model) toggleFollow() {
	r := m.selectedRepo()
	if r == nil {
		m.status = "no repo selected"
		return
	}
	var err error
	verb := "following"
	if r.Followed {
		verb = "unfollowed"
		err = m.d.Marketplace.Unfollow(r.Repo)
	} else {
		err = m.d.Marketplace.Follow(r.Repo)
	}
	if err != nil {
		m.status, m.statusOK = err.Error(), false
		return
	}
	on := !r.Followed
	for i := range m.mk.rows {
		if m.mk.rows[i].Repo == r.Repo {
			m.mk.rows[i].Followed = on
		}
	}
	m.clampMarket()
	m.note("%s %s — press s to sync", verb, r.Repo)
}

// startSync runs ipm catalog sync in the output modal.
func (m *Model) startSync() tea.Cmd {
	if m.refuseBusy() {
		return nil
	}
	if m.d.Marketplace == nil {
		m.status, m.statusOK = "no marketplace available", false
		return nil
	}
	return m.stream(runSync, "sync", "ipm catalog sync", "✓ synced", func(w io.Writer) error {
		return m.d.Marketplace.Sync(context.Background(), w)
	})
}

// marketTotals are the repos, followed repos and lists the tab holds; a repo
// of two lists counts once.
type marketTotals struct {
	repos, followed, lists []string
}

func (m *Model) marketStats() marketTotals {
	var t marketTotals
	for _, r := range m.mk.rows {
		if !slices.Contains(t.repos, r.Repo) {
			t.repos = append(t.repos, r.Repo)
			if r.Followed {
				t.followed = append(t.followed, r.Repo)
			}
		}
		if r.List != "Other" && !slices.Contains(t.lists, r.List) {
			t.lists = append(t.lists, r.List)
		}
	}
	return t
}

// marketplaceStats is the Marketplace tab's strip: repos, followed repos and
// lists, then the filter while set.
func (m *Model) marketplaceStats() string {
	mt := &m.mk
	if !mt.loaded || mt.err != nil {
		return ""
	}
	t := m.marketStats()
	parts := []string{count(len(t.repos), "repo"), strconv.Itoa(len(t.followed)) + " followed", count(len(t.lists), "list")}
	s := strings.Join(parts, stMuted.Render(" · "))
	if f := mt.filter.value(); f != "" {
		s += stMuted.Render(" · filter: ") + stAmber.Render(f)
	}
	return s
}

// marketHints are the Marketplace footer's keys, before help and quit.
func (m *Model) marketHints() []string {
	if m.mk.filter.typing {
		return []string{"type to filter", "enter keep", "esc clear"}
	}
	if !m.mk.loaded || m.mk.err != nil {
		return []string{"s sync", "1-4 tabs"}
	}
	return []string{"↑↓ move", "space follow", "s sync", "/ filter", "1-4 tabs"}
}

// marketRow is a row of the table: a list's divider or a repo.
type marketRow struct {
	divider string // the divider's text; "" for a repo
	repo    int    // the index among the visible repos
}

// marketRows are the table rows of vis: each run of one list under a divider
// that counts it. A list with no visible repo has no divider.
func marketRows(vis []Repo) []marketRow {
	var out []marketRow
	for i, r := range vis {
		if i == 0 || vis[i-1].List != r.List {
			n := 0
			for _, o := range vis[i:] {
				if o.List != r.List {
					break
				}
				n++
			}
			out = append(out, marketRow{divider: "── " + r.List + " (" + strconv.Itoa(n) + ")"})
		}
		out = append(out, marketRow{repo: i})
	}
	return out
}

// marketBody is the table of repos, with the selected one's detail beside it
// when the screen is wide, h lines in all.
func (m *Model) marketBody(w, h int) []string {
	mt := &m.mk
	message := func(line string) []string {
		lines := []string{line}
		for len(lines) < h-2 {
			lines = append(lines, "")
		}
		return panel(stMuted.Render("marketplace"), lines, w, true)
	}
	if mt.err != nil {
		return message(stError.Render("could not load the marketplace: " + mt.err.Error()))
	}
	if !mt.loaded {
		return message(stMuted.Render("loading the marketplace…"))
	}
	vis := m.visibleRepos()
	switch {
	case len(vis) == 0 && mt.filter.value() != "":
		return message(stMuted.Render("no repo matches the filter — esc clears it"))
	case len(vis) == 0:
		return message(stMuted.Render("the marketplace is empty — add sources with ipm source add"))
	}
	wide := w >= wideWidth
	tw := w
	if wide {
		tw = w * 62 / 100
	}
	var rows [][]string
	var styles [][]lipgloss.Style
	selRow, divider := 0, -1
	for _, row := range marketRows(vis) {
		if row.divider != "" {
			divider = len(rows)
			rows = append(rows, []string{row.divider})
			styles = append(styles, []lipgloss.Style{stMuted})
			continue
		}
		r := vis[row.repo]
		if row.repo == mt.sel {
			selRow = len(rows)
			// Keep the divider in view when the selection opens its group.
			if divider == selRow-1 && mt.offset >= selRow {
				mt.offset = divider
			}
		}
		rows = append(rows, r.cells())
		styles = append(styles, []lipgloss.Style{stTeal, stText, stMuted, starStyle(r.Stars), stMuted})
	}
	title := stMuted.Render("marketplace")
	if f := mt.filter.line(); f != "" {
		mt.filter.in.SetWidth(max(10, tw-30))
		title += stMuted.Render(" ── ") + f
	}
	list := panel(title, table(marketCols, rows, styles, selRow, &mt.offset, tw-4, h-2, true), tw, true)
	if !wide {
		return list
	}
	return joinPanels(list, m.marketDetail(vis[mt.sel], w-tw, h))
}

// marketDetail is the panel of one repo in full, w wide and h lines high.
func (m *Model) marketDetail(r Repo, w, h int) []string {
	var lines []string
	add := func(name, value string) {
		if value != "" {
			lines = append(lines, stMuted.Render(fit(name, 9))+stText.Render(value))
		}
	}
	var lists []string
	for _, o := range m.mk.rows {
		if o.Repo == r.Repo && !slices.Contains(lists, o.List) {
			lists = append(lists, o.List)
		}
	}
	add("lists", strings.Join(lists, ", "))
	add("stars", stars(r.Stars))
	add("items", strconv.Itoa(r.Items))
	if r.Followed {
		add("followed", "yes")
	}
	add("topics", strings.Join(r.Topics, ", "))
	if r.Description != "" {
		lines = append(lines, "")
		for _, l := range strings.Split(ansi.Wrap(r.Description, max(1, w-6), ""), "\n") {
			lines = append(lines, stText.Render(l))
		}
	}
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	return panel(stTitle.Render(r.Repo), lines[:h-2], w, false)
}

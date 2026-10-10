package tui

import (
	"fmt"
	"sort"
)

// extRow is one external reference in the e modal.
type extRow struct {
	ref    Ref
	stars  int
	market string
}

// extModal lists the external references to a repo.
type extModal struct {
	repo  string
	repos int
	rows  []extRow
	sel   int
	off   int // first row shown
}

// openExternal lists every external reference to the cursor's repo, grouped
// by the repo it comes from.
func (m *Model) openExternal() {
	node := m.tree.Node
	if node.Repo == "" {
		return
	}
	if node.Type != "repo" {
		if n, err := m.d.Catalog.Node("repo:" + node.Repo); err == nil {
			node = n
		}
	}
	em := &extModal{repo: node.Repo}
	repos := map[string]bool{}
	for _, r := range node.UsedBy {
		row := extRow{ref: r}
		if n, err := m.d.Catalog.Node("repo:" + r.Repo); err == nil {
			row.stars = n.Stars
		}
		if n, err := m.d.Catalog.Node(r.ID); err == nil && len(n.Parents) > 0 {
			row.market = n.Parents[0].Name
		}
		repos[r.Repo] = true
		em.rows = append(em.rows, row)
	}
	sort.SliceStable(em.rows, func(i, j int) bool {
		a, b := em.rows[i].ref, em.rows[j].ref
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		return a.Name < b.Name
	})
	em.repos = len(repos)
	m.ext = em
}

// extKey handles keys while the e modal is open.
func (m *Model) extKey(k string) {
	em := m.ext
	switch k {
	case "esc", "e":
		m.ext = nil
	case "down", "j":
		em.sel = max(0, min(len(em.rows)-1, em.sel+1))
	case "up", "k":
		em.sel = max(0, em.sel-1)
	case "enter":
		if em.sel >= 0 && em.sel < len(em.rows) {
			m.backStack = append(m.backStack, m.cursor)
			if len(m.backStack) > maxBack {
				m.backStack = m.backStack[len(m.backStack)-maxBack:]
			}
			m.ext = nil
			m.setCursor(em.rows[em.sel].ref.ID)
		}
	}
}

// extLines is the e modal in place of the body: exactly rows lines.
func (m *Model) extLines(w, rows int) []string {
	em := m.ext
	var lines []string
	if len(em.rows) == 0 {
		lines = append(lines, stTitle.Render("no other repo references "+em.repo), "", stMuted.Render("esc close"))
	} else {
		noun := "repos"
		if em.repos == 1 {
			noun = "repo"
		}
		lines = append(lines, stTitle.Render(fmt.Sprintf("External references to %s — used by %d %s", em.repo, em.repos, noun)), "")
		// Scroll so the selection stays in view between the header and hint.
		room := max(1, rows-4)
		if em.sel < em.off {
			em.off = em.sel
		}
		if em.sel >= em.off+room {
			em.off = em.sel - room + 1
		}
		for i, r := range em.rows {
			if i < em.off || i >= em.off+room {
				continue
			}
			star := ""
			if r.stars > 0 {
				star = "★" + stars(r.stars)
			}
			market := ""
			if r.market != "" {
				market = "(marketplace " + r.market + ")"
			}
			text := fit(r.ref.Repo, 34) + " " + starStyle(r.stars).Render(fit(star, 8)) + " " + fit(r.ref.Name, 24) + " " + stMuted.Render(market)
			if i == em.sel {
				lines = append(lines, stSelected.Render("▶ ")+text)
			} else {
				lines = append(lines, "  "+text)
			}
		}
		lines = append(lines, "", stMuted.Render("enter jump · esc close"))
	}
	out := make([]string, 0, rows)
	for _, l := range lines {
		if len(out) == rows {
			break
		}
		out = append(out, fit(l, w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}

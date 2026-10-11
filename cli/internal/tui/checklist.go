package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// CuratedRepo is one repo in a curated list, as the checklist shows it.
type CuratedRepo struct {
	Repo, Description, Language string
	Stars                       int
	LastPush                    time.Time
	Topics                      []string
}

// CuratedList is a named group of curated repos.
type CuratedList struct {
	Name  string
	Repos []CuratedRepo
}

// Checklist is ipm onboard's picker: curated lists that expand into their
// repos, each with a tick box. Repos are keyed by lower-case owner/repo, so
// a repo in two lists is ticked in both.
type Checklist struct {
	lists          []CuratedList
	ticked, locked map[string]bool
	now            time.Time
	expanded       map[int]bool

	cursor, off   int
	filter        string
	typing        bool
	confirming    bool
	confirmed     bool
	notice        string
	width, height int
	plain         bool
}

// ckRow is one visible row: a list (repo < 0) or a repo in it.
type ckRow struct{ list, repo int }

// NewChecklist starts with ticked repos ticked; locked repos (sources the
// user already follows) show ✓, can't be unticked, and aren't picks.
func NewChecklist(lists []CuratedList, ticked, locked map[string]bool, now time.Time) *Checklist {
	c := &Checklist{lists: lists, ticked: map[string]bool{}, locked: map[string]bool{}, now: now, expanded: map[int]bool{}, width: 100, height: 24}
	for k, v := range ticked {
		c.ticked[strings.ToLower(k)] = v
	}
	for k, v := range locked {
		c.locked[strings.ToLower(k)] = v
	}
	c.plain = os.Getenv("NO_COLOR") != ""
	return c
}

func ckKey(r CuratedRepo) string { return strings.ToLower(r.Repo) }

// Picked is every ticked repo that isn't locked, in list and row order,
// each once.
func (c *Checklist) Picked() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range c.lists {
		for _, r := range l.Repos {
			k := ckKey(r)
			if c.ticked[k] && !c.locked[k] && !seen[k] {
				seen[k] = true
				out = append(out, r.Repo)
			}
		}
	}
	return out
}

// Confirmed reports whether the user pressed enter, then y.
func (c *Checklist) Confirmed() bool { return c.confirmed }

func (c *Checklist) Init() tea.Cmd { return nil }

func (c *Checklist) matches(r CuratedRepo) bool {
	if c.filter == "" {
		return true
	}
	f := strings.ToLower(c.filter)
	if strings.Contains(strings.ToLower(r.Repo), f) || strings.Contains(strings.ToLower(r.Description), f) {
		return true
	}
	for _, t := range r.Topics {
		if strings.Contains(strings.ToLower(t), f) {
			return true
		}
	}
	return false
}

// rows is what is visible: every list (only those with matches while
// filtering) and the repos of expanded lists (every list while filtering).
func (c *Checklist) rows() []ckRow {
	var out []ckRow
	for i, l := range c.lists {
		var repos []ckRow
		for j, r := range l.Repos {
			if c.matches(r) {
				repos = append(repos, ckRow{i, j})
			}
		}
		if c.filter != "" && len(repos) == 0 {
			continue
		}
		out = append(out, ckRow{i, -1})
		if c.filter != "" || c.expanded[i] {
			out = append(out, repos...)
		}
	}
	return out
}

func (c *Checklist) current() (ckRow, bool) {
	rows := c.rows()
	if len(rows) == 0 {
		return ckRow{}, false
	}
	c.cursor = min(max(c.cursor, 0), len(rows)-1)
	return rows[c.cursor], true
}

// setList ticks (or unticks) every unlocked repo in list i.
func (c *Checklist) setList(i int, on bool) {
	for _, r := range c.lists[i].Repos {
		if k := ckKey(r); !c.locked[k] {
			c.ticked[k] = on
		}
	}
}

// listState is "x" when every repo in list i is ticked or locked, "~" when
// some are, " " when none are.
func (c *Checklist) listState(i int) string {
	n := 0
	for _, r := range c.lists[i].Repos {
		if k := ckKey(r); c.ticked[k] || c.locked[k] {
			n++
		}
	}
	switch {
	case n == 0:
		return " "
	case n == len(c.lists[i].Repos):
		return "x"
	}
	return "~"
}

func (c *Checklist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width, c.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return c, c.key(msg)
	}
	return c, nil
}

func (c *Checklist) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	c.notice = ""
	if c.typing {
		switch k {
		case "enter":
			c.typing = false
		case "esc":
			c.typing, c.filter = false, ""
		case "backspace":
			if r := []rune(c.filter); len(r) > 0 {
				c.filter = string(r[:len(r)-1])
			}
		default:
			if msg.Text != "" {
				c.filter += msg.Text
			}
		}
		c.cursor = 0
		return nil
	}
	if c.confirming {
		switch k {
		case "y":
			c.confirmed = true
			return tea.Quit
		case "esc", "n":
			c.confirming = false
		case "q":
			return tea.Quit
		}
		return nil
	}
	row, ok := c.current()
	switch k {
	case "q":
		return tea.Quit
	case "up", "k":
		c.cursor--
	case "down", "j":
		c.cursor++
	case "right", "l":
		if ok && row.repo < 0 {
			c.expanded[row.list] = true
		}
	case "left", "h":
		if ok && c.filter == "" {
			c.expanded[row.list] = false
			if row.repo >= 0 {
				for i, r := range c.rows() {
					if r == (ckRow{row.list, -1}) {
						c.cursor = i
					}
				}
			}
		}
	case "space", " ":
		if !ok {
			break
		}
		if row.repo < 0 {
			c.setList(row.list, c.listState(row.list) != "x")
		} else if k := ckKey(c.lists[row.list].Repos[row.repo]); !c.locked[k] {
			c.ticked[k] = !c.ticked[k]
		}
	case "a", "n":
		if ok {
			c.setList(row.list, k == "a")
		}
	case "/":
		c.typing = true
	case "esc":
		c.filter = ""
		c.cursor = 0
	case "enter":
		if len(c.Picked()) == 0 {
			c.notice = "tick at least one repo"
		} else {
			c.confirming = true
		}
	}
	c.current()
	return nil
}

// age is how long ago t was: 3d, 2mo, 1y; — for no date.
func age(now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	days := max(0, int(now.Sub(t).Hours()/24))
	switch {
	case days < 30:
		return strconv.Itoa(days) + "d"
	case days < 365:
		return strconv.Itoa(days/30) + "mo"
	}
	return strconv.Itoa(days/365) + "y"
}

func (c *Checklist) View() tea.View {
	content := c.render()
	if c.plain {
		content = ansi.Strip(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (c *Checklist) render() string {
	w := max(20, c.width)
	seen := map[string]bool{}
	for _, l := range c.lists {
		for _, r := range l.Repos {
			seen[ckKey(r)] = true
		}
	}
	title := stTitle.Render(fmt.Sprintf("Choose sources — %d repos in %d lists", len(seen), len(c.lists)))
	picked := stAmber.Render(fmt.Sprintf("%d picked", len(c.Picked())))
	gap := max(1, w-lipgloss.Width(title)-lipgloss.Width(picked))
	lines := []string{fit(title+spaces(gap)+picked, w)}

	rows := c.rows()
	body := max(1, c.height-3)
	if c.cursor < c.off {
		c.off = c.cursor
	}
	if c.cursor >= c.off+body {
		c.off = c.cursor - body + 1
	}
	c.off = max(0, min(c.off, max(0, len(rows)-body)))
	for i := c.off; i < len(rows) && i < c.off+body; i++ {
		row := c.rowText(rows[i], w-caretWidth, i == c.cursor)
		if i == c.cursor {
			lines = append(lines, selectedRow(row, w, true))
		} else {
			lines = append(lines, plainRow(row, w))
		}
	}
	if len(rows) == 0 {
		lines = append(lines, plainRow(stMuted.Render("no repo matches “"+c.filter+"”"), w))
	}
	for len(lines) < body+1 {
		lines = append(lines, spaces(w))
	}

	switch {
	case c.notice != "":
		lines = append(lines, fit(stError.Render(c.notice), w))
	case c.typing:
		lines = append(lines, fit(stAmber.Render("/ ")+c.filter+"▏", w))
	case c.filter != "":
		lines = append(lines, fit(stMuted.Render("filter: ")+c.filter+stMuted.Render(" · esc clears"), w))
	default:
		lines = append(lines, spaces(w))
	}
	foot := "space tick · ←→ collapse/expand · / filter · a all · n none · enter continue · q quit"
	if w < 100 {
		foot = "space tick · ←→ expand · / filter · a/n all/none · enter go · q quit"
	}
	if c.confirming {
		foot = fmt.Sprintf("%d repos to follow — y continue · esc back", len(c.Picked()))
		lines = append(lines, fit(stAmber.Render(foot), w))
	} else {
		lines = append(lines, fit(stMuted.Render(foot), w))
	}
	return strings.Join(lines, "\n")
}

// rowText draws one row, w cells wide (without the caret column).
func (c *Checklist) rowText(r ckRow, w int, selected bool) string {
	l := c.lists[r.list]
	if r.repo < 0 {
		sum := 0
		for _, x := range l.Repos {
			sum += x.Stars
		}
		name := stTitle
		if selected {
			name = stSelected
		}
		nw := min(44, max(10, w-24))
		return "[" + c.listState(r.list) + "] " + fit(name.Render(l.Name), nw) +
			stMuted.Render(fmt.Sprintf("%4d %-5s", len(l.Repos), plural(len(l.Repos), "repo"))) + "   " + starStyle(sum).Render("★"+stars(sum))
	}
	x := l.Repos[r.repo]
	k := ckKey(x)
	box := "[ ]"
	switch {
	case c.locked[k]:
		box = stTeal.Render("[✓]")
	case c.ticked[k]:
		box = "[x]"
	}
	repo := lipgloss.NewStyle().Foreground(colRepo)
	if selected {
		repo = stSelected
	}
	s := "    " + box + " " + fit(repo.Render(x.Repo), 28) + " " + fit(starStyle(x.Stars).Render("★"+stars(x.Stars)), 7) + " "
	if c.width >= 100 {
		lang := x.Language
		if lang == "" {
			lang = "—"
		}
		s += fit(stMuted.Render(lang), 10) + " " + fit(stMuted.Render(age(c.now, x.LastPush)), 4) + " "
	}
	return s + fit(stText.Render(x.Description), max(0, w-lipgloss.Width(s)))
}

// plural is word, with an s unless n is 1.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

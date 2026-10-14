package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestProgressLine(t *testing.T) {
	now := onboardNow
	var out bytes.Buffer
	p := &progress{w: &out, width: 80, steps: 5, start: now, now: func() time.Time { return now }}
	p.begin(3, "stats")
	p.count(17, 42, "anthropics/skills")
	now = now.Add(48 * time.Second)
	// two steps done and 17/42 of the third: 2.4 of 5, so 9 of 20 cells
	want := "[" + strings.Repeat("█", 9) + strings.Repeat("░", 11) + "] 3/5 stats · 17/42 anthropics/skills · 0:48"
	if got := p.line(); got != want {
		t.Errorf("line =\n%q, want\n%q", got, want)
	}

	p.width = 60
	p.begin(2, "sync")
	p.Write([]byte("pnpm install --no-frozen-lockfile --ignore-scripts --store-dir /somewhere/long\n\n"))
	got := p.line()
	if w := ansi.StringWidth(got); w > 59 {
		t.Errorf("line is %d columns, want it inside a 60-column terminal:\n%s", w, got)
	}
	if !strings.Contains(got, "2/5 sync · pnpm install") || !strings.HasSuffix(got, "… · 0:48") {
		t.Errorf("want the step's last line cut short and the time kept:\n%s", got)
	}

	// a line counts once it ends, so a table written cell by cell doesn't flicker
	p.Write([]byte("skill"))
	p.Write([]byte("  3"))
	if strings.Contains(p.line(), "skill") {
		t.Errorf("an unfinished line should not show:\n%s", p.line())
	}
	p.Write([]byte("\n\n9 imported, 0 failed in 1s — collection \"ipm\"\n"))
	if p.summary() != " · 9 imported, 0 failed in 1s" {
		t.Errorf("summary = %q", p.summary())
	}
}

func TestProgressNilIsSilent(t *testing.T) {
	var p *progress
	p.begin(1, "sync")
	p.count(1, 2, "x")
	p.close()
	if p.summary() != "" || p.output() != "" {
		t.Error("a nil progress should report nothing")
	}
}

func TestOnboardProgressInTerminal(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/hooks", "o/skills", "o/tdd")
	f.width = 80
	f.onPath["gh"] = true
	f.results["gh auth"] = result{"Logged in", nil}
	f.results["gh api"] = result{ghRepoJSON, nil}
	if err := f.run("onboard", "--yes", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{
		"] 2/5 sync · pnpm install",
		"✓ sync · - search not loaded: run `ipm catalog serve` then `ipm catalog rebuild`\n", // runSync now ends with loadIfUp; Task 7 reworks onboard
		"] 3/5 stats · 3/3 o/",
		"✓ stats · 3 fetched, 0 skipped, 0 failed\n",
		"- search skipped: start Docker, then ipm serve && ipm load\n",
		"✓ library ready — 3 sources, 3 artifacts\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%q", want, out)
		}
	}
	// the steps' own output stays out of the way
	for _, hidden := range []string{"sources synced\n", "catalog/\n", "\n\n"} {
		if strings.Contains(out, hidden) {
			t.Errorf("output should not hold %q:\n%q", hidden, out)
		}
	}
	if !strings.HasSuffix(out, "\r\x1b[K"+"✓ library ready — 3 sources, 3 artifacts\ntry: ipm search tdd · ipm search testing · ipm search hooks · ipm tui\n") {
		t.Errorf("want the bar cleared before the last lines:\n%q", out)
	}
}

func TestOnboardProgressShowsOutputOnFailure(t *testing.T) {
	f, list := onboardFake(t)
	f.width = 80
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404 nope", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("onboard", "--yes", "--list", list); !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("got %v, want exit 1\n%s", err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "\nERR_PNPM_FETCH_404 nope\n✗ pnpm failed — the catalog is unchanged\n✗ sync\nrerun: ipm sync\n") {
		t.Errorf("want sync's whole output, then the failure:\n%q", out)
	}
}

func TestOnboardWithoutTerminalHasNoBar(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/hooks", "o/skills", "o/tdd")
	if err := f.run("onboard", "--yes", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if strings.ContainsAny(out, "\r\x1b") || !strings.Contains(out, "✓ sources synced\n") || !strings.Contains(out, "✓ sync\n\n") {
		t.Errorf("want the plain output:\n%q", out)
	}
}

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/paths"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// existing is what an earlier ipm setup holds.
type existing struct {
	Sources, Artifacts int
	Synced             time.Time
}

// detect reports whether home has sources or the cache has a catalog, and
// what they hold.
func detect(p paths.Paths) (existing, bool, error) {
	var e existing
	srcs, err := sources.Load(p.Sources())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return e, false, err
	}
	e.Sources = len(srcs)
	info, err := os.Stat(filepath.Join(p.Catalog(), catalog.Files[catalog.TypeRepo]))
	hasCatalog := err == nil
	if hasCatalog {
		e.Synced = info.ModTime()
		for _, typ := range catalog.Types {
			if typ == catalog.TypeRepo {
				continue
			}
			if recs, err := catalog.Load(p.Catalog(), typ); err == nil {
				e.Artifacts += len(recs)
			}
		}
	}
	return e, e.Sources > 0 || hasCatalog, nil
}

// String is the one-line summary: 26 sources · 3,280 artifacts · synced
// 2026-10-09.
func (e existing) String() string {
	s := fmt.Sprintf("%s %s · %s %s", thousands(e.Sources), plural(e.Sources, "source"), thousands(e.Artifacts), plural(e.Artifacts, "artifact"))
	if !e.Synced.IsZero() {
		s += " · synced " + e.Synced.Format("2006-01-02")
	}
	return s
}

// plural is word, with an s unless n is 1.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// thousands formats n with commas: 3280 → 3,280.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// backup renames home, then the cache, to <dir>.backup-YYYYMMDD-HHMM (with
// -2, -3… when taken), skipping one that doesn't exist and a cache that
// lived inside home. It returns the new paths. If the cache can't be
// moved, home is moved back, so a failure changes nothing.
func backup(p paths.Paths, now time.Time) ([]string, error) {
	var moved []string
	var undo func()
	for i, dir := range []string{p.Home, p.Cache} {
		if i == 1 && inside(dir, p.Home) {
			continue
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		to := backupName(dir, now)
		if err := os.Rename(dir, to); err != nil {
			if undo != nil {
				undo()
			}
			return nil, fmt.Errorf("can't back up %s: %v — nothing was changed", dir, err)
		}
		from := dir
		undo = func() { _ = os.Rename(to, from) }
		moved = append(moved, to)
	}
	return moved, nil
}

// inside reports whether path is dir or below it.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func backupName(dir string, now time.Time) string {
	base := dir + ".backup-" + now.Format("20060102-1504")
	name := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(name); errors.Is(err, os.ErrNotExist) {
			return name
		}
		name = base + "-" + strconv.Itoa(n)
	}
}

// restoreHint says how to put backed-up folders back.
func restoreHint(p paths.Paths, moved []string) string {
	var b strings.Builder
	b.WriteString("to restore: ipm uninstall, then")
	for i, m := range moved {
		orig := p.Cache
		if strings.HasPrefix(m, p.Home+".backup-") {
			orig = p.Home
		}
		if i > 0 {
			b.WriteString(" &&")
		}
		fmt.Fprintf(&b, " mv %s %s", m, orig)
	}
	return b.String()
}

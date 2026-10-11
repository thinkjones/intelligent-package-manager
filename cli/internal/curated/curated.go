// Package curated reads curated source lists: TOML files of [[repos]]
// tables, each naming a GitHub repo and the list it belongs to, such as
// thinkjones/agentic-tools's starred_repos.toml. ipm onboard offers them
// as the sources a new user can pick from.
package curated

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultURL is the curated list ipm onboard offers unless told otherwise.
const DefaultURL = "https://raw.githubusercontent.com/thinkjones/agentic-tools/main/starred_repos.toml"

// githubBlob matches a GitHub file page: github.com/<owner>/<repo>/blob/<ref>/<path>.
var githubBlob = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/blob/([^/]+)/([^?#]+)`)

// rawURL turns a GitHub file page into the URL of the file itself; other
// URLs are returned as they are.
func rawURL(u string) string {
	m := githubBlob.FindStringSubmatch(u)
	if m == nil {
		return u
	}
	return "https://raw.githubusercontent.com/" + m[1] + "/" + m[2] + "/" + m[3] + "/" + m[4]
}

// Repo is one curated repo; List is the list it was read under.
type Repo struct {
	Repo, List, Description, Language string
	Stars                             int
	LastPush                          time.Time
	Topics                            []string
}

// List is a named group of repos, sorted by stars, most first.
type List struct {
	Name  string
	Repos []Repo
}

var ownerRepoRE = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// Parse reads a curated list. Lists keep the order they first appear in;
// rows without a valid owner/repo are skipped and counted. Every field
// other than repo, list, stars, description, language, last_push and
// topics is ignored.
func Parse(data []byte) (lists []List, skipped int, err error) {
	var doc struct {
		Repos []map[string]any `toml:"repos"`
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil, 0, err
	}
	index := map[string]int{}
	for _, row := range doc.Repos {
		repo, _ := row["repo"].(string)
		list, _ := row["list"].(string)
		if !ownerRepoRE.MatchString(repo) || list == "" {
			skipped++
			continue
		}
		r := Repo{Repo: repo, List: list, LastPush: date(row["last_push"])}
		r.Description, _ = row["description"].(string)
		r.Language, _ = row["language"].(string)
		switch n := row["stars"].(type) {
		case int64:
			r.Stars = int(n)
		case float64:
			r.Stars = int(n)
		}
		if ts, ok := row["topics"].([]any); ok {
			for _, t := range ts {
				if s, ok := t.(string); ok {
					r.Topics = append(r.Topics, s)
				}
			}
		}
		i, ok := index[list]
		if !ok {
			i = len(lists)
			index[list] = i
			lists = append(lists, List{Name: list})
		}
		lists[i].Repos = append(lists[i].Repos, r)
	}
	for _, l := range lists {
		sort.SliceStable(l.Repos, func(a, b int) bool {
			if l.Repos[a].Stars != l.Repos[b].Stars {
				return l.Repos[a].Stars > l.Repos[b].Stars
			}
			return l.Repos[a].Repo < l.Repos[b].Repo
		})
	}
	return lists, skipped, nil
}

// date reads last_push as a TOML date or date-time (both decode to
// time.Time) or a string; anything else is the zero time.
func date(v any) time.Time {
	switch d := v.(type) {
	case time.Time:
		return d
	case string:
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, d); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// Fetch reads src, a file path or an http(s) URL. A URL that answers with a
// list that parses is cached under cacheDir/curated; when it fails, the
// cached copy is returned with cached set. Paths are read as they are.
func Fetch(client *http.Client, src, cacheDir string) (data []byte, cached bool, err error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		data, err = os.ReadFile(src)
		return data, false, err
	}
	src = rawURL(src)
	sum := sha256.Sum256([]byte(src))
	path := filepath.Join(cacheDir, "curated", hex.EncodeToString(sum[:])[:16]+".toml")
	data, ferr := get(client, src)
	if ferr == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
		return data, false, nil
	}
	if old, err := os.ReadFile(path); err == nil {
		return old, true, nil
	}
	return nil, false, fmt.Errorf("can't read the curated list at %s: %v — pass a local copy with --list <path>", src, ferr)
}

// get fetches url and checks that the body is a curated list with at least
// one list in it.
func get(client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	lists, _, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("not a curated list: %v", err)
	}
	if len(lists) == 0 {
		return nil, errors.New("not a curated list: no [[repos]] with a repo and a list")
	}
	return data, nil
}

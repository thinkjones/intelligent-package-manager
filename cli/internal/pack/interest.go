package pack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// interest is the root manifest.toml: upstream repos worth watching for
// plugins, skills, hooks and agents. ipm sync pins and caches them alongside
// the packs' upstreams; ipm scan reports what each one holds. They publish
// nothing.
type interest struct {
	Repo []struct {
		Repo    string `toml:"repo"`
		Version string `toml:"version"`
	} `toml:"repo"`
}

// LoadInterest reads the repos of interest from root's manifest.toml, sorted
// by key; a missing file is none.
func LoadInterest(root string) ([]Upstream, error) {
	path := filepath.Join(root, ManifestFile)
	var f interest
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown key(s) %s", path, strings.Join(keys, ", "))
	}
	seen := map[string]bool{}
	var ups []Upstream
	for i, r := range f.Repo {
		u := Upstream{Repo: r.Repo, Version: r.Version}
		switch {
		case u.Repo == "":
			return nil, fmt.Errorf("%s: repo[%d]: repo is required", path, i)
		case seen[u.Key()]:
			return nil, fmt.Errorf("%s: repo[%d]: %s is listed twice", path, i, u.Key())
		}
		seen[u.Key()] = true
		ups = append(ups, u)
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].Key() < ups[j].Key() })
	return ups, nil
}

// NormalizeRepo writes a GitHub repo as owner/repo, whether given as that,
// an https URL (with or without .git) or an ssh URL. Anything else is
// returned unchanged.
func NormalizeRepo(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(s, prefix); ok {
			parts := strings.Split(strings.Trim(rest, "/"), "/")
			if len(parts) >= 2 {
				return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
			}
		}
	}
	return s
}

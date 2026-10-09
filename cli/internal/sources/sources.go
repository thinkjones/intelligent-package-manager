// Package sources keeps your repos of interest as a pnpm project: one git
// dependency per repo in <home>/sources/package.json, aliased owner--repo.
// pnpm fetches, pins (pnpm-lock.yaml) and stores them; ipm reads what it
// installed in node_modules/<alias> and the commit pnpm pinned.
package sources

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

// Source is one repo of interest.
type Source struct {
	// Alias is its key in package.json and its folder in node_modules.
	Alias string
	// Repo is GitHub owner/repo, or a git URL, as written.
	Repo string
	// Ref is a tag or branch to track; empty means the default branch.
	Ref string
	// Spec is the pnpm specifier: github:Owner/Repo[#ref] or git+URL[#ref].
	Spec string
}

var (
	ownerRepo = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)
	gitURL    = regexp.MustCompile(`^(https://|ssh://|file://|git@)\S+$`)
	unsafe    = regexp.MustCompile(`[^a-z0-9._-]+`)
	sha       = regexp.MustCompile(`[0-9a-f]{40}`)
)

// Alias is the package name pnpm installs repo under: owner--repo, lowercased.
func Alias(repo string) string {
	r := strings.ToLower(strings.TrimSuffix(pack.NormalizeRepo(repo), ".git"))
	for _, p := range []string{"https://", "ssh://", "file://", "git@"} {
		r = strings.TrimPrefix(r, p)
	}
	r = strings.ReplaceAll(strings.ReplaceAll(r, ":", "/"), "/", "--")
	return strings.Trim(unsafe.ReplaceAllString(r, "-"), "-.")
}

// parse reads "owner/repo[@ref]" or "URL[#ref]".
func parse(arg string) (Source, error) {
	arg = strings.TrimSpace(arg)
	var repo, ref string
	switch {
	case gitURL.MatchString(arg):
		repo, ref, _ = strings.Cut(arg, "#")
		repo = pack.NormalizeRepo(repo)
	default:
		repo, ref, _ = strings.Cut(arg, "@")
	}
	s := Source{Repo: repo, Ref: ref}
	switch {
	case ownerRepo.MatchString(repo):
		s.Spec = "github:" + repo
	case gitURL.MatchString(repo):
		u := repo
		if strings.HasPrefix(u, "git@") {
			host, path, _ := strings.Cut(strings.TrimPrefix(u, "git@"), ":")
			u = "ssh://git@" + host + "/" + path
		}
		s.Spec = "git+" + u
	default:
		return Source{}, fmt.Errorf("%q is not owner/repo or a git URL", arg)
	}
	if ref != "" {
		s.Spec += "#" + ref
	}
	s.Alias = Alias(repo)
	return s, nil
}

type packageJSON struct {
	Name         string            `json:"name"`
	Private      bool              `json:"private"`
	Dependencies map[string]string `json:"dependencies"`
}

// rootPrefix starts the sources project's own name. A random suffix makes it
// unguessable, so no source can pass for the root in the pnpmfile's
// readPackage guard.
const rootPrefix = "ipm-sources-"

func newRootName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return rootPrefix + hex.EncodeToString(b)
}

func read(dir string) (packageJSON, error) {
	pj := packageJSON{Name: newRootName(), Private: true, Dependencies: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if errors.Is(err, os.ErrNotExist) {
		return pj, nil
	}
	if err != nil {
		return pj, err
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return pj, fmt.Errorf("%s: %v", filepath.Join(dir, "package.json"), err)
	}
	if pj.Dependencies == nil {
		pj.Dependencies = map[string]string{}
	}
	if !strings.HasPrefix(pj.Name, rootPrefix) {
		pj.Name = newRootName() // also migrates the guessable "ipm-sources"
	}
	return pj, nil
}

// RootName is the sources project's own package name, which the pnpmfile
// exempts from its readPackage hook. It is written to package.json if the
// file exists but lacks one.
func RootName(dir string) (string, error) {
	data, statErr := os.ReadFile(filepath.Join(dir, "package.json"))
	pj, err := read(dir)
	if err != nil {
		return "", err
	}
	if statErr == nil && !strings.Contains(string(data), `"`+pj.Name+`"`) {
		if err := write(dir, pj); err != nil {
			return "", err
		}
	}
	return pj.Name, nil
}

func write(dir string, pj packageJSON) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pj, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path+".tmp", append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// fromSpec turns a package.json entry back into a Source.
func fromSpec(alias, spec string) Source {
	s := Source{Alias: alias, Spec: spec}
	rest := spec
	switch {
	case strings.HasPrefix(spec, "github:"):
		rest = strings.TrimPrefix(spec, "github:")
	case strings.HasPrefix(spec, "git+"):
		rest = strings.TrimPrefix(spec, "git+")
	}
	s.Repo, s.Ref, _ = strings.Cut(rest, "#")
	return s
}

// Load lists the sources in dir, sorted by alias; no package.json is none.
func Load(dir string) ([]Source, error) {
	pj, err := read(dir)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(pj.Dependencies))
	for a := range pj.Dependencies {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	var out []Source
	for _, a := range aliases {
		out = append(out, fromSpec(a, pj.Dependencies[a]))
	}
	return out, nil
}

// Add adds repos ("owner/repo[@ref]" or "URL[#ref]"). A repo already listed,
// or one whose alias collides with another's, is refused and nothing is
// written.
func Add(dir string, repos []string) ([]Source, error) {
	pj, err := read(dir)
	if err != nil {
		return nil, err
	}
	var added []Source
	for _, arg := range repos {
		s, err := parse(arg)
		if err != nil {
			return nil, err
		}
		if spec, taken := pj.Dependencies[s.Alias]; taken {
			old := fromSpec(s.Alias, spec)
			if old.Repo == s.Repo {
				return nil, fmt.Errorf("%s is already a source", s.Repo)
			}
			return nil, fmt.Errorf("%s and %s would both install as %q — remove one first", s.Repo, old.Repo, s.Alias)
		}
		pj.Dependencies[s.Alias] = s.Spec
		added = append(added, s)
	}
	return added, write(dir, pj)
}

// AddSkippingExisting adds repos, skipping any already listed (same repo,
// in any spelling); it returns what it added and what it skipped. A
// different repo that collides on alias is still an error.
func AddSkippingExisting(dir string, repos []string) ([]Source, []string, error) {
	pj, err := read(dir)
	if err != nil {
		return nil, nil, err
	}
	var fresh, skipped []string
	for _, r := range repos {
		s, err := parse(r)
		if err != nil {
			return nil, nil, err
		}
		if spec, taken := pj.Dependencies[s.Alias]; taken && strings.EqualFold(fromSpec(s.Alias, spec).Repo, s.Repo) {
			skipped = append(skipped, r)
			continue
		}
		fresh = append(fresh, r)
	}
	if len(fresh) == 0 {
		return nil, skipped, nil
	}
	added, err := Add(dir, fresh)
	return added, skipped, err
}

// Remove drops repos, matched by alias; an unknown repo is an error and
// nothing is written.
func Remove(dir string, repos []string) error {
	pj, err := read(dir)
	if err != nil {
		return err
	}
	for _, r := range repos {
		s, err := parse(r)
		if err != nil {
			return err
		}
		if _, ok := pj.Dependencies[s.Alias]; !ok {
			return fmt.Errorf("%s is not a source", r)
		}
		delete(pj.Dependencies, s.Alias)
	}
	return write(dir, pj)
}

// Checkout is a synced source: where pnpm installed it and the commit it
// pinned.
type Checkout struct {
	Source
	Dir, Commit string
}

// Checkouts is every source as pnpm installed it. A source pnpm hasn't
// installed or pinned to a commit is an error saying to run `ipm sync`.
func Checkouts(dir string) ([]Checkout, error) {
	srcs, err := Load(dir)
	if err != nil {
		return nil, err
	}
	var lock struct {
		Importers map[string]struct {
			Dependencies map[string]struct {
				Version string `yaml:"version"`
			} `yaml:"dependencies"`
		} `yaml:"importers"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "pnpm-lock.yaml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && len(srcs) > 0 {
			return nil, fmt.Errorf("sources are not synced yet — run `ipm sync`")
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("%s: %v", filepath.Join(dir, "pnpm-lock.yaml"), err)
	}
	deps := lock.Importers["."].Dependencies
	var out []Checkout
	for _, s := range srcs {
		commit := sha.FindString(deps[s.Alias].Version)
		if commit == "" {
			return nil, fmt.Errorf("%s has no pinned commit in pnpm-lock.yaml — run `ipm sync`", s.Repo)
		}
		// pnpm links node_modules/<alias> to its virtual store; walkers don't
		// descend through a symlinked root, so hand out the real folder.
		d, err := filepath.EvalSymlinks(filepath.Join(dir, "node_modules", s.Alias))
		if err != nil {
			return nil, fmt.Errorf("%s is not installed — run `ipm sync`", s.Repo)
		}
		out = append(out, Checkout{Source: s, Dir: d, Commit: commit})
	}
	return out, nil
}

// Package github fetches repo stats — stars, forks, description, topics — for
// the catalog, through the gh CLI so the user's own login is used. Stats
// change daily, so they live in their own file, catalog/github.jsonl,
// refreshed only by `ipm marketplace stats`.
package github

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Stats is one repo's GitHub metadata. Fields are in JSON key order, so each
// line's keys are sorted.
type Stats struct {
	Archived      bool     `json:"archived"`
	DefaultBranch string   `json:"default_branch"`
	Description   string   `json:"description"`
	FetchedAt     string   `json:"fetched_at"`
	Forks         int      `json:"forks"`
	License       string   `json:"license"`
	PushedAt      string   `json:"pushed_at"`
	Repo          string   `json:"repo"`
	Stars         int      `json:"stars"`
	Topics        []string `json:"topics"`
}

// GH runs gh with args and returns its output.
type GH func(args ...string) (string, error)

// Fetch asks GitHub for ownerRepo's stats.
func Fetch(gh GH, ownerRepo string, now time.Time) (Stats, error) {
	out, err := gh("api", "repos/"+ownerRepo)
	if err != nil {
		return Stats{}, fmt.Errorf("%s: gh api: %v: %s", ownerRepo, err, strings.TrimSpace(out))
	}
	var api struct {
		Stars       int      `json:"stargazers_count"`
		Forks       int      `json:"forks_count"`
		Description string   `json:"description"`
		Topics      []string `json:"topics"`
		License     *struct {
			SPDX string `json:"spdx_id"`
		} `json:"license"`
		PushedAt      string `json:"pushed_at"`
		Archived      bool   `json:"archived"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal([]byte(out), &api); err != nil {
		return Stats{}, fmt.Errorf("%s: %v", ownerRepo, err)
	}
	s := Stats{Repo: ownerRepo, Stars: api.Stars, Forks: api.Forks, Description: api.Description, Topics: api.Topics,
		PushedAt: api.PushedAt, Archived: api.Archived, DefaultBranch: api.DefaultBranch,
		FetchedAt: now.UTC().Format(time.RFC3339)}
	if api.License != nil {
		s.License = api.License.SPDX
	}
	return s, nil
}

// Read loads github.jsonl, keyed by repo; a missing file is empty.
func Read(path string) (map[string]Stats, error) {
	out := map[string]Stats{}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		var s Stats
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		out[s.Repo] = s
	}
	return out, sc.Err()
}

// Write writes stats sorted by repo, one per line, without HTML escaping.
func Write(path string, stats map[string]Stats) error {
	repos := make([]string, 0, len(stats))
	for r := range stats {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, r := range repos {
		s := stats[r]
		if s.Topics == nil {
			s.Topics = []string{}
		}
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path+".tmp", b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

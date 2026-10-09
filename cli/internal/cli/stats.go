package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/github"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

var ownerRepoRE = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

func statsFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	return fs, runStats
}

// runStats fetches GitHub stats for every repo in catalog/repos.jsonl into
// catalog/github.jsonl. A repo that fails keeps its previous record.
func runStats(sys System) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	if _, err := sys.LookPath("gh"); err != nil {
		return fmt.Errorf("ipm stats needs the GitHub CLI: install gh and run `gh auth login`")
	}
	if out, err := sys.Output(cfg.Root, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not logged in — run `gh auth login`: %s", strings.TrimSpace(out))
	}
	dir := filepath.Join(cfg.Root, "catalog")
	repos, err := catalog.Load(dir, catalog.TypeRepo)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "github.jsonl")
	stats, err := github.Read(path)
	if err != nil {
		return err
	}

	var targets []string
	skipped := 0
	for _, r := range repos {
		repo, _ := r["repo"].(string)
		if gh := pack.NormalizeRepo(repo); ownerRepoRE.MatchString(gh) {
			targets = append(targets, gh)
		} else {
			skipped++
			fmt.Fprintf(sys.Stdout, "- %s  not on GitHub, skipped\n", repo)
		}
	}
	gh := func(args ...string) (string, error) { return sys.Output(cfg.Root, "gh", args...) }
	results := make([]github.Stats, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, t := range targets {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errs[i] = github.Fetch(gh, t, sys.Now())
		})
	}
	wg.Wait()

	failed := 0
	for i, t := range targets {
		if errs[i] != nil {
			failed++
			fmt.Fprintf(sys.Stdout, "%s %v\n", bad, errs[i])
			continue
		}
		stats[t] = results[i]
	}
	if err := github.Write(path, stats); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "\n%d fetched, %d skipped, %d failed — %s\n", len(targets)-failed, skipped, failed, path)
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

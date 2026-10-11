package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/github"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"

	"github.com/spf13/cobra"
)

var ownerRepoRE = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

func newStatsCmd(sys System) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "fetch GitHub stats for every repo in the catalog into catalog/github.jsonl",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runStats(sys) },
	}
}

// runStats fetches GitHub stats for every repo in catalog/repos.jsonl into
// catalog/github.jsonl. A repo that fails keeps its previous record.
func runStats(sys System) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	if _, err := sys.LookPath("gh"); err != nil {
		return fmt.Errorf("ipm stats needs the GitHub CLI: install gh and run `gh auth login`")
	}
	if out, err := sys.Output("", "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not logged in — run `gh auth login`: %s", strings.TrimSpace(out))
	}
	dir := p.Catalog()
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
	gh := func(args ...string) (string, error) { return sys.Output("", "gh", args...) }
	results := make([]github.Stats, len(targets))
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var done atomic.Int32
	for i, t := range targets {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errs[i] = github.Fetch(gh, t, sys.Now())
			sys.progress(int(done.Add(1)), len(targets), t)
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

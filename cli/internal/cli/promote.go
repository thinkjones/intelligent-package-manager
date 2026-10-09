package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

// gitURL is a full git URL, as a [[repo]] entry accepts.
var gitURL = regexp.MustCompile(`^(https://|ssh://|file://|git@)\S+$`)

func promoteFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	return fs, func(sys System) error { return runPromote(sys, fs.Args()) }
}

// runPromote adds repos to the root manifest.toml. An external plugin's ID
// promotes the repo it lives in.
func runPromote(sys System, args []string) error {
	if len(args) == 0 {
		return usagef("promote: name at least one repo (owner/repo or URL) or external plugin ID. Try `ipm help promote`.")
	}
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	var plugins []map[string]any
	repos := make([]string, 0, len(args))
	for _, a := range args {
		if !strings.HasPrefix(a, catalog.TypePlugin+":") {
			repo := pack.NormalizeRepo(a)
			if !ownerRepoRE.MatchString(repo) && !gitURL.MatchString(repo) {
				return usagef("promote: %q is not owner/repo, a git URL or an external plugin ID", a)
			}
			repos = append(repos, repo)
			continue
		}
		if plugins == nil {
			if plugins, err = catalog.Load(filepath.Join(cfg.Root, "catalog"), catalog.TypePlugin); err != nil {
				return err
			}
		}
		repo, err := sourceRepo(plugins, a)
		if err != nil {
			return err
		}
		repos = append(repos, repo)
	}
	if err := pack.AppendInterest(cfg.Root, repos); err != nil {
		return err
	}
	for _, r := range repos {
		fmt.Fprintf(sys.Stdout, "%s %s added to %s\n", ok, r, pack.ManifestFile)
	}
	fmt.Fprintln(sys.Stdout, "\nNext: ipm sync && ipm index")
	return nil
}

// sourceRepo is the repo an external plugin record lives in.
func sourceRepo(plugins []map[string]any, id string) (string, error) {
	for _, p := range plugins {
		if p["id"] != id {
			continue
		}
		if repo, _ := p["source_repo"].(string); repo != "" {
			return repo, nil
		}
		return "", fmt.Errorf("%s is not an external plugin — its repo is already a repo of interest", id)
	}
	return "", fmt.Errorf("no plugin %s in catalog/plugins.jsonl — run `ipm index`", id)
}

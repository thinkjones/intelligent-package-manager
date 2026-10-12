// sourcepacks updates source-packs.toml, the curated list ipm onboard
// offers. It reads a wider list of repos, scans the ones it hasn't seen for
// anything ipm can catalog, adds those that hold something, and prints a
// Markdown report of what it added. It is a maintainer's tool, run with
// "mise run packs:update" or by the Source packs workflow, and is not part
// of the installed ipm. See internal/sourcepacks.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sourcepacks"
)

// allFailedIsOutage is how many repos must be scanned, every one failing,
// before the run is taken for an outage and nothing is written.
const allFailedIsOutage = 10

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "sourcepacks: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("sourcepacks", flag.ContinueOnError)
	from := fs.String("from", sourcepacks.StarredURL, "the list to look for new repos in: a URL or a file")
	packsFile := fs.String("packs", "source-packs.toml", "the source packs file to update")
	skippedFile := fs.String("skipped", "source-packs.skipped.txt", "the file of repos found to hold nothing")
	report := fs.String("report", "", "also write the Markdown report to this file")
	recheck := fs.Bool("recheck", false, "scan the repos in the skipped file again")
	workers := fs.Int("workers", 6, "how many repos to fetch at once")
	timeout := fs.Duration("timeout", 10*time.Minute, "how long one repo may take to download")
	base := fs.String("github", sourcepacks.GitHub, "where repos are fetched from")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	tmp, err := os.MkdirTemp("", "sourcepacks-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	data, _, err := curated.Fetch(&http.Client{Timeout: 30 * time.Second}, *from, tmp)
	if err != nil {
		return err
	}
	starred, _, err := curated.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %v", *from, err)
	}
	if len(starred) == 0 {
		return fmt.Errorf("%s lists no repos", *from)
	}
	var packs []curated.List
	switch data, err := os.ReadFile(*packsFile); {
	case err == nil:
		if packs, _, err = curated.Parse(data); err != nil {
			return fmt.Errorf("%s: %v", *packsFile, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	var skipped []string
	switch data, err := os.ReadFile(*skippedFile); {
	case err == nil:
		skipped = sourcepacks.ParseSkipped(data)
	case !errors.Is(err, os.ErrNotExist):
		return err
	}

	fetcher := sourcepacks.Fetcher{Client: &http.Client{}, Base: *base, Tmp: tmp, Timeout: *timeout}
	scan := func(repo string) (map[string]int, error) {
		counts, err := fetcher.Scan(repo)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", repo, err)
		case len(counts) == 0:
			fmt.Fprintf(os.Stderr, "- %s: nothing to catalog\n", repo)
		case !sourcepacks.Qualifies(counts):
			fmt.Fprintf(os.Stderr, "- %s: only %s\n", repo, sourcepacks.Holds(counts))
		default:
			fmt.Fprintf(os.Stderr, "✓ %s: %s\n", repo, sourcepacks.Holds(counts))
		}
		return counts, err
	}
	res := sourcepacks.Update(starred, packs, skipped, *recheck, scan, *workers)
	// Many repos all failing is the network or GitHub, not the repos. A few
	// can fail on every run (deleted, or too big to fetch in time) and must
	// not stop the rest being written.
	if res.Scanned > allFailedIsOutage && len(res.Failed) == res.Scanned {
		return fmt.Errorf("none of the %d repos could be fetched; nothing was written — first error: %s: %v", res.Scanned, res.Failed[0].Repo, res.Failed[0].Err)
	}
	if err := os.WriteFile(*packsFile, curated.Encode(sourcepacks.Header, res.Packs), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*skippedFile, sourcepacks.EncodeSkipped(res.Skipped), 0o644); err != nil {
		return err
	}
	md := sourcepacks.Report(res)
	if *report != "" {
		if err := os.WriteFile(*report, []byte(md), 0o644); err != nil {
			return err
		}
	}
	fmt.Print(md)
	return nil
}

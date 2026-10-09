package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/copilot"
	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

type createPluginOpts struct {
	packs                         multi
	name, description, owner, url string
	force, dryRun, noValidate     bool
}

func createPluginFlags() (*flag.FlagSet, func(System) error) {
	var o createPluginOpts
	fs := flag.NewFlagSet("create-plugin", flag.ContinueOnError)
	fs.Var(&o.packs, "pack", "pack `NAME` to publish; repeatable (default: every pack)")
	fs.StringVar(&o.name, "name", "", "marketplace name (default: <checkout-dir>-marketplace)")
	fs.StringVar(&o.description, "description", "", "marketplace description")
	fs.StringVar(&o.owner, "owner", "", "maintainer name (default: git user.name)")
	fs.StringVar(&o.url, "repo-url", "", "this repo's `URL`, recorded as the owner URL (default: the GitHub origin remote, if any)")
	fs.BoolVar(&o.force, "force", false, "overwrite a marketplace.json ipm didn't write")
	fs.BoolVar(&o.dryRun, "dry-run", false, "report what would be written, write nothing")
	fs.BoolVar(&o.noValidate, "no-validate", false, "skip `claude plugin validate`")
	return fs, func(sys System) error { return createPlugin(sys, o) }
}

func createPlugin(sys System, o createPluginOpts) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	root := cfg.Root

	packs, err := pack.LoadAll(root, o.packs)
	if err != nil {
		return err
	}
	var publishing []pack.Pack
	for _, p := range packs {
		if !p.Empty() {
			publishing = append(publishing, p)
		} else if len(o.packs) > 0 {
			fmt.Fprintf(sys.Stderr, "warning: pack %q publishes nothing: no [[plugin]], no skills/, no includes\n", p.Name)
		}
	}
	if len(publishing) == 0 {
		return fmt.Errorf("no pack under %s declares a plugin or includes a pack — nothing to publish", root)
	}
	// Conflicts among declared plugins read best before anything else; the
	// check runs again once Copilot items add theirs.
	if problems := pack.Problems(publishing); len(problems) > 0 {
		return fmt.Errorf("cannot publish:\n  %s", strings.Join(problems, "\n  "))
	}
	lock, err := upstream.ReadLock(root)
	if err != nil {
		return err
	}
	if missing := lock.Missing(pack.Upstreams(publishing)); len(missing) > 0 {
		keys := make([]string, len(missing))
		for i, u := range missing {
			keys[i] = u.Key()
		}
		return fmt.Errorf("%s does not pin %s — run `ipm sync`", upstream.LockFile, strings.Join(keys, ", "))
	}
	expanded := map[pack.CopilotItem][]pack.Plugin{} // an item listed by several packs is expanded once
	for i, p := range publishing {
		for _, item := range p.Copilot() {
			plugins, done := expanded[item]
			if !done {
				res, err := copilot.Expand(root, item, lock)
				if err != nil {
					return fmt.Errorf("%s: %v", p.Name, err)
				}
				if len(res.Skipped) > 0 {
					fmt.Fprintf(sys.Stderr, "%s: %s %s/%s: skipped %s — only Copilot skills are published\n", p.Name, item.Kind, item.Repo, item.Path, strings.Join(res.Skipped, ", "))
				}
				plugins = res.Plugins
				expanded[item] = plugins
			}
			publishing[i].Plugins = append(publishing[i].Plugins, plugins...)
		}
	}
	if problems := pack.Problems(publishing); len(problems) > 0 {
		return fmt.Errorf("cannot publish:\n  %s", strings.Join(problems, "\n  "))
	}

	opts := marketplace.Options{
		Name:        o.name,
		Description: o.description,
		Owner:       o.owner,
		RepoURL:     strings.TrimRight(o.url, "/"),
	}
	if opts.Name == "" {
		opts.Name = filepath.Base(root) + "-marketplace"
	}
	if !pack.ValidName(opts.Name) {
		return fmt.Errorf("marketplace name %q may use only letters, digits, '.', '_' and '-'", opts.Name)
	}
	if opts.Owner == "" {
		opts.Owner = gitOutput(sys, root, "config", "user.name")
	}
	if opts.Owner == "" {
		opts.Owner = filepath.Base(root)
	}
	if opts.RepoURL == "" {
		opts.RepoURL = githubURL(gitOutput(sys, root, "remote", "get-url", "origin"))
	}

	names := make([]string, len(publishing))
	for i, p := range publishing {
		names[i] = p.Name
	}
	if opts.Description == "" {
		opts.Description = fmt.Sprintf("Plugins from %s (%s)", filepath.Base(root), strings.Join(names, ", "))
	}
	entries := marketplace.Entries(publishing, lock)
	m := marketplace.Build(entries, names, opts)

	path := marketplace.Path(root)
	if _, err := os.Stat(path); err == nil && !o.force && !marketplace.IsOurs(path) {
		return fmt.Errorf("%s exists and was not written by ipm — pass --force to overwrite", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	verb := "would write"
	if !o.dryRun {
		if err := marketplace.Write(path, m); err != nil {
			return err
		}
		verb = "wrote"
	}

	fmt.Fprintf(sys.Stdout, "%s %s\n", verb, path)
	fmt.Fprintf(sys.Stdout, "marketplace %q — %d plugins, all referenced upstream at locked commits, nothing copied\n", opts.Name, len(entries))
	width := 0
	for _, e := range entries {
		width = max(width, len(e.Name))
	}
	last := ""
	for _, e := range entries {
		if e.Pack != last {
			fmt.Fprintf(sys.Stdout, "  %s\n", e.Pack)
			last = e.Pack
		}
		fmt.Fprintf(sys.Stdout, "    %-*s  %s\n", width, e.Name, marketplace.Describe(e.Source))
	}

	if o.dryRun {
		return nil
	}
	ok := o.noValidate || validate(sys, root)
	fmt.Fprintf(sys.Stdout, "\nTry it from this checkout:\n  claude plugin marketplace add %s\n  claude plugin install <plugin>@%s\n", root, opts.Name)
	if opts.RepoURL != "" {
		fmt.Fprintf(sys.Stdout, "\nOnce committed and pushed:\n  claude plugin marketplace add %s\n", opts.RepoURL)
	}
	if !ok {
		return &ExitError{Code: 1}
	}
	return nil
}

// validate runs `claude plugin validate` when claude is on PATH.
func validate(sys System, root string) bool {
	if _, err := sys.LookPath("claude"); err != nil {
		fmt.Fprintln(sys.Stdout, "claude not on PATH — skipped `claude plugin validate`")
		return true
	}
	out, err := sys.Output(root, "claude", "plugin", "validate", root)
	if err != nil {
		fmt.Fprintf(sys.Stderr, "error: claude plugin validate failed:\n%s\n", strings.TrimSpace(out))
		return false
	}
	fmt.Fprintln(sys.Stdout, "claude plugin validate: ok")
	return true
}

// gitOutput is `git -C root args…`, trimmed; "" on any failure.
func gitOutput(sys System, root string, args ...string) string {
	out, err := sys.Output(root, "git", append([]string{"-C", root}, args...)...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var githubRemote = regexp.MustCompile(`^(?:git@github\.com:|https://github\.com/)([^/]+)/(.+?)(?:\.git)?/?$`)

// githubURL turns a GitHub remote into https://github.com/<owner>/<repo>;
// "" for anything else.
func githubURL(remote string) string {
	m := githubRemote.FindStringSubmatch(remote)
	if m == nil {
		return ""
	}
	return "https://github.com/" + m[1] + "/" + m[2]
}

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/assemble"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/marketplace"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

func newPackBuildCmd(sys System, global *bool) *cobra.Command {
	var update, noValidate bool
	cmd := &cobra.Command{
		Use:   "build",
		Short: "build each pack in ipm.toml into a Claude Code plugin under .ipm/ (-g for your user scope)",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runBuild(sys, *global, update, noValidate) },
	}
	cmd.Flags().BoolVar(&update, "update", false, "re-pin every item to the commit in your current catalog")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "skip claude plugin validate --strict")
	return cmd
}

// buildStamp is a hex SHA-256 over the packs, the lock and ipm's version: what
// a build is made from.
func buildStamp(scope project.Scope, version string) (string, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return "", err
	}
	lock, err := os.ReadFile(filepath.Join(scope.IPMDir(), project.LockFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(project.PacksDigest(packs)))
	h.Write([]byte{0})
	h.Write(lock)
	h.Write([]byte{0})
	h.Write([]byte(version))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// buildCurrent reports whether scope's .ipm holds a build made from its
// current packs, lock and ipm version.
func buildCurrent(sys System, scope project.Scope) bool {
	dir := scope.IPMDir()
	for _, rel := range []string{"plugins", filepath.Join(".claude-plugin", "marketplace.json")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			return false
		}
	}
	have, err := os.ReadFile(filepath.Join(dir, project.StampFile))
	if err != nil {
		return false
	}
	want, err := buildStamp(scope, sys.Version)
	return err == nil && strings.TrimSpace(string(have)) == want
}

// pluginJSON is a built pack's manifest, fields in a fixed order.
type pluginJSON struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Dependencies []string `json:"dependencies,omitempty"`
	Metadata     struct {
		IPM struct {
			Items []map[string]string `json:"items"`
		} `json:"ipm"`
	} `json:"metadata"`
}

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// runBuild builds every pack in the scope's ipm.toml into a plugin under
// .ipm/plugins and writes .ipm's marketplace. Nothing is written until every
// item resolves.
func runBuild(sys System, global, update, noValidate bool) error {
	return build(sys, global, update, noValidate, true)
}

// build is ipm pack build; hints adds the commands to install the result in
// Claude Code, which install claude runs itself instead.
func build(sys System, global, update, noValidate, hints bool) error {
	scope, err := currentScope(sys, global)
	if err != nil {
		return err
	}
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	if len(packs) == 0 {
		return fmt.Errorf("no packs in %s — create one with `ipm pack new <name>`", filepath.Join(scope.Root, project.File))
	}
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	dir := scope.IPMDir()
	lock := map[string]project.LockItem{}
	if !update {
		if lock, err = project.LoadLock(dir); err != nil {
			return err
		}
	}
	items, err := assemble.Resolve(packs, lock, p.Catalog())
	if err != nil {
		return err
	}
	plan, err := assemble.Build(packs, items)
	if err != nil {
		return err
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(sys.Stdout, "%s %s\n", warn, w)
	}

	root, err := sources.WriteProject(dir, plan.Repos)
	if err != nil {
		return err
	}
	hooks, err := writePnpmfile(dir, root)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\nplugins/\nplugins.tmp/\n"), 0o644); err != nil {
		return err
	}
	if len(plan.Repos) > 0 {
		pn, err := findPnpm(sys)
		if err != nil {
			return err
		}
		args := []string{"install", "--no-frozen-lockfile", "--ignore-scripts", "--config.ignore-pnpmfile=false", "--config.pnpmfile=" + hooks,
			"--store-dir", p.Store(), "--dir", dir}
		fmt.Fprintf(sys.Stdout, "%s %s\n", pn, strings.Join(args, " "))
		if out, err := sys.Output(dir, pn.name, append(append([]string{}, pn.prefix...), args...)...); err != nil {
			fmt.Fprintln(sys.Stdout, strings.TrimSpace(out))
			fmt.Fprintf(sys.Stdout, "%s pnpm failed\n", bad)
			return &ExitError{Code: 1}
		}
	}

	owner := "ipm"
	if out, err := sys.Output(scope.Root, "git", "config", "user.name"); err == nil && strings.TrimSpace(out) != "" {
		owner = strings.TrimSpace(out)
	}
	// Build into plugins.tmp and swap it in, so a failure keeps the last
	// good build; the lock is written only once the build is in place.
	plugins, staging := filepath.Join(dir, "plugins"), filepath.Join(dir, "plugins.tmp")
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	var entries []any
	var names []string
	for _, pp := range plan.Packs {
		pluginDir := filepath.Join(staging, pp.Name)
		if err := assemble.Place(pp, filepath.Join(dir, "node_modules"), pluginDir, os.Link); err != nil {
			return err
		}
		var pj pluginJSON
		pj.Name, pj.Description, pj.Version, pj.Dependencies = pp.Name, pp.Description, "0.0.0-ipm", pp.DepNames()
		pj.Author.Name = owner
		pj.Metadata.IPM.Items = pp.Items()
		if pj.Metadata.IPM.Items == nil {
			pj.Metadata.IPM.Items = []map[string]string{}
		}
		if err := writeJSON(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), pj); err != nil {
			return err
		}
		entries = append(entries, marketplace.Entry{Name: pp.Name, Description: pp.Description, Source: "./plugins/" + pp.Name,
			Tags: []string{marketplace.OwnerTag, pp.Name}})
		entries = append(entries, pp.Deps...)
		names = append(names, pp.Name)
		fmt.Fprintf(sys.Stdout, "%s %s: %d item(s), %d plugin dependency(ies)\n", ok, pp.Name, len(pp.Files), len(pp.Deps))
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(plugins); err != nil {
		return err
	}
	if err := os.Rename(staging, plugins); err != nil {
		return err
	}
	name := marketName(scope)
	m := marketplace.Build(entries, names, marketplace.Options{Name: name, Owner: owner, Description: "ipm packs: " + strings.Join(names, ", ")})
	if err := marketplace.Write(filepath.Join(dir, ".claude-plugin", "marketplace.json"), m); err != nil {
		return err
	}
	if err := project.SaveLock(dir, items); err != nil {
		return err
	}

	if noValidate {
		fmt.Fprintln(sys.Stdout, "validate: skipped (--no-validate)")
	} else if _, err := sys.LookPath("claude"); err != nil {
		fmt.Fprintln(sys.Stdout, "validate: skipped (claude not on PATH)")
	} else if out, err := sys.Output(scope.Root, "claude", "plugin", "validate", "--strict", dir); err != nil {
		fmt.Fprintln(sys.Stdout, strings.TrimSpace(out))
		fmt.Fprintf(sys.Stdout, "%s claude plugin validate --strict failed\n", bad)
		return &ExitError{Code: 1}
	} else {
		fmt.Fprintln(sys.Stdout, "validate: ok")
	}
	stamp, err := buildStamp(scope, sys.Version)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, project.StampFile), []byte(stamp+"\n"), 0o644); err != nil {
		return err
	}
	if !hints {
		return nil
	}
	fmt.Fprintf(sys.Stdout, "\nInstall in Claude Code: ipm install claude\nOr wire it yourself:\n  claude plugin marketplace add %s\n", dir)
	return nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// marketName is the name of the marketplace ipm pack build builds for scope:
// <project folder>-ipm, or ipm-global for the user scope.
func marketName(scope project.Scope) string {
	if scope.Global {
		return "ipm-global"
	}
	return strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(filepath.Base(scope.Root)), "-"), "-.") + "-ipm"
}

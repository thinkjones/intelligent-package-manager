package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/assemble"
	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/project"
	"github.com/thinkjones/fs-skills/cli/internal/sources"
)

func installFlags() (*flag.FlagSet, func(System) error) {
	var global, update, noValidate bool
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.BoolVar(&global, "g", false, "build your user scope's packs ($IPM_HOME/global)")
	fs.BoolVar(&update, "update", false, "re-pin every item to the commit in your current catalog")
	fs.BoolVar(&noValidate, "no-validate", false, "skip claude plugin validate --strict")
	return fs, func(sys System) error { return runInstall(sys, global, update, noValidate) }
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

// runInstall builds every pack in the scope's ipm.toml into a plugin under
// .ipm/plugins and writes .ipm's marketplace. Nothing is written until every
// item resolves.
func runInstall(sys System, global, update, noValidate bool) error {
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

	if err := project.SaveLock(dir, items); err != nil {
		return err
	}
	root, err := sources.WriteProject(dir, plan.Repos)
	if err != nil {
		return err
	}
	hooks, err := writePnpmfile(dir, root)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\nplugins/\n"), 0o644); err != nil {
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
	plugins := filepath.Join(dir, "plugins")
	if err := os.RemoveAll(plugins); err != nil {
		return err
	}
	var entries []any
	var names []string
	for _, pp := range plan.Packs {
		pluginDir := filepath.Join(plugins, pp.Name)
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
	name := "ipm-global"
	if !scope.Global {
		name = strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(filepath.Base(scope.Root)), "-"), "-.") + "-ipm"
	}
	m := marketplace.Build(entries, names, marketplace.Options{Name: name, Owner: owner, Description: "ipm packs: " + strings.Join(names, ", ")})
	if err := marketplace.Write(filepath.Join(dir, ".claude-plugin", "marketplace.json"), m); err != nil {
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
	fmt.Fprintf(sys.Stdout, "\nInstall in Claude Code:\n  claude plugin marketplace add %s\n", dir)
	for _, n := range names {
		fmt.Fprintf(sys.Stdout, "  /plugin install %s@%s\n", n, name)
	}
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

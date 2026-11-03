package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/assemble"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// museManifests are the plugin manifests Muse Code reads, any one of which
// makes a folder a plugin it can install.
var museManifests = []string{".muse-plugin", ".claude-plugin", ".codex-plugin"}

func newInstallMuseCmd(sys System) *cobra.Command {
	var o installOpts
	cmd := &cobra.Command{
		Use:   "muse [pack]",
		Short: "build a pack if needed and install it, and its whole plugins, into Muse Code",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("install muse: give at most one pack. Try `ipm help install muse`.")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			pack := ""
			if len(args) == 1 {
				pack = args[0]
			}
			if o.DryRun && o.Update {
				return usagef("install muse: --dry-run and --update can't be used together")
			}
			scope, err := currentScope(sys, o.Global)
			if err != nil {
				return err
			}
			done, err := installMuse(sys, scope, pack, project.Install{}, o, sys.Stdout)
			if err != nil || o.DryRun {
				return err
			}
			fmt.Fprintf(sys.Stdout, "%s installed %s\n", ok, done)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&o.Global, "global", "g", false, "install a pack from your user scope ($IPM_HOME/global)")
	f.BoolVar(&o.Update, "update", false, "re-pin every item to your current catalog and rebuild first")
	f.BoolVar(&o.DryRun, "dry-run", false, "print the muse commands without building, running or recording anything")
	f.BoolVar(&o.Force, "force", false, "replace a plugin Muse Code holds from a folder ipm didn't install it from")
	return cmd
}

// installMuse installs pack into Muse Code, writing everything it does to
// out: it builds the scope's packs unless the build is current (or always
// with o.Update), installs the pack's plugin, then each whole plugin from its
// repo at the locked commit (Muse reads no plugin dependencies), and records
// the install. Muse keeps one set of plugins per user, so there is no scope.
// A pack Muse refuses stops the install, recording nothing; a whole plugin it
// refuses doesn't stop the others, and the install is recorded and ends with
// an error naming the plugins that failed. With o.DryRun it only prints the
// muse commands, from a current build. It returns what it installed where,
// as "kit (muse)".
func installMuse(sys System, scope project.Scope, pack string, _ project.Install, o installOpts, out io.Writer) (string, error) {
	if err := needMuse(sys); err != nil {
		return "", err
	}
	pack, err := pickPack(scope, "muse", pack)
	if err != nil {
		return "", err
	}
	if err := gascityBuild(sys, scope, o, out); err != nil {
		return "", err
	}
	plugins, err := musePluginDirs(sys, scope, pack, o.DryRun, out)
	if err != nil {
		return "", err
	}
	if plugins, err = museCopies(scope, pack, plugins, o.DryRun, out); err != nil {
		return "", err
	}
	desc := pack + " (muse)"
	var failed []string
	for i, p := range plugins {
		if o.DryRun {
			fmt.Fprintf(out, "$ muse plugins install %s\n", p.dir)
			continue
		}
		err := museInstall(sys, scope, p.dir, o.Force, out)
		if err == nil {
			continue
		}
		if i == 0 {
			return "", err
		}
		fmt.Fprintf(out, "%s %s: %v\n", bad, p.id, err)
		failed = append(failed, p.id)
	}
	if o.DryRun {
		return desc, nil
	}
	if err := recordInstall(sys, scope, pack, project.Install{Target: "muse"}); err != nil {
		return "", err
	}
	if len(failed) > 0 {
		return "", fmt.Errorf("installed %s, but %d of %d whole plugins failed: %s — muse's output above says why", desc, len(failed), len(plugins)-1, strings.Join(failed, ", "))
	}
	return desc, nil
}

var museElsewhere = regexp.MustCompile("plugin `([^`]+)` is already installed from a different source")

// museInstall runs muse plugins install dir, printing each command and what
// muse prints. Muse refuses a plugin it holds from another folder; ipm then
// removes it and installs again, when ipm installed it (from a .ipm/ folder or
// ipm's home) or with force.
func museInstall(sys System, scope project.Scope, dir string, force bool, out io.Writer) error {
	run := func(args ...string) (string, error) {
		line := "muse " + strings.Join(args, " ")
		fmt.Fprintf(out, "$ %s\n", line)
		res, err := sys.Output(scope.Root, "muse", args...)
		museOutput(out, res, dir)
		if err != nil {
			return res, fmt.Errorf("`%s` failed: %v", line, err)
		}
		return res, nil
	}
	res, err := run("plugins", "install", dir)
	g := museElsewhere.FindStringSubmatch(res)
	if err == nil || g == nil {
		return err
	}
	name, from := g[1], museSource(sys, scope, g[1])
	if !force && !ipmFolder(sys, from) {
		return fmt.Errorf("%s is installed in Muse Code from %s, which ipm didn't install — use --force to replace it", name, cmp.Or(from, "another folder"))
	}
	fmt.Fprintf(out, "replacing %s in Muse Code: it was installed from %s\n", name, cmp.Or(from, "another folder"))
	if _, err := run("plugins", "remove", name); err != nil {
		return err
	}
	_, err = run("plugins", "install", dir)
	return err
}

// museSource is the folder Muse installed plugin name from, or empty.
func museSource(sys System, scope project.Scope, name string) string {
	res, err := sys.Output(scope.Root, "muse", "plugins", "inspect", name, "--json")
	if err != nil {
		return ""
	}
	var v struct {
		Record struct {
			Source struct{ Path string } `json:"source"`
		} `json:"record"`
	}
	_ = json.Unmarshal([]byte(res), &v)
	return v.Record.Source.Path
}

// ipmFolder reports whether ipm installs plugins from p: a folder under a
// .ipm/ folder (a build or its Muse copies) or ipm's home (synced sources and
// the pin store).
func ipmFolder(sys System, p string) bool {
	if p == "" {
		return false
	}
	if strings.Contains(filepath.ToSlash(p)+"/", "/.ipm/") {
		return true
	}
	paths, err := ipmPaths(sys)
	return err == nil && inside(realPath(p), realPath(paths.Home))
}

// museOutput writes what muse printed for dir to out, without its warnings
// (one line each, usually about Claude fields it ignores), which it counts.
func museOutput(out io.Writer, res, dir string) {
	hidden := 0
	for line := range strings.Lines(strings.TrimSpace(res)) {
		switch {
		case strings.Contains(line, " severity=warning "):
			hidden++
		case strings.TrimSpace(line) != "":
			fmt.Fprintln(out, strings.TrimRight(line, "\n"))
		}
	}
	if hidden > 0 {
		fmt.Fprintf(out, "%d muse warning(s) hidden — `muse plugins validate %s` shows them\n", hidden, dir)
	}
}

// museCopies is plugins as Muse copies under .ipm/muse/<pack>/, fixed where
// Muse Code differs from Claude Code (see assemble.FixForMuse), printing each
// change. The pack's copy is named after it, each whole plugin's after the
// plugin. With dryRun nothing is written; a plugin not yet fetched is
// installed as it is.
func museCopies(scope project.Scope, pack string, plugins []musePlugin, dryRun bool, out io.Writer) ([]musePlugin, error) {
	root := filepath.Join(scope.IPMDir(), "muse", pack)
	if !dryRun {
		if err := os.RemoveAll(root); err != nil {
			return nil, err
		}
		if err := ignoreIn(scope.IPMDir(), "muse/"); err != nil {
			return nil, err
		}
	}
	used := map[string]bool{}
	copies := make([]musePlugin, 0, len(plugins))
	for i, p := range plugins {
		if _, err := os.Stat(p.dir); err != nil && dryRun {
			copies = append(copies, p)
			continue
		}
		fix, err := assemble.FixForMuse(p.dir, i == 0)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p.id, err)
		}
		for _, f := range fix.Fixed {
			fmt.Fprintf(out, "fixed %s for Muse Code: %s\n", p.id, f)
		}
		for _, d := range fix.Dropped {
			fmt.Fprintf(out, "%s %s: %s\n", warn, p.id, d)
		}
		name := p.name
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", p.name, n)
		}
		used[name] = true
		dst := filepath.Join(root, name)
		if !dryRun {
			if err := assemble.WriteMuseCopy(p.dir, dst, fix); err != nil {
				return nil, fmt.Errorf("%s: %v", p.id, err)
			}
		}
		copies = append(copies, musePlugin{p.id, p.name, dst})
	}
	return copies, nil
}

// musePlugin is one folder ipm install muse installs: the pack's plugin, or
// a whole plugin, by its item ID.
type musePlugin struct{ id, name, dir string }

// musePluginDirs is the folders to install for pack from scope's build: the
// pack's plugin (by the pack's name), then each whole plugin's folder, fetched into the pin store
// when the sources aren't at its commit (with dryRun, nothing is fetched and
// a plugin not yet on disk is named by where it will be). Plugins ipm can't
// fetch, or whose folder holds no manifest, are skipped with a warning.
func musePluginDirs(sys System, scope project.Scope, pack string, dryRun bool, out io.Writer) ([]musePlugin, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return nil, err
	}
	lock, err := project.LoadLock(scope.IPMDir())
	if err != nil {
		return nil, err
	}
	plan, err := assemble.PlanMuse(packs[findPack(packs, pack)], lock)
	if err != nil {
		return nil, err
	}
	for _, s := range plan.Skips {
		fmt.Fprintf(out, "%s %s\n", warn, s)
	}
	fetch := sys
	fetch.Stdout = out
	repos, err := findRepoFolders(fetch, plan.Repos(), !dryRun)
	if err != nil {
		return nil, err
	}
	dirs := []musePlugin{{pack, pack, filepath.Join(scope.IPMDir(), "plugins", pack)}}
	for _, p := range plan.Plugins {
		dir := filepath.Join(repos[p.Alias], filepath.FromSlash(p.From))
		if !dryRun && !hasMuseManifest(dir) {
			fmt.Fprintf(out, "%s skipped %s: its folder has no plugin manifest (its marketplace entry defines it), which Muse Code can't install\n", warn, p.Item.ID)
			continue
		}
		dirs = append(dirs, musePlugin{p.Item.ID, assemble.FileName(p.Item), dir})
	}
	return dirs, nil
}

// hasMuseManifest reports whether dir holds a plugin manifest Muse reads.
func hasMuseManifest(dir string) bool {
	for _, m := range museManifests {
		if _, err := os.Stat(filepath.Join(dir, m, "plugin.json")); err == nil {
			return true
		}
	}
	return false
}

func needMuse(sys System) error {
	if _, err := sys.LookPath("muse"); err != nil {
		return errors.New("ipm install muse needs muse (Muse Code) on PATH")
	}
	return nil
}

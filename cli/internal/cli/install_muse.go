package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	desc := pack + " (muse)"
	var failed []string
	for i, p := range plugins {
		args := []string{"plugins", "install", p.dir}
		line := "muse " + strings.Join(args, " ")
		fmt.Fprintf(out, "$ %s\n", line)
		if o.DryRun {
			continue
		}
		res, err := sys.Output(scope.Root, "muse", args...)
		if res = strings.TrimSpace(res); res != "" {
			fmt.Fprintln(out, res)
		}
		if err == nil {
			continue
		}
		if i == 0 {
			return "", fmt.Errorf("`%s` failed: %v", line, err)
		}
		fmt.Fprintf(out, "%s %s: `%s` failed: %v\n", bad, p.id, line, err)
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

// musePlugin is one folder ipm install muse installs: the pack's plugin, or
// a whole plugin, by its item ID.
type musePlugin struct{ id, dir string }

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
	dirs := []musePlugin{{pack, filepath.Join(scope.IPMDir(), "plugins", pack)}}
	for _, p := range plan.Plugins {
		dir := filepath.Join(repos[p.Alias], filepath.FromSlash(p.From))
		if !dryRun && !hasMuseManifest(dir) {
			fmt.Fprintf(out, "%s skipped %s: its folder has no plugin manifest (its marketplace entry defines it), which Muse Code can't install\n", warn, p.Item.ID)
			continue
		}
		dirs = append(dirs, musePlugin{p.Item.ID, dir})
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

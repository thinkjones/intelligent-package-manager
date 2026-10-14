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

// newInstallGascityCmd is ipm install gascity: in.Path is the folder the pack
// folder goes in, in.Name the pack folder's name.
func newInstallGascityCmd(sys System) *cobra.Command {
	var o installOpts
	var in project.Install
	cmd := &cobra.Command{
		Use:   "gascity [pack]",
		Short: "write a pack as a Gas City pack folder, building it first if needed",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("install gascity: give at most one pack. Try `ipm help install gascity`.")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			pack := ""
			if len(args) == 1 {
				pack = args[0]
			}
			if o.DryRun && o.Update {
				return usagef("install gascity: --dry-run and --update can't be used together")
			}
			scope, err := currentScope(sys, o.Global)
			if err != nil {
				return err
			}
			done, err := installGascity(sys, scope, pack, in, o, sys.Stdout)
			if err != nil || o.DryRun {
				return err
			}
			fmt.Fprintf(sys.Stdout, "%s installed %s\n", ok, done)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&in.Path, "dir", "C", "", "the folder to write the pack folder into (default: the working directory)")
	f.StringVar(&in.Name, "name", "", "the pack folder's name (default: the pack's name)")
	f.BoolVarP(&o.Global, "global", "g", false, "install a pack from your user scope ($IPM_HOME/global)")
	f.BoolVar(&o.Force, "force", false, "replace a folder that ipm did not write")
	f.BoolVar(&o.DryRun, "dry-run", false, "list the files that would be written, without writing or recording anything (needs a current build)")
	f.BoolVar(&o.Update, "update", false, "re-pin every item to your current catalog and rebuild first")
	return cmd
}

// installGascity writes pack from scope as the Gas City pack folder
// <in.Path>/<in.Name>, writing what it does to out, then records the install
// and prints how to import it. With o.DryRun it only lists the files, from a
// current build. It returns what it installed where, as "kit (gascity /d/kit)".
func installGascity(sys System, scope project.Scope, pack string, in project.Install, o installOpts, out io.Writer) (string, error) {
	pack, err := pickPack(scope, "gascity", pack)
	if err != nil {
		return "", err
	}
	name := in.Name
	if name == "" {
		name = pack
	}
	if !assemble.ValidGasName(name) {
		return "", usagef("install gascity: --name %q must start with a letter or digit and use only letters, digits, '_' and '-'", name)
	}
	dir, err := gascityDir(in.Path, workDir(sys, scope), sys.Getenv("HOME"))
	if err != nil {
		return "", err
	}
	folder := filepath.Join(dir, name)
	if err := gascityRefusal(sys, scope, folder); err != nil {
		return "", err
	}
	if err := assemble.CheckGascityTarget(dir, name, o.Force); err != nil {
		if o.DryRun {
			fmt.Fprintf(out, "would refuse: %s\n", err)
		}
		return "", err
	}
	if o.DryRun {
		if !buildCurrent(sys, scope) {
			return "", errors.New("--dry-run needs a current build — run ipm pack build first")
		}
	} else if err := ensureBuilt(sys, scope, o, out); err != nil {
		return "", err
	}
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return "", err
	}
	lock, err := project.LoadLock(scope.IPMDir())
	if err != nil {
		return "", err
	}
	plan, err := assemble.PlanGascity(packs[findPack(packs, pack)], name, lock)
	if err != nil {
		return "", err
	}
	files, warns, err := assemble.RenderGascity(plan, filepath.Join(scope.IPMDir(), "node_modules"))
	if err != nil {
		return "", err
	}
	for _, s := range append(append([]string{}, plan.Skips...), warns...) {
		fmt.Fprintf(out, "%s %s\n", warn, s)
	}
	desc := pack + " (gascity " + folder + ")"
	if o.DryRun {
		fmt.Fprintf(out, "would write %s\n", folder)
		for _, f := range files {
			fmt.Fprintf(out, "  %s\n", f.Path)
		}
		return desc, nil
	}
	if _, err := assemble.WriteGascity(files, dir, name, o.Force); err != nil {
		return "", err
	}
	if err := recordInstall(sys, scope, pack, project.Install{Target: "gascity", Path: dir, Name: name}); err != nil {
		return "", err
	}
	fmt.Fprintf(out, "Import it into a city's pack.toml or city.toml:\n  [imports.%s]\n  source = %q\n", name, folder)
	return desc, nil
}

// gascityDir makes the folder to write into absolute: empty is cwd, a leading
// ~ is home, a relative path is from cwd; it must be an existing folder.
func gascityDir(p, cwd, home string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		p = cwd
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if info, err := os.Stat(p); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", p)
	}
	return p, nil
}

// gascityRefusal refuses, even with --force, a pack folder that is or holds
// the scope's root or ipm's home: replacing it would delete them.
func gascityRefusal(sys System, scope project.Scope, folder string) error {
	paths, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	if !scope.Global && holds(folder, scope.Root) {
		return fmt.Errorf("refusing to replace %s: it holds the project", folder)
	}
	if holds(folder, paths.Home) {
		return fmt.Errorf("refusing to replace %s: it holds ipm's home", folder)
	}
	return nil
}

// holds reports whether folder is path or one of its ancestors, by real path:
// symlinks are resolved (through the nearest existing ancestor, since either
// may not exist yet), and where folder exists os.SameFile on each ancestor of
// path catches what spelling cannot, such as letter case on a case-insensitive
// filesystem.
func holds(folder, path string) bool {
	if inside(realPath(path), realPath(folder)) {
		return true
	}
	want, err := os.Stat(folder)
	if err != nil {
		return false
	}
	for dir := filepath.Clean(path); ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil && os.SameFile(want, info) {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

// realPath resolves symlinks in the nearest existing ancestor of p and joins
// the components that do not exist back on.
func realPath(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for dir := p; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, rest)
		}
		if filepath.Dir(dir) == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

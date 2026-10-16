package cli

import (
	"bytes"
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
// folder goes in (empty is the local project's .ipm/.gascity), in.Name the
// pack folder's name. Without a pack it installs every pack of the scope.
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
			if pack == "" && in.Name != "" {
				return usagef("install gascity: --name needs a pack — it names one pack folder")
			}
			scope, err := currentScope(sys, o.Global)
			if err != nil {
				return err
			}
			if pack == "" {
				return installAllGascity(sys, scope, in, o, sys.Stdout)
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
	f.StringVarP(&in.Path, "dir", "C", "", "the folder to write the pack folder into (default: ./.ipm/.gascity in the project)")
	f.StringVar(&in.Name, "name", "", "the pack folder's name (default: the pack's name)")
	f.BoolVarP(&o.Global, "global", "g", false, "install packs from your user scope ($IPM_HOME/global) into the project")
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
	t, err := gascityPlace(sys, scope, pack, in, o, out)
	if err != nil {
		return "", err
	}
	if err := gascityBuild(sys, scope, o, out); err != nil {
		return "", err
	}
	return writeGascityPack(sys, scope, pack, t, o, out)
}

// installAllGascity is ipm install gascity without a pack: every pack of
// scope, in ipm.toml's order, each into <in.Path>/<its name>, after one
// build. A failing pack doesn't stop the rest; the command then exits 1.
func installAllGascity(sys System, scope project.Scope, in project.Install, o installOpts, out io.Writer) error {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	if len(packs) == 0 {
		_, err := pickPack(scope, "gascity", "")
		return err
	}
	if err := gascityBuild(sys, scope, o, out); err != nil {
		return err
	}
	failed := 0
	for _, p := range packs {
		done, err := func() (string, error) {
			t, err := gascityPlace(sys, scope, p.Name, in, o, out)
			if err != nil {
				return "", err
			}
			return writeGascityPack(sys, scope, p.Name, t, o, out)
		}()
		if err != nil {
			failed++
			fmt.Fprintf(out, "%s %s: %v\n", bad, p.Name, err)
			continue
		}
		if !o.DryRun {
			fmt.Fprintf(out, "%s installed %s\n", ok, done)
		}
	}
	if failed > 0 {
		fmt.Fprintf(out, "%s %d of %d packs failed\n", bad, failed, len(packs))
		return &ExitError{Code: 1}
	}
	if len(packs) > 1 && !o.DryRun {
		fmt.Fprintf(out, "%s installed %d packs\n", ok, len(packs))
	}
	return nil
}

// gascityTarget is where a pack folder goes: <dir>/<name>, and whether dir is
// a default folder an install creates.
type gascityTarget struct {
	dir, name string
	create    bool
}

// gascityPlace is where pack's folder goes, refusing a folder that holds the
// project or ipm's home, or one ipm did not write unless o.Force.
func gascityPlace(sys System, scope project.Scope, pack string, in project.Install, o installOpts, out io.Writer) (gascityTarget, error) {
	name := in.Name
	if name == "" {
		name = pack
	}
	if !assemble.ValidGasName(name) {
		return gascityTarget{}, usagef("install gascity: --name %q must start with a letter or digit and use only letters, digits, '_' and '-'", name)
	}
	dir, create, err := gascityFolder(sys, scope, in.Path)
	if err != nil {
		return gascityTarget{}, err
	}
	if err := gascityRefusal(sys, scope, filepath.Join(dir, name)); err != nil {
		return gascityTarget{}, err
	}
	if err := assemble.CheckGascityTarget(dir, name, o.Force); err != nil {
		if o.DryRun {
			fmt.Fprintf(out, "would refuse: %s\n", err)
		}
		return gascityTarget{}, err
	}
	return gascityTarget{dir, name, create}, nil
}

// gascityBuild makes sure scope's build is current: with o.DryRun it only
// checks, otherwise it builds when stale or with o.Update.
func gascityBuild(sys System, scope project.Scope, o installOpts, out io.Writer) error {
	if !o.DryRun {
		return ensureBuilt(sys, scope, o, out)
	}
	if !buildCurrent(sys, scope) {
		return errors.New("--dry-run needs a current build — run ipm pack build first")
	}
	return nil
}

// writeGascityPack writes pack from scope's build to t, records the install
// and prints how to import it; with o.DryRun it only lists the files.
func writeGascityPack(sys System, scope project.Scope, pack string, t gascityTarget, o installOpts, out io.Writer) (string, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return "", err
	}
	lock, err := project.LoadLock(scope.IPMDir())
	if err != nil {
		return "", err
	}
	plan, err := assemble.PlanGascity(packs[findPack(packs, pack)], t.name, lock)
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
	folder := filepath.Join(t.dir, t.name)
	desc := pack + " (gascity " + folder + ")"
	if o.DryRun {
		fmt.Fprintf(out, "would write %s\n", folder)
		for _, f := range files {
			fmt.Fprintf(out, "  %s\n", f.Path)
		}
		return desc, nil
	}
	if t.create {
		if err := os.MkdirAll(t.dir, 0o755); err != nil {
			return "", err
		}
		if err := ignoreGascity(filepath.Dir(t.dir)); err != nil {
			return "", err
		}
	}
	if _, err := assemble.WriteGascity(files, t.dir, t.name, o.Force); err != nil {
		return "", err
	}
	if err := recordInstall(sys, scope, pack, project.Install{Target: "gascity", Path: t.dir, Name: t.name}); err != nil {
		return "", err
	}
	fmt.Fprintf(out, "Import it into a city's pack.toml or city.toml:\n  [imports.%s]\n  source = %q\n", t.name, folder)
	return desc, nil
}

// gascityDefault is the folder ipm install gascity writes into without -C:
// the local project's .ipm/.gascity, for user-scope packs too.
func gascityDefault(sys System, scope project.Scope) string {
	return filepath.Join(localRoot(sys, scope), ".ipm", ".gascity")
}

// localRoot is the project ipm install gascity writes into: scope's, or for
// the user scope the one the working directory is in, else the working
// directory.
func localRoot(sys System, scope project.Scope) string {
	if !scope.Global {
		return scope.Root
	}
	s, _, _ := project.Find(workDir(sys, scope), false, "")
	return s.Root
}

// isGascityDefault reports whether dir is some project's .ipm/.gascity, the
// default folder of an install from there: <root>/.ipm/.gascity with
// <root>/ipm.toml. A recorded install replays into it from anywhere.
func isGascityDefault(dir string) bool {
	ipm := filepath.Dir(dir)
	if filepath.Base(dir) != ".gascity" || filepath.Base(ipm) != ".ipm" {
		return false
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(ipm), project.File))
	return err == nil
}

// ignoreGascity adds .gascity/ to the .gitignore ipm pack build wrote in
// ipmDir, which a build older than the default folder lacks. It keeps the rest
// of the file and creates none: a build writes it.
func ignoreGascity(ipmDir string) error {
	path := filepath.Join(ipmDir, ".gitignore")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == ".gascity/" {
			return nil
		}
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	return os.WriteFile(path, append(data, ".gascity/\n"...), 0o644)
}

// gascityFolder is the folder to write the pack folder into: p as gascityDir
// makes it, or the default folder when p is empty or names it, however
// spelled. create reports a default folder, the local one or another
// project's .ipm/.gascity (as a replay names it): that folder need not exist
// yet, and an install creates it.
func gascityFolder(sys System, scope project.Scope, p string) (dir string, create bool, err error) {
	def := gascityDefault(sys, scope)
	if strings.TrimSpace(p) == "" {
		return def, true, nil
	}
	cwd, home := workDir(sys, scope), sys.Getenv("HOME")
	abs := resolveDir(p, cwd, home)
	if abs == def || realPath(abs) == realPath(def) {
		return def, true, nil
	}
	if isGascityDefault(abs) {
		return abs, true, nil
	}
	dir, err = gascityDir(p, cwd, home)
	return dir, false, err
}

// resolveDir makes p absolute and clean: empty is cwd, a leading ~ is home, a
// relative path is from cwd.
func resolveDir(p, cwd, home string) string {
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
	return filepath.Clean(p)
}

// gascityDir is p as resolveDir makes it, which must be an existing folder.
func gascityDir(p, cwd, home string) (string, error) {
	p = resolveDir(p, cwd, home)
	if info, err := os.Stat(p); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", p)
	}
	return p, nil
}

// gascityRefusal refuses, even with --force, a pack folder that is or holds
// the local project (see localRoot) or ipm's home: replacing it would delete
// them.
func gascityRefusal(sys System, scope project.Scope, folder string) error {
	paths, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	holdsProject := holds(folder, localRoot(sys, scope))
	if !scope.Global && holdsProject {
		return fmt.Errorf("refusing to replace %s: it holds the project", folder)
	}
	if holds(folder, paths.Home) {
		return fmt.Errorf("refusing to replace %s: it holds ipm's home", folder)
	}
	if scope.Global && holdsProject {
		return fmt.Errorf("refusing to replace %s: it holds the project", folder)
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

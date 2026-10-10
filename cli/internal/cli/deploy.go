package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// deployTarget is where a pack is installed in Claude Code: user scope, or
// project or local scope in the folder Path.
type deployTarget struct{ Scope, Path string }

// claudeCmd is one claude command deploy runs, in the folder Dir.
type claudeCmd struct {
	Dir  string
	Args []string
}

func (c claudeCmd) String() string { return "claude " + strings.Join(c.Args, " ") }

func newDeployCmd(sys System) *cobra.Command {
	var global bool
	var t deployTarget
	cmd := &cobra.Command{
		Use:   "deploy [pack]",
		Short: "build a pack and install it into Claude Code (user, project or local scope)",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("deploy: give at most one pack. Try `ipm help deploy`.")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			pack := ""
			if len(args) == 1 {
				pack = args[0]
			}
			return runDeploy(sys, global, pack, t, sys.Stdout)
		},
	}
	f := cmd.Flags()
	f.StringVar(&t.Scope, "scope", "", "user, project or local (default project; user with -g)")
	f.StringVar(&t.Path, "path", "", "the folder for project or local scope (default: the pack's project)")
	f.BoolVarP(&global, "global", "g", false, "deploy a pack from your user scope ($IPM_HOME/global)")
	return cmd
}

// cleanTarget checks a target and makes its path absolute: a leading ~ is
// home, a relative path is from cwd, and it must be an existing folder. User
// scope keeps no path.
func cleanTarget(t deployTarget, cwd, home string) (deployTarget, error) {
	switch t.Scope {
	case "user":
		return deployTarget{Scope: "user"}, nil
	case "project", "local":
	default:
		return deployTarget{}, fmt.Errorf("scope %q: use user, project or local", t.Scope)
	}
	p := strings.TrimSpace(t.Path)
	if p == "" {
		return deployTarget{}, fmt.Errorf("%s scope needs a folder", t.Scope)
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	p = filepath.Clean(p)
	if info, err := os.Stat(p); err != nil || !info.IsDir() {
		return deployTarget{}, fmt.Errorf("%s is not a folder", p)
	}
	return deployTarget{Scope: t.Scope, Path: p}, nil
}

// planDeploy is the claude commands that install pack from the marketplace
// built in ipmDir at target t: always declare the marketplace at the
// target's scope (claude accepts a repeat), refresh it when claude already
// knows it, then install the pack, or update it when it is already
// installed there.
func planDeploy(sys System, ipmDir, pack string, t deployTarget) ([]claudeCmd, error) {
	mp, err := marketplaceName(ipmDir)
	if err != nil {
		return nil, err
	}
	dir := t.Path
	if dir == "" {
		dir = filepath.Dir(ipmDir)
	}
	var marketplaces []struct {
		Name string `json:"name"`
	}
	if err := claudeJSON(sys, dir, &marketplaces, "plugin", "marketplace", "list", "--json"); err != nil {
		return nil, err
	}
	var plugins []struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		ProjectPath string `json:"projectPath"`
	}
	if err := claudeJSON(sys, dir, &plugins, "plugin", "list", "--json"); err != nil {
		return nil, err
	}
	id := pack + "@" + mp
	cmds := []claudeCmd{{dir, []string{"plugin", "marketplace", "add", ipmDir, "--scope", t.Scope}}}
	for _, m := range marketplaces {
		if m.Name == mp {
			cmds = append(cmds, claudeCmd{dir, []string{"plugin", "marketplace", "update", mp}})
			break
		}
	}
	verb := "install"
	for _, p := range plugins {
		if p.ID == id && p.Scope == t.Scope && (t.Scope == "user" || samePath(p.ProjectPath, t.Path)) {
			verb = "update"
			break
		}
	}
	return append(cmds, claudeCmd{dir, []string{"plugin", verb, id, "--scope", t.Scope}}), nil
}

// claudeJSON runs a claude command that prints JSON and decodes it into v.
func claudeJSON(sys System, dir string, v any, args ...string) error {
	line := "claude " + strings.Join(args, " ")
	out, err := sys.Output(dir, "claude", args...)
	if err != nil {
		return fmt.Errorf("`%s` failed: %v: %s", line, err, strings.TrimSpace(out))
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("can't read the output of `%s`: %v", line, err)
	}
	return nil
}

func marketplaceName(ipmDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(ipmDir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return "", fmt.Errorf("no built marketplace in %s — run `ipm install`: %v", ipmDir, err)
	}
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &m); err != nil || m.Name == "" {
		return "", fmt.Errorf("%s/.claude-plugin/marketplace.json has no name", ipmDir)
	}
	return m.Name, nil
}

// samePath reports whether two folder paths are the same folder, following
// symlinks (/tmp and /private/tmp on macOS).
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// runDeploy builds the scope's packs, then installs pack into Claude Code at
// t, writing everything it does to out.
func runDeploy(sys System, global bool, pack string, t deployTarget, out io.Writer) error {
	if _, err := sys.LookPath("claude"); err != nil {
		return errors.New("ipm deploy needs claude (Claude Code) on PATH")
	}
	scope, err := currentScope(sys, global)
	if err != nil {
		return err
	}
	if pack, err = deployPack(scope, pack); err != nil {
		return err
	}
	t = deployDefaults(scope, t)
	cwd := scope.Root
	if sys.Getwd != nil {
		if wd, err := sys.Getwd(); err == nil {
			cwd = wd
		}
	}
	if t, err = cleanTarget(t, cwd, sys.Getenv("HOME")); err != nil {
		return err
	}
	build := sys
	build.Stdout = out
	if err := runInstall(build, global, false, false); err != nil {
		return err
	}
	cmds, err := planDeploy(sys, scope.IPMDir(), pack, t)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		fmt.Fprintf(out, "$ %s\n", c)
		res, err := sys.Output(c.Dir, "claude", c.Args...)
		if res = strings.TrimSpace(res); res != "" {
			fmt.Fprintln(out, res)
		}
		if err != nil {
			return fmt.Errorf("`%s` failed: %v", c, err)
		}
	}
	fmt.Fprintf(out, "%s deployed %s (%s)\n", ok, pack, describeTarget(t))
	return nil
}

// deployPack is the pack to deploy: the one named, or the scope's only pack.
func deployPack(scope project.Scope, pack string) (string, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return "", err
	}
	if pack == "" {
		switch len(packs) {
		case 0:
			return "", fmt.Errorf("no packs in %s — create one with `ipm pack new <name>`", filepath.Join(scope.Root, project.File))
		case 1:
			return packs[0].Name, nil
		}
		return "", usagef("deploy: name a pack — this scope has %d. Try `ipm pack list`.", len(packs))
	}
	if findPack(packs, pack) < 0 {
		return "", fmt.Errorf("no pack %s — `ipm pack list` shows the packs here", pack)
	}
	return pack, nil
}

// deployDefaults fills an unset scope and path: project scope in the pack's
// project, or user scope for the user's own packs.
func deployDefaults(scope project.Scope, t deployTarget) deployTarget {
	if t.Scope == "" {
		t.Scope = "project"
		if scope.Global {
			t.Scope = "user"
		}
	}
	if t.Path == "" && t.Scope != "user" {
		t.Path = scope.Root
	}
	return t
}

// describeTarget is a target as people read it: "user" or "project /work".
func describeTarget(t deployTarget) string {
	if t.Scope == "user" {
		return "user"
	}
	return t.Scope + " " + t.Path
}

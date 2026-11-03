package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/assemble"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// newInstallAgentSkillsCmd is ipm install agentskills: in.Path is the skills
// folder (empty is .agents/skills in the project, or in your home with -g).
func newInstallAgentSkillsCmd(sys System) *cobra.Command {
	var o installOpts
	var in project.Install
	cmd := &cobra.Command{
		Use:   "agentskills [pack]",
		Short: "write a pack's skills into an Agent Skills folder (.agents/skills), building first if needed",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("install agentskills: give at most one pack. Try `ipm help install agentskills`.")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			pack := ""
			if len(args) == 1 {
				pack = args[0]
			}
			if o.DryRun && o.Update {
				return usagef("install agentskills: --dry-run and --update can't be used together")
			}
			scope, err := currentScope(sys, o.Global)
			if err != nil {
				return err
			}
			if in.Path != "" {
				if in.Path, err = gascityDir(in.Path, workDir(sys, scope), sys.Getenv("HOME")); err != nil {
					return err
				}
			}
			done, err := installAgentSkills(sys, scope, pack, in, o, sys.Stdout)
			if err != nil || o.DryRun {
				return err
			}
			fmt.Fprintf(sys.Stdout, "%s installed %s\n", ok, done)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&in.Path, "dir", "C", "", "the skills folder to write into (default: .agents/skills in the project, or in your home with -g)")
	f.BoolVarP(&o.Global, "global", "g", false, "install a pack from your user scope ($IPM_HOME/global) into ~/.agents/skills")
	f.BoolVar(&o.Force, "force", false, "replace a skill folder ipm didn't write, or another pack's")
	f.BoolVar(&o.DryRun, "dry-run", false, "list the skill folders that would be written, without writing or recording anything (needs a current build)")
	f.BoolVar(&o.Update, "update", false, "re-pin every item to your current catalog and rebuild first")
	return cmd
}

// installAgentSkills writes pack's skills from scope's build into an Agent
// Skills folder, writing what it does to out, and records the install. Each
// skill is a folder named by its SKILL.md name; the folder's .ipm-skills.json
// says which are ipm's, so a reinstall removes the skills a pack no longer
// has and leaves the rest alone. It returns what it installed where, as
// "kit (agentskills /work/.agents/skills)".
func installAgentSkills(sys System, scope project.Scope, pack string, in project.Install, o installOpts, out io.Writer) (string, error) {
	pack, err := pickPack(scope, "agentskills", pack)
	if err != nil {
		return "", err
	}
	dir := skillsFolder(sys, scope, in.Path)
	if in.Path != "" {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return "", fmt.Errorf("%s is not a folder", dir)
		}
	}
	if err := gascityBuild(sys, scope, o, out); err != nil {
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
	plan, err := assemble.PlanAgentSkills(packs[findPack(packs, pack)], lock)
	if err != nil {
		return "", err
	}
	for _, s := range plan.Skips {
		fmt.Fprintf(out, "%s %s\n", warn, s)
	}
	fetch := sys
	fetch.Stdout = out
	repos, err := repoFolders(fetch, plan.Repos())
	if err != nil {
		return "", err
	}
	skills, warns, err := assemble.RenderAgentSkills(pack, plan, repos)
	if err != nil {
		return "", err
	}
	for _, w := range warns {
		fmt.Fprintf(out, "%s %s\n", warn, w)
	}
	desc := pack + " (agentskills " + dir + ")"
	if o.DryRun {
		if err := assemble.CheckAgentSkills(dir, pack, skills, o.Force); err != nil {
			fmt.Fprintf(out, "would refuse: %s\n", err)
			return "", err
		}
		for _, s := range skills {
			fmt.Fprintf(out, "would write %s\n", filepath.Join(dir, s.Name))
		}
		return desc, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	removed, err := assemble.WriteAgentSkills(dir, pack, skills, o.Force)
	if err != nil {
		return "", err
	}
	for _, s := range skills {
		fmt.Fprintf(out, "wrote %s\n", filepath.Join(dir, s.Name))
	}
	for _, r := range removed {
		fmt.Fprintf(out, "removed %s: no longer in %s\n", r, pack)
	}
	if err := recordInstall(sys, scope, pack, project.Install{Target: "agentskills", Path: in.Path}); err != nil {
		return "", err
	}
	return desc, nil
}

// skillsFolder is the Agent Skills folder p names: as given when absolute, a
// recorded relative path from the project, and with none the default,
// .agents/skills in the project or, for the user scope, in your home.
func skillsFolder(sys System, scope project.Scope, p string) string {
	switch {
	case p != "" && filepath.IsAbs(p):
		return filepath.Clean(p)
	case p != "":
		return filepath.Join(scope.Root, p)
	case scope.Global:
		return filepath.Join(sys.Getenv("HOME"), ".agents", "skills")
	}
	return filepath.Join(scope.Root, ".agents", "skills")
}

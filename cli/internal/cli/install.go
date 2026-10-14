package cli

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// installOpts are the flags every install target shares.
type installOpts struct{ Global, Update, DryRun, Force bool }

// installer installs pack from scope at in, writing what it does to out, and
// returns what it installed where.
type installer func(sys System, scope project.Scope, pack string, in project.Install, o installOpts, out io.Writer) (string, error)

// installers are the install targets, by the name ipm install takes and
// records.
var installers = map[string]installer{
	"claude":  installClaude,
	"gascity": installGascity,
}

// targetNames is the install targets for a message, sorted: "claude", or
// "claude or gascity".
func targetNames() string {
	names := slices.Sorted(maps.Keys(installers))
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// newInstallCmd is ipm install, also ipm pack install: with a target it
// installs one pack there; bare, it replays the scope's recorded installs.
func newInstallCmd(sys System) *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "install [target] [pack]",
		Short: "install a pack into claude or gascity (same as pack install)",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usagef("install: unknown target %q%s — use %s. Try `ipm help install`.", args[0], didYouMean(c, args[0]), targetNames())
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return replayInstalls(sys, global) },
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "redo the installs of your user scope ($IPM_HOME/global)")
	cmd.AddCommand(newInstallClaudeCmd(sys), newInstallGascityCmd(sys))
	return cmd
}

func newInstallClaudeCmd(sys System) *cobra.Command {
	var o installOpts
	var in project.Install
	cmd := &cobra.Command{
		Use:   "claude [pack]",
		Short: "build a pack if needed and install it into Claude Code (user, project or local scope)",
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usagef("install claude: give at most one pack. Try `ipm help install claude`.")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			pack := ""
			if len(args) == 1 {
				pack = args[0]
			}
			if o.DryRun && o.Update {
				return usagef("install claude: --dry-run and --update can't be used together")
			}
			scope, err := currentScope(sys, o.Global)
			if err != nil {
				return err
			}
			done, err := installClaude(sys, scope, pack, in, o, sys.Stdout)
			if err != nil || o.DryRun {
				return err
			}
			fmt.Fprintf(sys.Stdout, "%s installed %s\n", ok, done)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&in.Scope, "scope", "s", "", "user, project or local (default project; user with -g)")
	f.StringVarP(&in.Path, "path", "p", "", "the folder for project or local scope (default: the pack's project)")
	f.BoolVarP(&o.Global, "global", "g", false, "install a pack from your user scope ($IPM_HOME/global)")
	f.BoolVar(&o.Update, "update", false, "re-pin every item to your current catalog and rebuild first")
	f.BoolVar(&o.DryRun, "dry-run", false, "print the claude commands without building, running or recording anything")
	return cmd
}

// replayInstalls is bare ipm install: every recorded install in the scope,
// in loadInstalls order, through its target's installer. Every target is
// checked before anything runs; a failing record doesn't stop the rest, and
// the replay ends with exit 1 and the file that holds the records.
func replayInstalls(sys System, global bool) error {
	scope, err := currentScope(sys, global)
	if err != nil {
		return err
	}
	recs, err := loadInstalls(sys, scope)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return usagef("install: say %s — nothing is recorded for this scope yet. Try `ipm help install`.", targetNames())
	}
	file, err := installsFileName(sys, scope)
	if err != nil {
		return err
	}
	var unknown []string
	for _, rec := range recs {
		if _, found := installers[rec.Target]; !found {
			unknown = append(unknown, fmt.Sprintf("%s: unknown target %q (use %s)", describeRecord(rec), rec.Target, targetNames()))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%s\nnothing was installed; remove the stale record from %s or upgrade ipm", strings.Join(unknown, "\n"), file)
	}
	failed := 0
	for _, rec := range recs {
		done, err := installers[rec.Target](sys, scope, rec.Pack, rec.Install, installOpts{Global: global}, sys.Stdout)
		if err != nil {
			failed++
			fmt.Fprintf(sys.Stdout, "%s %s: %v\n", bad, describeRecord(rec), err)
			continue
		}
		fmt.Fprintf(sys.Stdout, "%s installed %s\n", ok, done)
	}
	if failed > 0 {
		fmt.Fprintf(sys.Stdout, "%s %d of %d recorded installs failed; if a record is stale, remove it from %s\n", bad, failed, len(recs), file)
		return &ExitError{Code: 1}
	}
	return nil
}

// describeRecord is rec as "<pack> → <target> <scope or name> <path>",
// leaving out what is empty.
func describeRecord(rec installRecord) string {
	parts := []string{rec.Target}
	for _, p := range []string{rec.Scope, rec.Name, rec.Path} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return rec.Pack + " → " + strings.Join(parts, " ")
}

// installsFileName names the file that holds scope's install records, with
// its path: the project's ipm.toml, or $IPM_HOME/installs.json for -g.
func installsFileName(sys System, scope project.Scope) (string, error) {
	if !scope.Global {
		return project.File + " (" + filepath.Join(scope.Root, project.File) + ")", nil
	}
	path, err := globalInstallsPath(sys)
	if err != nil {
		return "", err
	}
	return "$IPM_HOME/" + installsFile + " (" + path + ")", nil
}

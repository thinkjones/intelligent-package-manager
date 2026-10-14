// Package cli is ipm's command line, built with Cobra. Every command is Go
// code in this package; newRootCmd assembles them and Run executes argv.
//
// Commands reach the outside world only through System, so tests drive them
// in-process with a fake: no real git, claude or environment.
package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"
)

// System is everything a command needs from outside the process.
type System struct {
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	// TermWidth is stdout's terminal width in columns, 0 when stdout is not
	// a terminal; nil is 0.
	TermWidth func() int
	// LookPath reports whether a program is on PATH.
	LookPath func(string) (string, error)
	// Output runs name with args in dir and returns its combined output.
	Output func(dir, name string, args ...string) (string, error)
	// HTTP is the client for the local Typesense server; nil uses defaults.
	HTTP *http.Client
	// Getwd is the working directory, where a project's ipm.toml is found.
	Getwd func() (string, error)
	// Now is the current time; tests fix it.
	Now     func() time.Time
	Version string
	// Stdin is where prompts read answers; nil reads nothing.
	Stdin io.Reader
	// Interactive reports whether stdin and stdout are both terminals; nil
	// is false.
	Interactive func() bool
	// Executable is the running binary's path; nil uses os.Executable.
	Executable func() (string, error)
	// RunChecklist shows ipm onboard's checklist until the user confirms or
	// quits; nil runs it as a Bubble Tea program.
	RunChecklist func(*tui.Checklist) error
	// Progress is told how far a long step has got: done of total, what
	// being the latest. ipm onboard sets it to move its bar; nil is silent.
	// It may be called from several goroutines.
	Progress func(done, total int, what string)
}

func (s System) progress(done, total int, what string) {
	if s.Progress != nil {
		s.Progress(done, total, what)
	}
}

// ExitError ends ipm with Code and no further message: the output already
// said why.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// UsageError is a malformed command line; ipm exits 2.
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &UsageError{fmt.Sprintf(format, a...)} }

// Run executes the command line args (without the program name).
func Run(args []string, sys System) error {
	root := newRootCmd(sys)
	root.SetArgs(args)
	return root.Execute()
}

// newRootCmd is ipm's command tree. Every command writes to sys and returns
// a UsageError for a malformed command line, so ipm exits 2 for those.
func newRootCmd(sys System) *cobra.Command {
	// Help lists commands in registration order, which is the workflow's.
	// The setting is process-global; every ipm command tree wants it.
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:   "ipm",
		Short: "ipm — intelligent package manager for agent skills, agents, commands and plugins",
		Long: `ipm — intelligent package manager for agent skills, agents, commands and plugins

Follow repos, search everything in them, and build the skills, agents,
commands and plugins you pick into Claude Code plugins, pinned to commits.

Workflow: onboard → marketplace → catalog → search → pack → install

Config
  IPM_HOME        sources and the pnpm store (default ~/Library/ipm, ~/.local/share/ipm)
  IPM_CACHE_DIR   catalog and search data (default ~/Library/Caches/ipm, ~/.cache/ipm)`,
		Version:       sys.Version,
		Args:          unknownCommand,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetOut(sys.Stdout)
	root.SetErr(sys.Stderr)
	root.SetVersionTemplate("ipm {{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		msg := err.Error()
		// pflag reads -upgrade as -u -p -g …; say what was probably meant.
		if _, word, found := strings.Cut(msg, " in -"); found && strings.HasPrefix(msg, "unknown shorthand flag") && len(word) > 1 {
			msg += " (long flags take two dashes: --" + word + ")"
		}
		return usagef("%s. Try `%s`.", msg, helpFor(cmd))
	})
	for _, g := range [][2]string{
		{"start", "Getting started:"},
		{"market", "Marketplace — curated repos you can follow:"},
		{"catalog", "Catalog — the repos you follow, fetched and searchable:"},
		{"packs", "Packs — your picks, built and installed:"},
		{"tools", "Tools:"},
	} {
		root.AddGroup(&cobra.Group{ID: g[0], Title: g[1]})
	}
	root.SetHelpCommandGroupID("tools")
	root.SetCompletionCommandGroupID("tools")
	root.SetHelpCommand(&cobra.Command{
		Use:     "help [command]",
		GroupID: "tools",
		Short:   "help for ipm, or for one command",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil || len(rest) > 0 {
				return usagef("no command %q. Try `ipm help`.", strings.Join(args, " "))
			}
			return target.Help()
		},
	})
	root.AddCommand(
		newOnboardCmd(sys),
		newMarketplaceCmd(sys),
		newCatalogCmd(sys),
		newSearchCmd(sys),
		newShowCmd(sys),
		newPackCmd(sys),
		newInstallCmd(sys),
		newTuiCmd(sys),
		newDoctorCmd(sys),
		newUninstallCmd(sys),
		renamed("source", "marketplace"),
		renamed("stats", "marketplace stats"),
		renamed("sync", "catalog sync"),
		renamed("index", "catalog rebuild"),
		renamed("load", "catalog rebuild"),
		renamed("serve", "catalog serve"),
		renamed("scan", "catalog scan"),
		renamed("tree", "show tree"),
		renamed("graph", "show graph"),
		renamed("deploy", "install claude"),
		&cobra.Command{
			Use:     "version",
			GroupID: "tools",
			Short:   "print ipm's version",
			Args:    noArgs,
			RunE: func(*cobra.Command, []string) error {
				fmt.Fprintf(sys.Stdout, "ipm %s\n", sys.Version)
				return nil
			},
		},
	)
	groups := map[string]string{
		"onboard": "start", "marketplace": "market",
		"catalog": "catalog", "search": "catalog", "show": "catalog",
		"pack": "packs", "install": "packs",
		"tui": "tools", "doctor": "tools", "uninstall": "tools",
	}
	for _, c := range root.Commands() {
		if g, ok := groups[c.Name()]; ok {
			c.GroupID = g
		}
	}
	// Cobra adds its help command first and completion after it; adding
	// completion now puts help last, as the Tools list reads.
	root.InitDefaultCompletionCmd()
	return root
}

// renamed is a stub for a command that moved: hidden from help, it takes
// any flags and arguments and says where the command went.
func renamed(old, now string) *cobra.Command {
	return &cobra.Command{
		Use:                old,
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error {
			return usagef("ipm %s was renamed: use ipm %s. Try `ipm help`.", old, now)
		},
	}
}

// helpFor is the help command for cmd: `ipm help sync`, or `ipm help`.
func helpFor(cmd *cobra.Command) string {
	return strings.TrimSpace("ipm help " + strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "ipm")))
}

// unknownCommand rejects words that name no command, suggesting near ones.
func unknownCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return usagef("unknown command %q%s. Try `ipm help`.", args[0], didYouMean(cmd, args[0]))
}

// didYouMean is " — did you mean x?" for the subcommands of cmd near word.
func didYouMean(cmd *cobra.Command, word string) string {
	cmd.SuggestionsMinimumDistance = 2
	if s := cmd.SuggestionsFor(word); len(s) > 0 {
		return fmt.Sprintf(" — did you mean %s?", strings.Join(s, " or "))
	}
	return ""
}

// noArgs, exactArgs and minArgs check positional arguments, failing with a
// UsageError that says what the command wants.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef("%s: unexpected argument %q. Try `%s`.", cmd.CommandPath(), args[0], helpFor(cmd))
	}
	return nil
}

func exactArgs(n int, want string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("%s: give %s. Try `%s`.", cmd.CommandPath(), want, helpFor(cmd))
		}
		return nil
	}
}

func minArgs(n int, want string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usagef("%s: give %s. Try `%s`.", cmd.CommandPath(), want, helpFor(cmd))
		}
		return nil
	}
}

// verbs is a parent command such as `ipm pack`: run alone, or with a word
// that names none of its subcommands, it is a usage error listing them.
func verbs(cmd *cobra.Command, list string) *cobra.Command {
	cmd.Args = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usagef("%s: unknown action %q%s — use %s. Try `%s`.", c.CommandPath(), args[0], didYouMean(c, args[0]), list, helpFor(c))
		}
		return nil
	}
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		return usagef("%s: say %s. Try `%s`.", c.CommandPath(), list, helpFor(c))
	}
	return cmd
}

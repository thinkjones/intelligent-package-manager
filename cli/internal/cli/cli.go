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
	root := &cobra.Command{
		Use:   "ipm",
		Short: "ipm — intelligent package manager for agent skills, agents, commands and plugins",
		Long: `ipm — intelligent package manager for agent skills, agents, commands and plugins

Follow repos, search everything in them, and build the skills, agents,
commands and plugins you pick into Claude Code plugins, pinned to commits.

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
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "help for ipm, or for one command",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil || len(rest) > 0 {
				return usagef("no command %q. Try `ipm help`.", strings.Join(args, " "))
			}
			return target.Help()
		},
	})
	root.AddCommand(
		newSourceCmd(sys),
		newSyncCmd(sys),
		newDoctorCmd(sys),
		newIndexCmd(sys),
		newServeCmd(sys),
		newLoadCmd(sys),
		newSearchCmd(sys),
		newShowCmd(sys),
		newTreeCmd(sys),
		newGraphCmd(sys),
		newPackCmd(sys),
		newInstallCmd(sys),
		newStatsCmd(sys),
		newScanCmd(sys),
		&cobra.Command{
			Use:   "version",
			Short: "print ipm's version",
			Args:  noArgs,
			RunE: func(*cobra.Command, []string) error {
				fmt.Fprintf(sys.Stdout, "ipm %s\n", sys.Version)
				return nil
			},
		},
	)
	return root
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

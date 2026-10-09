// Package cli is aihs's command line. Every command is Go code in this
// package, registered in the commands table; Run picks one from argv.
//
// Commands reach the outside world only through System, so tests drive them
// in-process with a fake: no real git, claude or environment.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// System is everything a command needs from outside the process.
type System struct {
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	// LookPath reports whether a program is on PATH.
	LookPath func(string) (string, error)
	// Output runs name with args in dir and returns its combined output.
	Output func(dir, name string, args ...string) (string, error)
	// HTTP is the client for the local Typesense server; nil uses defaults.
	HTTP *http.Client
	// Now is the current time; tests fix it.
	Now     func() time.Time
	Version string
}

// ExitError ends aihs with Code and no further message: the output already
// said why.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// UsageError is a malformed command line; aihs exits 2.
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &UsageError{fmt.Sprintf(format, a...)} }

type command struct {
	name    string
	summary string
	// args names the positional arguments the command takes, for help; empty
	// means none. The command reads them from its flag set's Args.
	args string
	// flags returns the command's flag set, bound to fresh option values, and
	// the function that runs the command with them.
	flags func() (*flag.FlagSet, func(sys System) error)
}

var commands []command

func init() {
	commands = []command{
		{"sync", "cache every upstream the packs and repos of interest read from and pin it in aihs.lock", "", syncFlags},
		{"create-plugin", "write .claude-plugin/marketplace.json from the packs' manifest.toml", "", createPluginFlags},
		{"doctor", "check config and every pack manifest — exits 1 on problems", "", doctorFlags},
		{"index", "write catalog/*.jsonl from the repos of interest at their locked commits", "", indexFlags},
		{"serve", "start the local Typesense search server in Docker (--stop to stop it)", "", serveFlags},
		{"load", "rebuild the search index from catalog/", "", loadFlags},
		{"search", "hybrid search over the catalog (needs aihs serve and aihs load)", "<query>", searchFlags},
		{"show", "an artifact's fields, GitHub stats and neighbours (reads catalog/)", "<id>", showFlags},
		{"tree", "an artifact's relationships as a tree (reads catalog/)", "<id>", treeFlags},
		{"graph", "an artifact's neighbourhood as Mermaid or Graphviz (reads catalog/)", "<id>", graphFlags},
		{"stats", "fetch GitHub stats for every repo in the catalog into catalog/github.jsonl", "", statsFlags},
		{"promote", "add repos (or an external plugin's source repo) to the root manifest.toml", "<repo|plugin-id>...", promoteFlags},
		{"scan", "cache repos and report their plugin, skills, hooks and agents folders", "[repo...]", scanFlags},
		{"version", "print aihs's version", "", versionFlags},
		{"help", "this text, or `help <command>` for one command's flags", "", nil},
	}
}

func lookup(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// Run executes the command line args (without the program name).
func Run(args []string, sys System) error {
	if len(args) == 0 {
		printHelp(sys.Stdout)
		return nil
	}
	name, rest := args[0], args[1:]
	switch name {
	case "help", "-h", "--help":
		return runHelp(rest, sys)
	case "--version":
		name = "version"
	}
	cmd, ok := lookup(name)
	if !ok {
		if strings.HasPrefix(name, "-") {
			return usagef("unknown flag %q. Try `aihs help`.", name)
		}
		return usagef("unknown command %q. Try `aihs help`.", name)
	}
	fs, run := cmd.flags()
	fs.SetOutput(io.Discard)
	parse := fs.Parse
	if cmd.args != "" {
		parse = func(args []string) error {
			words, err := interspersed(fs, args)
			if err != nil {
				return err
			}
			// Leave exactly the positional words in fs.Args().
			return fs.Parse(append([]string{"--"}, words...))
		}
	}
	if err := parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCommandHelp(sys.Stdout, cmd)
			return nil
		}
		return usagef("%s: %v. Try `aihs help %s`.", cmd.name, err, cmd.name)
	}
	if fs.NArg() > 0 && cmd.args == "" {
		return usagef("%s: unexpected argument %q. Try `aihs help %s`.", cmd.name, fs.Arg(0), cmd.name)
	}
	return run(sys)
}

func runHelp(args []string, sys System) error {
	switch len(args) {
	case 0:
		printHelp(sys.Stdout)
		return nil
	case 1:
		cmd, ok := lookup(args[0])
		if !ok || cmd.flags == nil {
			return usagef("no command %q. Try `aihs help`.", args[0])
		}
		printCommandHelp(sys.Stdout, cmd)
		return nil
	}
	return usagef("help takes at most one command")
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "aihs — local tooling for this checkout's packs")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage")
	fmt.Fprintln(w, "  aihs <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-15s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Config")
	fmt.Fprintln(w, "  AIHS_ROOT       this checkout's root, set by bin/aihs (on PATH after direnv allow)")
}

func printCommandHelp(w io.Writer, cmd command) {
	fmt.Fprintf(w, "aihs %s — %s\n", cmd.name, cmd.summary)
	if cmd.args != "" {
		fmt.Fprintf(w, "\nUsage\n  aihs %s [flags] %s\n", cmd.name, cmd.args)
	}
	fs, _ := cmd.flags()
	any := false
	fs.VisitAll(func(*flag.Flag) { any = true })
	if !any {
		return
	}
	fmt.Fprintln(w, "\nFlags")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func versionFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	return fs, func(sys System) error {
		fmt.Fprintf(sys.Stdout, "aihs %s\n", sys.Version)
		return nil
	}
}

// multi is a repeatable string flag.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

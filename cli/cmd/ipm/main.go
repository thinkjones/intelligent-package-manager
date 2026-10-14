// ipm is an intelligent package manager for agent skills, agents, commands
// and plugins. Every command is Go code in internal/cli:
//
//	ipm onboard                  learn the concepts, pick repos, build your catalog
//	ipm marketplace              the curated repos and which you follow (add, remove, import, stats)
//	ipm catalog | search | show  what you follow, fetched and searchable (sync, rebuild, serve, scan)
//	ipm pack | install           build picks into plugins and install them (claude, gascity)
//	ipm tui | doctor | uninstall the TUI, checks, removal
//	ipm version | help [command]
//
// Data lives in IPM_HOME and IPM_CACHE_DIR (see internal/paths). install.sh
// installs this binary; bin/ipm in the source checkout builds it on demand.
// main only wires the process into cli.Run. See cli/README.md.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/cli"
	"golang.org/x/term"
)

// version is the module version Go records in the binary, so it always
// matches the release tag it was installed from.
func version() string {
	info, _ := debug.ReadBuildInfo()
	return versionFrom(info)
}

// versionFrom reads "0.2.0" from an installed release, a pseudo-version from
// go install of a commit between releases, and "dev" from a build of a
// checkout, where Go records no module version.
func versionFrom(info *debug.BuildInfo) string {
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

func main() {
	os.Exit(run())
}

func run() int {
	err := cli.Run(os.Args[1:], cli.System{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
		TermWidth: func() int {
			if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
				return w
			}
			return 0
		},
		LookPath: exec.LookPath,
		Output: func(dir, name string, args ...string) (string, error) {
			cmd := exec.Command(name, args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		HTTP:    &http.Client{},
		Getwd:   os.Getwd,
		Now:     time.Now,
		Version: version(),
		Stdin:   os.Stdin,
		Interactive: func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		},
		Executable: os.Executable,
	})
	var exit *cli.ExitError
	var usage *cli.UsageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.Code
	case errors.As(err, &usage):
		fmt.Fprintf(os.Stderr, "ipm: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "ipm: %v\n", err)
	return 1
}

// ipm is an intelligent package manager for agent skills, agents, commands
// and plugins. Every command is Go code in internal/cli:
//
//	ipm source | sync | index    your repos of interest, fetched by pnpm
//	ipm stats                    GitHub stats for the catalog
//	ipm serve | load | search    local hybrid search over the catalog
//	ipm show | tree | graph      an artifact and its relationships
//	ipm scan | doctor            quick checks
//	ipm version | help [command]
//
// Data lives in IPM_HOME and IPM_CACHE_DIR (see internal/paths). bin/ipm in
// the source checkout builds this binary on demand. main only wires the
// process into cli.Run. See cli/README.md.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/thinkjones/fs-skills/cli/internal/cli"
)

const version = "0.2.0"

func main() {
	os.Exit(run())
}

func run() int {
	err := cli.Run(os.Args[1:], cli.System{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Getenv:   os.Getenv,
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
		Version: version,
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

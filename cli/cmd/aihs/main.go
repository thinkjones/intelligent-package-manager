// aihs is local tooling for this checkout's packs. Every command is Go code
// in internal/cli:
//
//	aihs create-plugin [flags]   write .claude-plugin/marketplace.json
//	aihs doctor                  check config and every pack manifest
//	aihs index | stats | promote build the artifact catalog in catalog/
//	aihs version | help [command]
//
// bin/aihs at the checkout root builds this binary on demand and sets
// AIHS_ROOT. main only wires the process into cli.Run. See cli/README.md.
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
		fmt.Fprintf(os.Stderr, "aihs: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "aihs: %v\n", err)
	return 1
}

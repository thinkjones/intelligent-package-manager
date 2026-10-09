// Package config resolves aihs's configuration from the environment.
//
// There is one value, AIHS_ROOT, and only the checkout's bin/aihs shim
// sets it — to the checkout's root, where the packs live. aihs is a local
// tool: there is no user config file, no installed binary, and nothing shared
// with Gas City or fsh. Missing config is an error that says how to run aihs.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// RootVar is the variable aihs reads, set by bin/aihs.
const RootVar = "AIHS_ROOT"

const source = "bin/aihs sets it; run aihs inside the ai-harness-skills checkout after `direnv allow`"

type Config struct {
	// Root is the checkout's root, where the packs live.
	Root string
}

// Load reads the configuration through getenv and checks it names a real
// directory.
func Load(getenv func(string) string) (Config, error) {
	dir := getenv(RootVar)
	if dir == "" {
		return Config{}, fmt.Errorf("%s is not set — %s", RootVar, source)
	}
	if !filepath.IsAbs(dir) {
		return Config{}, fmt.Errorf("%s must be an absolute path, got %q — %s", RootVar, dir, source)
	}
	if err := isDir(dir); err != nil {
		return Config{}, fmt.Errorf("%s=%s: %v — %s", RootVar, dir, err, source)
	}
	return Config{Root: dir}, nil
}

func isDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("does not exist")
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("is not a directory")
	}
	return nil
}

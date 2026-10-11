package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newUninstallCmd(sys System) *cobra.Command {
	var purge, yes, force bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "remove ipm and its search container (--purge also deletes your data)",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runUninstall(sys, purge, yes, force) },
	}
	f := cmd.Flags()
	f.BoolVar(&purge, "purge", false, "also delete ipm's home and cache (asks first)")
	f.BoolVar(&yes, "yes", false, "with --purge, delete without asking")
	f.BoolVar(&force, "force", false, "remove a development build in a git checkout too")
	return cmd
}

// runUninstall removes the search container and the running binary, and
// with purge, ipm's home and cache. Everything is checked before anything
// is removed.
func runUninstall(sys System, purge, yes, force bool) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	exe := sys.Executable
	if exe == nil {
		exe = os.Executable
	}
	bin, err := exe()
	if err != nil {
		return fmt.Errorf("can't find the ipm binary: %v", err)
	}
	if real, err := filepath.EvalSymlinks(bin); err == nil {
		bin = real
	}
	if devBuild(bin) && !force {
		return fmt.Errorf("%s is a development build in a git checkout — rerun with --force to remove it", bin)
	}
	if purge {
		for _, dir := range []string{p.Home, p.Cache} {
			if err := purgeRefusal(dir, sys.Getenv("HOME")); err != nil {
				return err
			}
		}
	}
	interactive := sys.Interactive != nil && sys.Interactive()
	if purge && !yes && !interactive {
		return usagef("uninstall: --purge needs --yes when not in a terminal")
	}
	if purge && !yes {
		fmt.Fprintf(sys.Stdout, "type %s to delete it and %s: ", p.Home, p.Cache)
		line, _ := bufio.NewReader(orEmpty(sys.Stdin)).ReadString('\n')
		if strings.TrimSpace(line) != p.Home {
			fmt.Fprintln(sys.Stdout, "\nnothing deleted")
			return &ExitError{Code: 1}
		}
	}

	if _, err := sys.LookPath("docker"); err != nil {
		fmt.Fprintln(sys.Stdout, "- docker not available: skipped the search container")
	} else if _, err := sys.Output("", "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		fmt.Fprintln(sys.Stdout, "- docker not available: skipped the search container")
	} else if out, err := sys.Output("", "docker", "rm", "-f", tsContainer); err != nil {
		if !strings.Contains(out, "No such container") {
			return fmt.Errorf("docker rm -f %s: %v: %s", tsContainer, err, strings.TrimSpace(out))
		}
		fmt.Fprintln(sys.Stdout, "- no search container")
	} else {
		fmt.Fprintf(sys.Stdout, "%s removed the %s container\n", ok, tsContainer)
	}

	if err := os.Remove(bin); err != nil {
		return fmt.Errorf("can't remove %s: %v", bin, err)
	}
	fmt.Fprintf(sys.Stdout, "%s removed %s\n", ok, bin)

	if !purge {
		fmt.Fprintf(sys.Stdout, "kept your data: %s, %s — ipm onboard will offer to use or back it up\n", p.Home, p.Cache)
		return nil
	}
	for _, dir := range []string{p.Home, p.Cache} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("can't delete %s: %v", dir, err)
		}
		fmt.Fprintf(sys.Stdout, "%s deleted %s\n", ok, dir)
	}
	return nil
}

// devBuild reports whether bin is a development build: its folder is named
// bin and a folder above it is an ipm checkout (it holds .git and
// cli/go.mod). A home folder kept in git (dotfiles) is not a checkout.
func devBuild(bin string) bool {
	dir := filepath.Dir(bin)
	if filepath.Base(dir) != "bin" {
		return false
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			_, err := os.Stat(filepath.Join(d, "cli", "go.mod"))
			return err == nil
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

// purgeRefusal refuses to delete dir when it is the filesystem root, the
// user's home folder or a folder above it — an IPM_HOME or IPM_CACHE_DIR
// pointed somewhere it shouldn't be.
func purgeRefusal(dir, userHome string) error {
	d := filepath.Clean(dir)
	if d == string(filepath.Separator) || filepath.Dir(d) == d {
		return fmt.Errorf("refusing to delete %s: it is the root of the filesystem", dir)
	}
	if userHome != "" && inside(filepath.Clean(userHome), d) {
		return fmt.Errorf("refusing to delete %s: it holds your home folder %s — check IPM_HOME and IPM_CACHE_DIR", dir, userHome)
	}
	return nil
}

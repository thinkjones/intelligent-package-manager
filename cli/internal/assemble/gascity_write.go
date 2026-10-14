package assemble

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotOurs marks a refusal to replace a folder ipm did not write.
var ErrNotOurs = errors.New("not written by ipm")

// WriteGascity writes files as the folder <dir>/<name> and returns its
// absolute path. dir must exist. The folder is built beside the target and
// swapped in, so a failure leaves the old folder in place. A target that is
// not ours (no pack.toml starting with GasHeader) is refused unless force is
// set; a missing target or an empty directory needs no force.
func WriteGascity(files []GasFile, dir, name string, force bool) (string, error) {
	if !ValidGasName(name) {
		return "", fmt.Errorf("invalid pack name %q", name)
	}
	for _, f := range files {
		if !filepath.IsLocal(filepath.FromSlash(f.Path)) {
			return "", fmt.Errorf("invalid file path %q in pack %s", f.Path, name)
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(dir); err != nil {
		return "", err
	} else if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	target := filepath.Join(dir, name)
	tmp := filepath.Join(dir, "."+name+".ipm-tmp")
	old := filepath.Join(dir, "."+name+".ipm-old")

	exists, err := gasCheckTarget(target, force)
	if err != nil {
		return "", err
	}

	for _, p := range []string{tmp, old} {
		if err := os.RemoveAll(p); err != nil {
			return "", err
		}
	}
	if err := gasWriteTree(files, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if exists {
		if err := os.Rename(target, old); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		if exists {
			_ = os.Rename(old, target)
		}
		_ = os.RemoveAll(tmp)
		return "", err
	}
	if exists {
		if err := os.RemoveAll(old); err != nil {
			return "", err
		}
	}
	return target, nil
}

// gasCheckTarget reports whether target exists and, unless force is set,
// refuses one that ipm did not write.
func gasCheckTarget(target string, force bool) (bool, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if force || gasIsOurs(target, info) {
		return true, nil
	}
	return true, fmt.Errorf("%s exists and was not written by ipm — use --force to replace it: %w", target, ErrNotOurs)
}

func gasIsOurs(target string, info os.FileInfo) bool {
	if !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(target)
	if err == nil && len(entries) == 0 {
		return true
	}
	f, err := os.Open(filepath.Join(target, "pack.toml"))
	if err != nil {
		return false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return strings.TrimRight(line, "\r\n") == GasHeader
}

func gasWriteTree(files []GasFile, root string) error {
	if err := os.Mkdir(root, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Data, f.Mode.Perm()); err != nil {
			return err
		}
		if err := os.Chmod(p, f.Mode.Perm()); err != nil {
			return err
		}
	}
	return nil
}

package sourcepacks

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
)

// GitHub is where repos are fetched from; tests point it at a local server.
const GitHub = "https://github.com"

// maxFiles stops a repo with an absurd number of files filling the disk
// with empty ones.
const maxFiles = 400_000

// Fetcher scans repos by downloading the archive of their default branch.
// Only file names are kept: every file is written empty under Tmp, which is
// all catalog.Walk reads, and the folder is removed after the scan.
type Fetcher struct {
	Client  *http.Client
	Base    string        // GitHub, or a test server
	Tmp     string        // where archives are unpacked; "" is the system's
	Timeout time.Duration // per repo
}

// Scan is a Scan for Update.
func (f Fetcher) Scan(repo string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.Base+"/"+repo+"/archive/HEAD.tar.gz", nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	dir, err := os.MkdirTemp(f.Tmp, "sourcepacks-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := unpackNames(resp.Body, dir); err != nil {
		return nil, err
	}
	return CountDir(dir)
}

// unpackNames writes every regular file of a gzipped tar under dir, empty,
// dropping the archive's top folder. Links and paths that would leave dir
// are ignored.
func unpackNames(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("not a gzip archive: %v", err)
	}
	tr := tar.NewReader(gz)
	made := map[string]bool{}
	for n := 0; ; {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		top, rel, ok := strings.Cut(filepath.ToSlash(h.Name), "/")
		if !ok || top == "" || top == ".." || !filepath.IsLocal(filepath.FromSlash(rel)) {
			continue
		}
		if n++; n > maxFiles {
			return fmt.Errorf("more than %d files", maxFiles)
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if parent := filepath.Dir(path); !made[parent] {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return err
			}
			made[parent] = true
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return err
		}
	}
}

// CountDir counts what ipm's catalog would find in the checkout at dir, by
// kind; kinds with nothing are left out.
func CountDir(dir string) (map[string]int, error) {
	found, err := catalog.Walk(dir)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for k, n := range map[string]int{
		"plugins": len(found.Plugins), "marketplaces": len(found.Marketplaces), "copilot_plugins": len(found.CopilotPlugins),
		"skills": len(found.Skills), "agents": len(found.Agents), "commands": len(found.Commands),
		"hooks": len(found.HookFiles), "mcp": len(found.MCPFiles),
	} {
		if n > 0 {
			counts[k] = n
		}
	}
	return counts, nil
}

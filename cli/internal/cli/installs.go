package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// installsFile, in $IPM_HOME, records where the user-scope packs are installed;
// a project's installs live in its ipm.toml.
const installsFile = "installs.json"

// installRecord is one place a pack is installed.
type installRecord struct {
	Pack string
	project.Install
}

type installJSON struct {
	Pack   string `json:"pack"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Scope  string `json:"scope,omitempty"`
	Name   string `json:"name,omitempty"`
}

func sortInstalls(rs []installRecord) {
	slices.SortStableFunc(rs, func(a, b installRecord) int {
		return cmp.Or(cmp.Compare(a.Pack, b.Pack), cmp.Compare(a.Target, b.Target), cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Scope, b.Scope), cmp.Compare(a.Name, b.Name))
	})
}

// sameInstall reports whether a and b are the same install: Claude's project
// and local scopes at one folder are two installs.
func sameInstall(a, b project.Install) bool {
	return a.Target == b.Target && a.Path == b.Path && a.Scope == b.Scope && a.Name == b.Name
}

// relativeTo is path made relative to root when it lies inside it.
func relativeTo(root, path string) string {
	if path == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

func globalInstallsPath(sys System) (string, error) {
	p, err := ipmPaths(sys)
	if err != nil {
		return "", err
	}
	return filepath.Join(p.Home, installsFile), nil
}

func readGlobalInstalls(path string) ([]installRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var raw []installJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New(path + ": " + err.Error())
	}
	out := make([]installRecord, len(raw))
	for i, r := range raw {
		out[i] = installRecord{r.Pack, project.Install{Target: r.Target, Path: r.Path, Scope: r.Scope, Name: r.Name}}
	}
	return out, nil
}

// recordInstall notes that pack is installed at in, replacing an earlier record
// of the same target, path, scope and name. A project keeps it in ipm.toml, with a path
// inside the project made relative; the user scope keeps it in installs.json.
func recordInstall(sys System, scope project.Scope, pack string, in project.Install) error {
	if scope.Global {
		path, err := globalInstallsPath(sys)
		if err != nil {
			return err
		}
		recs, err := readGlobalInstalls(path)
		if err != nil {
			return err
		}
		recs = slices.DeleteFunc(recs, func(r installRecord) bool { return r.Pack == pack && sameInstall(r.Install, in) })
		return writeGlobalInstalls(path, append(recs, installRecord{pack, in}))
	}
	in.Path = relativeTo(scope.Root, in.Path)
	_, err := editPack(scope, pack, func(p *project.Pack) error {
		for i := range p.Installs {
			if sameInstall(p.Installs[i], in) {
				p.Installs[i] = in
				return nil
			}
		}
		p.Installs = append(p.Installs, in)
		return nil
	})
	return err
}

// forgetInstall removes rec, as loadInstalls lists it, from scope's records.
func forgetInstall(sys System, scope project.Scope, rec installRecord) error {
	if scope.Global {
		path, err := globalInstallsPath(sys)
		if err != nil {
			return err
		}
		recs, err := readGlobalInstalls(path)
		if err != nil {
			return err
		}
		return writeGlobalInstalls(path, slices.DeleteFunc(recs, func(r installRecord) bool { return r.Pack == rec.Pack && sameInstall(r.Install, rec.Install) }))
	}
	in := rec.Install
	in.Path = relativeTo(scope.Root, in.Path)
	_, err := editPack(scope, rec.Pack, func(p *project.Pack) error {
		p.Installs = slices.DeleteFunc(p.Installs, func(i project.Install) bool { return sameInstall(i, in) })
		return nil
	})
	return err
}

// writeGlobalInstalls writes recs, sorted, to installs.json at path.
func writeGlobalInstalls(path string, recs []installRecord) error {
	sortInstalls(recs)
	raw := make([]installJSON, len(recs))
	for i, r := range recs {
		raw[i] = installJSON{r.Pack, r.Target, r.Path, r.Scope, r.Name}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// loadInstalls lists every recorded install in scope, sorted by pack, target,
// path, scope and name, with project paths made absolute.
func loadInstalls(sys System, scope project.Scope) ([]installRecord, error) {
	var out []installRecord
	if scope.Global {
		path, err := globalInstallsPath(sys)
		if err != nil {
			return nil, err
		}
		if out, err = readGlobalInstalls(path); err != nil {
			return nil, err
		}
	} else {
		packs, err := project.LoadPacks(scope.Root)
		if err != nil {
			return nil, err
		}
		for _, p := range packs {
			for _, in := range p.Installs {
				if in.Path != "" && !filepath.IsAbs(in.Path) {
					in.Path = filepath.Join(scope.Root, in.Path)
				}
				out = append(out, installRecord{p.Name, in})
			}
		}
	}
	sortInstalls(out)
	return out, nil
}

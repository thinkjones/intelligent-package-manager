// Package copilot publishes GitHub Copilot plugins' skills to Claude Code as
// they are: it expands each [[copilot-plugin]] into a plugin entry that
// references the upstream skill folders, reading only what it needs from the
// cache at the locked commit. Nothing is copied or rewritten.
//
// A Copilot plugin (agent-plugins.org plugin.json) lists its skills and
// agents under the com.github.awesome-copilot extension. Its skills become a
// git-subdir plugin over their folder, named for the plugin. Its agents are
// not published: Claude Code loads plugin agents only from an agents/ folder
// inside the plugin, and awesome-copilot keeps them in agents/ at its repo
// root, so referencing them would publish the whole repository. Agents,
// hooks, MCP servers and anything else are reported as skipped.
package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

// namespace is awesome-copilot's extension in plugin.json: where it lists a
// plugin's skills and agents.
const namespace = "com.github.awesome-copilot"

// Result is one item's expansion: the plugins to publish, and the
// components that were not published.
type Result struct {
	Plugins []pack.Plugin
	Skipped []string
}

// Expand turns one Copilot item into plugin entries.
func Expand(root string, item pack.CopilotItem, lock upstream.Lock) (Result, error) {
	dir, err := upstream.Checkout(root, item.Upstream(), lock)
	if err != nil {
		return Result{}, err
	}
	var res Result
	switch item.Kind {
	case pack.CopilotPlugin:
		res, err = plugin(dir, item)
	default:
		err = fmt.Errorf("unknown kind %q", item.Kind)
	}
	if err != nil {
		return Result{}, fmt.Errorf("%s %s/%s: %v", item.Kind, item.Repo, item.Path, err)
	}
	for _, pl := range res.Plugins {
		if !pack.ValidName(pl.Name) || pack.Reserved(pl.Name) {
			return Result{}, fmt.Errorf("%s %s/%s: derived name %q is not a valid plugin name — set `name`", item.Kind, item.Repo, item.Path, pl.Name)
		}
	}
	return res, nil
}

type pluginJSON struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Extensions  map[string]json.RawMessage `json:"extensions"`
}

// readPluginJSON reads the plugin.json of the Copilot plugin at pluginPath:
// beside it, or under .github/plugin/.
func readPluginJSON(dir, pluginPath string) (pluginJSON, bool, error) {
	var pj pluginJSON
	for _, candidate := range []string{"plugin.json", ".github/plugin/plugin.json"} {
		data, err := os.ReadFile(filepath.Join(dir, pluginPath, candidate))
		if err == nil {
			if err := json.Unmarshal(data, &pj); err != nil {
				return pj, true, fmt.Errorf("%s: %v", candidate, err)
			}
			return pj, true, nil
		}
	}
	return pj, false, nil
}

// SkillPaths is the repo-relative folder of every skill the Copilot plugin at
// pluginPath lists in its com.github.awesome-copilot extension.
func SkillPaths(dir, pluginPath string) ([]string, error) {
	pj, found, err := readPluginJSON(dir, pluginPath)
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, errors.New("no plugin.json found")
	}
	var ext struct {
		Skills []string `json:"skills"`
	}
	if raw, ok := pj.Extensions[namespace]; ok {
		if err := json.Unmarshal(raw, &ext); err != nil {
			return nil, fmt.Errorf("extension %s: %v", namespace, err)
		}
	}
	var out []string
	for _, ref := range ext.Skills {
		p, err := resolve(dir, pluginPath, ref, "skills")
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func plugin(dir string, item pack.CopilotItem) (Result, error) {
	pj, found, err := readPluginJSON(dir, item.Path)
	if err != nil {
		return Result{}, err
	}
	if !found {
		if _, err := os.Stat(filepath.Join(dir, item.Path, ".claude-plugin", "plugin.json")); err == nil {
			return Result{}, errors.New("this is a Claude Code plugin — list it as a [[plugin]]")
		}
		return Result{}, errors.New("no plugin.json found")
	}

	var res Result
	components := map[string][]string{}
	for ns, raw := range pj.Extensions {
		if ns != namespace {
			res.Skipped = append(res.Skipped, fmt.Sprintf("extension %s", ns))
			continue
		}
		var all map[string]json.RawMessage
		if err := json.Unmarshal(raw, &all); err != nil {
			return Result{}, fmt.Errorf("extension %s: %v", ns, err)
		}
		for key, v := range all {
			if key != "skills" {
				res.Skipped = append(res.Skipped, key)
				continue
			}
			var paths []string
			if err := json.Unmarshal(v, &paths); err != nil {
				return Result{}, fmt.Errorf("%s: %v", key, err)
			}
			components[key] = paths
		}
	}
	slices.Sort(res.Skipped)

	refs := components["skills"]
	if len(refs) == 0 {
		return Result{}, errors.New("publishes nothing: its plugin.json lists no skills (Copilot agents are not published)")
	}
	parent, skills, err := group(dir, item.Path, refs, "skills")
	if err != nil {
		return Result{}, err
	}
	for _, s := range skills {
		if _, err := os.Stat(filepath.Join(dir, parent, s, "SKILL.md")); err != nil {
			return Result{}, fmt.Errorf("skill %s has no SKILL.md", path.Join(parent, s))
		}
	}
	res.Plugins = []pack.Plugin{{
		Name: cmp(item.Name, pj.Name), Repo: item.Repo, Path: parent, Version: item.Version,
		Skills: skills, Description: cmp(item.Description, pj.Description),
	}}
	return res, nil
}

// group resolves plugin-relative refs to repo paths and returns their common
// parent and each as "./<base>" relative to it. All must share one parent,
// since one git-subdir source fetches one folder.
func group(dir, pluginPath string, refs []string, kind string) (string, []string, error) {
	parent := ""
	var out []string
	for i, ref := range refs {
		p, err := resolve(dir, pluginPath, ref, kind)
		if err != nil {
			return "", nil, err
		}
		if i > 0 && path.Dir(p) != parent {
			return "", nil, fmt.Errorf("%s live in more than one folder (%s, %s)", kind, parent, path.Dir(p))
		}
		parent = path.Dir(p)
		out = append(out, "./"+path.Base(p))
	}
	if parent == "." {
		return "", nil, fmt.Errorf("%s at the repo root are not supported — the whole repo would be published", kind)
	}
	return parent, out, nil
}

// resolve finds a plugin-relative path in the repo: beside plugin.json if it
// is there, else where awesome-copilot keeps it — <kind>/<name> at the repo
// root (its eng/materialize-plugins.mjs).
func resolve(dir, pluginPath, ref, kind string) (string, error) {
	rel := strings.TrimSuffix(strings.TrimPrefix(ref, "./"), "/")
	candidates := []string{path.Join(pluginPath, rel)}
	if strings.HasPrefix(rel, kind+"/") {
		candidates = append(candidates, rel)
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(dir, c)); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s not found upstream (looked in %s)", ref, strings.Join(candidates, ", "))
}

func cmp(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

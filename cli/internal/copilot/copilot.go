// Package copilot reads GitHub Copilot plugins (agent-plugins.org
// plugin.json): which skills a plugin lists in its com.github.awesome-copilot
// extension, resolved to folders in the repo — beside plugin.json, or where
// awesome-copilot keeps them at the repo root.
package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// namespace is awesome-copilot's extension in plugin.json: where it lists a
// plugin's skills and agents.
const namespace = "com.github.awesome-copilot"

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

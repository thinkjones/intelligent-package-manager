package assemble

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
)

// GasFile is one file of a Gas City pack folder.
type GasFile struct {
	Path string // slash-separated, relative to the pack folder
	Data []byte
	Mode fs.FileMode // permission bits
}

type gasPackTOML struct {
	Pack struct {
		Name        string `toml:"name"`
		Schema      int    `toml:"schema"`
		Version     string `toml:"version"`
		Description string `toml:"description,omitempty"`
	} `toml:"pack"`
}

type gasAgentTOML struct {
	Description    string            `toml:"description,omitempty"`
	Provider       string            `toml:"provider"`
	OptionDefaults map[string]string `toml:"option_defaults,omitempty"`
}

type gasMCPTOML struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description,omitempty"`
	Command     string            `toml:"command,omitempty"`
	Args        []string          `toml:"args,omitempty"`
	Env         map[string]string `toml:"env,omitempty"`
	URL         string            `toml:"url,omitempty"`
	Headers     map[string]string `toml:"headers,omitempty"`
}

const pluginRootVar = "${CLAUDE_PLUGIN_ROOT}"

func encodeTOML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderGascity reads every entry from its repo's folder in repos (alias →
// folder, as Place takes) and returns the folder's files sorted by Path, with
// pack.toml first, plus warnings in entry order.
func RenderGascity(gp GascityPlan, repos map[string]string) ([]GasFile, []string, error) {
	var warns []string
	manifest, err := gasPackToml(gp)
	if err != nil {
		return nil, nil, err
	}
	files := []GasFile{{Path: "pack.toml", Data: manifest, Mode: 0o644}}
	for _, e := range gp.Entries {
		src, err := source(repos, Placed{Alias: e.Alias, From: e.From, Item: e.Item})
		if err != nil {
			return nil, nil, err
		}
		var got []GasFile
		var w []string
		switch e.Kind {
		case GasSkill:
			got, err = renderSkill(e, src)
		case GasAgent:
			got, w, err = renderAgent(e, src)
		case GasMCP:
			got, w, err = renderMCP(e, src)
		default:
			err = fmt.Errorf("unknown entry kind %q", e.Kind)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", e.Item.ID, err)
		}
		files = append(files, got...)
		warns = append(warns, w...)
	}
	rest := files[1:]
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].Path < rest[j].Path })
	return files, warns, nil
}

func gasPackToml(gp GascityPlan) ([]byte, error) {
	picked := append(gp.Picked[:0:0], gp.Picked...)
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].ID < picked[j].ID })
	var buf bytes.Buffer
	buf.WriteString(GasHeader + "\n")
	for _, it := range picked {
		sha := it.SHA
		if sha == "" {
			sha = "-"
		}
		fmt.Fprintf(&buf, "# %s @ %s\n", it.ID, sha)
	}
	buf.WriteString("\n")
	var t gasPackTOML
	t.Pack.Name, t.Pack.Schema, t.Pack.Version, t.Pack.Description = gp.Name, 2, "0.0.0-ipm", gp.Description
	body, err := encodeTOML(t)
	if err != nil {
		return nil, err
	}
	buf.Write(body)
	return buf.Bytes(), nil
}

// renderSkill copies every regular file of the skill folder.
func renderSkill(e GasEntry, src string) ([]GasFile, error) {
	var out []GasFile
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out = append(out, GasFile{Path: "skills/" + e.Name + "/" + filepath.ToSlash(rel), Data: data, Mode: info.Mode().Perm()})
		return nil
	})
	return out, err
}

func renderAgent(e GasEntry, src string) ([]GasFile, []string, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, nil, err
	}
	fm, _, err := catalog.Frontmatter(raw)
	if err != nil {
		return renderBadAgent(e, raw)
	}
	desc, _ := fm["description"].(string)
	t := gasAgentTOML{Description: desc, Provider: "claude"}
	if m, _ := fm["model"].(string); m != "" && m != "inherit" {
		t.OptionDefaults = map[string]string{"model": m}
	}
	var dropped []string
	for k := range fm {
		if k != "name" && k != "description" && k != "model" {
			dropped = append(dropped, k)
		}
	}
	sort.Strings(dropped)
	var warns []string
	if len(dropped) > 0 {
		warns = []string{fmt.Sprintf("agents/%s: dropped %s — Gas City agents have no equivalent", e.Name, strings.Join(dropped, ", "))}
	}
	cfg, err := encodeTOML(t)
	if err != nil {
		return nil, nil, err
	}
	dir := "agents/" + e.Name + "/"
	return []GasFile{
		{Path: dir + "agent.toml", Data: cfg, Mode: 0o644},
		{Path: dir + "prompt.md", Data: catalog.Body(raw), Mode: 0o644},
	}, warns, nil
}

func renderMCP(e GasEntry, src string) ([]GasFile, []string, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %v", e.From, err)
	}
	servers := doc
	switch m := doc["mcpServers"].(type) {
	case map[string]any:
		servers = m
	case []any:
		// An array of server maps: merge them, later ones winning.
		servers = map[string]any{}
		for _, el := range m {
			if em, ok := el.(map[string]any); ok {
				for k, v := range em {
					servers[k] = v
				}
			}
		}
	}
	cfg, ok := servers[e.Server].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s has no server %q", e.From, e.Server)
	}
	t := gasMCPTOML{Name: e.Name}
	var dropped []string
	var rooted bool
	check := func(s string) string {
		rooted = rooted || strings.Contains(s, pluginRootVar)
		return s
	}
	var bad error
	str := func(k string) string {
		v, ok := cfg[k].(string)
		if _, present := cfg[k]; present && !ok {
			bad = fmt.Errorf("server %q: %s must be a string", e.Server, k)
		}
		return check(v)
	}
	strMap := func(k string) map[string]string {
		raw, present := cfg[k]
		if !present {
			return nil
		}
		m, ok := raw.(map[string]any)
		if !ok {
			bad = fmt.Errorf("server %q: %s must be a table of strings", e.Server, k)
			return nil
		}
		out := map[string]string{}
		for mk, mv := range m {
			s, ok := mv.(string)
			if !ok {
				bad = fmt.Errorf("server %q: %s.%s must be a string", e.Server, k, mk)
				continue
			}
			out[mk] = check(s)
		}
		return out
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "description":
			t.Description = str(k)
		case "command":
			t.Command = str(k)
		case "url":
			t.URL = str(k)
		case "args":
			list, ok := cfg[k].([]any)
			if !ok {
				bad = fmt.Errorf("server %q: args must be a list of strings", e.Server)
				break
			}
			for _, a := range list {
				s, ok := a.(string)
				if !ok {
					bad = fmt.Errorf("server %q: args must be a list of strings", e.Server)
					break
				}
				t.Args = append(t.Args, check(s))
			}
		case "env":
			t.Env = strMap(k)
		case "headers":
			t.Headers = strMap(k)
		case "type":
		default:
			dropped = append(dropped, k)
		}
	}
	if bad != nil {
		return nil, nil, bad
	}
	if (t.Command == "") == (t.URL == "") {
		return nil, nil, fmt.Errorf("server %q needs exactly one of command or url", e.Server)
	}
	var warns []string
	if len(dropped) > 0 {
		warns = append(warns, fmt.Sprintf("mcp/%s: dropped %s", e.Name, strings.Join(dropped, ", ")))
	}
	if rooted {
		warns = append(warns, fmt.Sprintf("mcp/%s: uses %s, which Gas City does not set", e.Name, pluginRootVar))
	}
	data, err := encodeTOML(t)
	if err != nil {
		return nil, nil, err
	}
	return []GasFile{{Path: "mcp/" + e.Name + ".toml", Data: data, Mode: 0o644}}, warns, nil
}

// renderBadAgent writes an agent whose frontmatter is not valid YAML: the
// body after the fences (the whole file if there are none) as the prompt,
// and a config with only the provider.
func renderBadAgent(e GasEntry, raw []byte) ([]GasFile, []string, error) {
	cfg, err := encodeTOML(gasAgentTOML{Provider: "claude"})
	if err != nil {
		return nil, nil, err
	}
	body := catalog.Body(raw)
	if body == nil {
		body = []byte{}
	}
	dir := "agents/" + e.Name + "/"
	return []GasFile{
			{Path: dir + "agent.toml", Data: cfg, Mode: 0o644},
			{Path: dir + "prompt.md", Data: body, Mode: 0o644},
		}, []string{
			fmt.Sprintf("agents/%s: frontmatter is not valid YAML — wrote the body only", e.Name),
		}, nil
}

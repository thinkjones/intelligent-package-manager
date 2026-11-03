package assemble

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// SkillsManifest is the file in an Agent Skills folder that lists the skill
// folders ipm wrote there, by pack; any other folder is not ipm's.
const SkillsManifest = ".ipm-skills.json"

// SkillEntry is one skill a pack puts in an Agent Skills folder: the folder
// From in the repo installed as Alias, which Item pins to a commit.
type SkillEntry struct {
	Alias, From string
	Item        project.LockItem
}

// SkillsPlan is what a pack becomes in an Agent Skills folder.
type SkillsPlan struct {
	Entries []SkillEntry // in ipm.toml order, a whole plugin's in its parts' order
	Skips   []string     // "skipped <id>: …", in item order
}

// Repos is every repo the skills read: alias → pnpm spec pinned to a
// commit, as Plan.Repos.
func (sp SkillsPlan) Repos() map[string]string {
	out := map[string]string{}
	for _, e := range sp.Entries {
		out[e.Alias] = PinSpec(e.Item)
	}
	return out
}

// PlanAgentSkills plans p's skills: the skills picked, a Copilot plugin's
// skills, and the skills of each whole plugin from the parts the lock
// records. Agents and commands are skipped, as are a whole plugin's other
// parts and an external plugin, whose files ipm doesn't know.
func PlanAgentSkills(p project.Pack, items map[string]project.LockItem) (SkillsPlan, error) {
	var plan SkillsPlan
	placed := map[string]bool{}
	add := func(it project.LockItem) {
		if !placed[it.ID] {
			placed[it.ID] = true
			plan.Entries = append(plan.Entries, SkillEntry{Alias: sources.Alias(it.Repo), From: it.Path, Item: it})
		}
	}
	for _, id := range p.Items {
		it, ok := items[id]
		if !ok {
			return SkillsPlan{}, fmt.Errorf("%s is not resolved", id)
		}
		switch it.Type {
		case catalog.TypeSkill:
			add(it)
		case catalog.TypeAgent, catalog.TypeCommand:
			plan.Skips = append(plan.Skips, fmt.Sprintf("skipped %s: an Agent Skills folder holds only skills", id))
		case catalog.TypeCopilotPlugin:
			skills, err := copilotSkills(it)
			if err != nil {
				return SkillsPlan{}, fmt.Errorf("%s: %w", id, err)
			}
			for _, s := range skills {
				sk, ok := items[s]
				if !ok || sk.Type != catalog.TypeSkill {
					return SkillsPlan{}, fmt.Errorf("%s lists %s, which is not a resolved skill", id, s)
				}
				add(sk)
			}
		case catalog.TypePlugin:
			switch {
			case it.Path == "":
				plan.Skips = append(plan.Skips, fmt.Sprintf("skipped %s: external plugin — its files live in another repo; add its skills directly", id))
				continue
			case it.Parts == nil:
				plan.Skips = append(plan.Skips, fmt.Sprintf(skipNoParts, id))
				continue
			}
			other := 0
			for _, part := range it.Parts {
				if pi := partItem(it, part); pi.Type == catalog.TypeSkill {
					add(pi)
				} else {
					other++
				}
			}
			if other > 0 {
				plan.Skips = append(plan.Skips, fmt.Sprintf("skipped %d agent(s), command(s), hook(s) and MCP server(s) of %s: an Agent Skills folder holds only skills", other, id))
			}
		default:
			return SkillsPlan{}, fmt.Errorf("%s is a %s; only skills, agents, commands and whole plugins can be picked — hooks and MCP servers come with their plugin", id, it.Type)
		}
	}
	return plan, nil
}

// AgentSkill is one skill folder as written: Name is its folder and the name
// in its SKILL.md, as the Agent Skills spec requires.
type AgentSkill struct {
	Name  string
	Item  project.LockItem
	Files []GasFile // relative to the skill folder
}

var specUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// SpecName is s as an Agent Skills name: lowercase letters, digits and single
// hyphens, at most 64 characters, neither starting nor ending with a hyphen.
func SpecName(s string) string {
	s = strings.Trim(specUnsafe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 64 {
		s = strings.TrimRight(s[:64], "-")
	}
	return s
}

var nameLine = regexp.MustCompile(`(?m)^name:[ \t]*.*?([ \t]*\r?)$`)

// RenderAgentSkills reads each planned skill from repos (alias → folder
// holding the repo at its commit) and names it by the name in its SKILL.md,
// made a valid Agent Skills name, rewriting the SKILL.md when that changes
// it. A skill without frontmatter or a usable name is skipped, and so is one
// whose name an earlier skill has; warns says why.
func RenderAgentSkills(pack string, sp SkillsPlan, repos map[string]string) ([]AgentSkill, []string, error) {
	var out []AgentSkill
	var warns []string
	first := map[string]string{} // name → the first item ID with it
	for _, e := range sp.Entries {
		src, err := source(repos, Placed{Alias: e.Alias, From: e.From, Dir: true, Item: e.Item})
		if err != nil {
			return nil, nil, err
		}
		data, err := os.ReadFile(filepath.Join(src, "SKILL.md"))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", e.Item.ID, err)
		}
		fix := MuseFix{Files: map[string][]byte{}}
		fm, ok := fix.frontmatter("SKILL.md", data)
		if !ok {
			warns = append(warns, fmt.Sprintf("skipped %s: its SKILL.md has no frontmatter, which Agent Skills requires", e.Item.ID))
			continue
		}
		if fixed, ok := fix.Files["SKILL.md"]; ok {
			data = fixed
			warns = append(warns, fmt.Sprintf("fixed %s: %s", e.Item.ID, fix.Fixed[0]))
		}
		orig, _ := fm["name"].(string)
		name := SpecName(orig)
		if name == "" {
			name = SpecName(path.Base(e.From))
		}
		if name == "" {
			warns = append(warns, fmt.Sprintf("skipped %s: it has no name usable as an Agent Skills name", e.Item.ID))
			continue
		}
		if prev, dup := first[name]; dup {
			warns = append(warns, pack+": "+DuplicateSkill(pack, e.Item.ID, name, prev))
			continue
		}
		first[name] = e.Item.ID
		if desc, _ := fm["description"].(string); strings.TrimSpace(desc) == "" {
			warns = append(warns, fmt.Sprintf("%s: its SKILL.md has no description, which Agent Skills requires", e.Item.ID))
		}
		if name != orig {
			data = setName(data, name)
			warns = append(warns, fmt.Sprintf("renamed %s to %q: an Agent Skills name is lowercase letters, digits and hyphens, and matches its folder", e.Item.ID, name))
		}
		files, err := skillFiles(src, data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", e.Item.ID, err)
		}
		out = append(out, AgentSkill{Name: name, Item: e.Item, Files: files})
	}
	return out, warns, nil
}

// setName sets the name in skill's frontmatter, adding the line when there
// is none.
func setName(skill []byte, name string) []byte {
	s := string(skill)
	nl := strings.Index(s, "\n") + 1
	end := strings.Index(s[nl:], "\n---")
	if end < 0 {
		end = len(s) - nl
	}
	block := s[nl : nl+end]
	if loc := nameLine.FindStringSubmatchIndex(block); loc != nil {
		block = block[:loc[0]] + "name: " + name + block[loc[2]:]
	} else {
		block = "name: " + name + "\n" + block
	}
	return []byte(s[:nl] + block + s[nl+end:])
}

// skillFiles is every regular file of the skill folder src, with skill as
// its SKILL.md.
func skillFiles(src string, skill []byte) ([]GasFile, error) {
	var out []GasFile
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		data := skill
		if rel != "SKILL.md" {
			if data, err = os.ReadFile(p); err != nil {
				return err
			}
		}
		out = append(out, GasFile{Path: rel, Data: data, Mode: info.Mode().Perm()})
		return nil
	})
	return out, err
}

// skillsManifest is SkillsManifest's content: pack → the skill folders ipm
// wrote for it.
type skillsManifest struct {
	Packs map[string][]string `json:"packs"`
}

func readSkillsManifest(dir string) (skillsManifest, error) {
	m := skillsManifest{Packs: map[string][]string{}}
	data, err := os.ReadFile(filepath.Join(dir, SkillsManifest))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("%s: %v", filepath.Join(dir, SkillsManifest), err)
	}
	if m.Packs == nil {
		m.Packs = map[string][]string{}
	}
	return m, nil
}

func (m skillsManifest) owner(name string) string {
	for pack, names := range m.Packs {
		for _, n := range names {
			if n == name {
				return pack
			}
		}
	}
	return ""
}

// CheckAgentSkills refuses, unless force, a skill folder in dir that holds
// another pack's skill or one ipm didn't write.
func CheckAgentSkills(dir, pack string, skills []AgentSkill, force bool) error {
	m, err := readSkillsManifest(dir)
	if err != nil || force {
		return err
	}
	for _, s := range skills {
		folder := filepath.Join(dir, s.Name)
		if _, err := os.Lstat(folder); err != nil {
			continue
		}
		switch owner := m.owner(s.Name); owner {
		case pack:
		case "":
			return fmt.Errorf("%s exists and was not written by ipm — use --force to replace it", folder)
		default:
			return fmt.Errorf("%s holds pack %s's skill — use --force to replace it", folder, owner)
		}
	}
	return nil
}

// WriteAgentSkills writes each skill as the folder <dir>/<name>, swapping it
// in whole, removes the folders it wrote for pack before that pack no longer
// has, and records what it wrote in dir's SkillsManifest. It refuses as
// CheckAgentSkills does before writing anything. It returns the folders it
// removed.
func WriteAgentSkills(dir, pack string, skills []AgentSkill, force bool) ([]string, error) {
	if err := CheckAgentSkills(dir, pack, skills, force); err != nil {
		return nil, err
	}
	m, err := readSkillsManifest(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(skills))
	keep := map[string]bool{}
	for _, s := range skills {
		if _, err := WriteGascity(s.Files, dir, s.Name, true); err != nil {
			return nil, fmt.Errorf("%s: %v", s.Item.ID, err)
		}
		names = append(names, s.Name)
		keep[s.Name] = true
	}
	var removed []string
	for _, old := range m.Packs[pack] {
		if !keep[old] && filepath.IsLocal(old) {
			if err := os.RemoveAll(filepath.Join(dir, old)); err != nil {
				return nil, err
			}
			removed = append(removed, filepath.Join(dir, old))
		}
	}
	for other, owned := range m.Packs {
		m.Packs[other] = slicesDelete(owned, keep)
		if len(m.Packs[other]) == 0 {
			delete(m.Packs, other)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		m.Packs[pack] = names
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return removed, os.WriteFile(filepath.Join(dir, SkillsManifest), append(data, '\n'), 0o644)
}

// slicesDelete is names without those in drop.
func slicesDelete(names []string, drop map[string]bool) []string {
	var out []string
	for _, n := range names {
		if !drop[n] {
			out = append(out, n)
		}
	}
	return out
}

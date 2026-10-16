// Package pack holds the naming rules shared by everything ipm publishes,
// and reads the [[repo]] list of an aihs-era manifest.toml for
// `ipm marketplace import`. (The old pack flow — root folders of manifest.toml
// [[plugin]] tables — is gone; part 3 of the ipm design brings packs back as
// ipm.toml.)
package pack

import (
	"regexp"
	"strings"
)

// ManifestFile is the aihs-era list of repos of interest.
const ManifestFile = "manifest.toml"

// nameRE is what Claude Code accepts in a plugin or marketplace name.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName reports whether s is usable as a plugin or marketplace name.
func ValidName(s string) bool { return nameRE.MatchString(s) }

var (
	kebabRE  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonKebab = regexp.MustCompile(`[^a-z0-9]+`)
)

// Kebab reports whether s is kebab-case: lowercase letters and digits in
// words joined by single hyphens. claude plugin validate --strict wants
// plugin names so, and ipm pack new refuses any other.
func Kebab(s string) bool { return kebabRE.MatchString(s) }

// KebabOf is s turned kebab-case, or "" when nothing usable is left.
func KebabOf(s string) string {
	return strings.Trim(nonKebab.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// Reserved reports whether Claude Code refuses s as a third-party plugin
// name because it passes as one of Anthropic's own (`claude plugin validate`
// fails it). Case and separators are ignored, as Claude Code does.
func Reserved(s string) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	if len(words) == 0 {
		return false
	}
	joined := strings.Join(words, "-")
	switch joined {
	case "claude", "anthropic", "anthropics", "claude-code", "claude-mods":
		return true
	}
	switch words[0] {
	case "claude", "anthropic", "anthropics":
		return true
	}
	if strings.HasPrefix(joined, "cc-plugin-") {
		return true
	}
	for i := 0; i+1 < len(words); i++ {
		pair := [2]string{words[i], words[i+1]}
		for _, brand := range []string{"claude", "anthropic"} {
			if pair == [2]string{"official", brand} || pair == [2]string{brand, "official"} {
				return true
			}
		}
	}
	return false
}

// Upstream is one repo at one version, as an aihs-era manifest.toml listed
// it (read by `ipm marketplace import`).
type Upstream struct {
	// Repo is a GitHub owner/repo, or a full git URL.
	Repo string
	// Version is a tag, branch or full commit SHA; empty means the default
	// branch.
	Version string
}

// Key is the upstream npm style: repo@version.
func (u Upstream) Key() string {
	if u.Version == "" {
		return u.Repo
	}
	return u.Repo + "@" + u.Version
}

package pack

import "testing"

func TestReserved(t *testing.T) {
	for name, want := range map[string]bool{
		"claude-code-setup":     true,
		"Claude_MD-Management":  true,
		"anthropics-skills":     true,
		"claude":                true,
		"claude-mods":           true,
		"cc-plugin-x":           true,
		"tools-official-claude": true,
		"skill-creator":         false,
		"claudemd-management":   false,
		"skills-anthropics":     false, // only a warning in Claude Code
		"cc-tools":              false,
	} {
		if got := Reserved(name); got != want {
			t.Errorf("Reserved(%q) = %v, want %v", name, got, want)
		}
	}
}

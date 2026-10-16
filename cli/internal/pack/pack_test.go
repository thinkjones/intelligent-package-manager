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

func TestKebab(t *testing.T) {
	for name, want := range map[string]bool{
		"my-pack": true, "kit": true, "a1-b2-3": true,
		"my.pack": false, "My-Pack": false, "my_pack": false, "-kit": false, "kit-": false, "my--pack": false, "": false,
	} {
		if got := Kebab(name); got != want {
			t.Errorf("Kebab(%q) = %v, want %v", name, got, want)
		}
	}
	for in, want := range map[string]string{"my.pack": "my-pack", "My__Pack.": "my-pack", "..": ""} {
		if got := KebabOf(in); got != want {
			t.Errorf("KebabOf(%q) = %q, want %q", in, got, want)
		}
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(env(map[string]string{RootVar: root}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Root != root {
		t.Fatalf("Root = %s, want %s", cfg.Root, root)
	}
}

func TestLoadErrors(t *testing.T) {
	empty := t.TempDir()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"unset names the source", "", "direnv allow"},
		{"relative", "packs", "must be an absolute path"},
		{"missing", filepath.Join(empty, "nope"), "does not exist"},
		{"not a directory", file, "is not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(map[string]string{RootVar: tt.value}))
			if err == nil {
				t.Fatalf("Load(%q): expected an error", tt.value)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load(%q) error %q does not contain %q", tt.value, err, tt.want)
			}
		})
	}
}

package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionFrom(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"a release installed with go install", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}, "0.2.0"},
		{"a commit between releases", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.1-0.20261011101500-b16e7d9c0ffe+dirty"}}, "0.2.1-0.20261011101500-b16e7d9c0ffe+dirty"},
		{"a build of a checkout", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{"no module version", &debug.BuildInfo{}, "dev"},
		{"no build info", nil, "dev"},
	}
	for _, tt := range tests {
		if got := versionFrom(tt.info); got != tt.want {
			t.Errorf("%s: versionFrom = %q, want %q", tt.name, got, tt.want)
		}
	}
}

package github

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const apiJSON = `{"full_name":"o/r","stargazers_count":12,"forks_count":3,"description":"Skills <3","topics":["a"],
	"license":{"spdx_id":"MIT"},"pushed_at":"2026-10-01T00:00:00Z","archived":false,"default_branch":"main"}`

var now = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

func TestFetch(t *testing.T) {
	var got []string
	gh := func(args ...string) (string, error) { got = args; return apiJSON, nil }
	s, err := Fetch(gh, "o/r", now)
	want := Stats{Repo: "o/r", Stars: 12, Forks: 3, Description: "Skills <3", Topics: []string{"a"}, License: "MIT",
		PushedAt: "2026-10-01T00:00:00Z", DefaultBranch: "main", FetchedAt: "2026-10-09T00:00:00Z"}
	if err != nil || !reflect.DeepEqual(s, want) {
		t.Errorf("got %+v %v", s, err)
	}
	if !reflect.DeepEqual(got, []string{"api", "repos/o/r"}) {
		t.Errorf("gh args %v", got)
	}
	if _, err := Fetch(func(...string) (string, error) { return "HTTP 404", errors.New("exit 1") }, "o/r", now); err == nil || !strings.Contains(err.Error(), "o/r") {
		t.Errorf("failure: %v", err)
	}
}

func TestWriteSortedAndReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github.jsonl")
	if m, err := Read(path); err != nil || len(m) != 0 {
		t.Fatalf("missing file: %v %v", m, err)
	}
	in := map[string]Stats{"z/z": {Repo: "z/z", Description: "<b>"}, "a/a": {Repo: "a/a"}}
	if err := Write(path, in); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"archived":false,"default_branch":"","description":"","fetched_at":"","forks":0,"license":"","pushed_at":"","repo":"a/a","stars":0,"topics":[]}`) ||
		!strings.Contains(lines[1], `"<b>"`) {
		t.Errorf("file:\n%s", data)
	}
	back, err := Read(path)
	if err != nil || back["z/z"].Description != "<b>" || len(back) != 2 {
		t.Errorf("read back %v %v", back, err)
	}
}

package assemble

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gasTree(paths ...string) []GasFile {
	files := []GasFile{{Path: "pack.toml", Data: []byte(GasHeader + "\n[pack]\nname = \"kit\"\n"), Mode: 0o644}}
	for _, p := range paths {
		files = append(files, GasFile{Path: p, Data: []byte("x"), Mode: 0o644})
	}
	return files
}

func TestWriteGascityReplaces(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteGascity(gasTree("skills/a/SKILL.md"), dir, "kit", false); err != nil {
		t.Fatal(err)
	}
	got, err := WriteGascity(gasTree("skills/b/SKILL.md"), dir, "kit", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "kit") {
		t.Errorf("path = %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "kit", "skills", "a")); !os.IsNotExist(err) {
		t.Errorf("skills/a should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kit", "skills", "b", "SKILL.md")); err != nil {
		t.Error(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftovers in dir: %v", entries)
	}
}

func TestWriteGascityMode(t *testing.T) {
	dir := t.TempDir()
	files := append(gasTree(), GasFile{Path: "scripts/run.sh", Data: []byte("#!/bin/sh\n"), Mode: 0o755})
	if _, err := WriteGascity(files, dir, "kit", false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "kit", "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func TestWriteGascityRefusesForeign(t *testing.T) {
	dir := t.TempDir()
	pack := filepath.Join(dir, "kit", "pack.toml")
	foreign := "[pack]\nname = \"mine\"\n"
	if err := os.MkdirAll(filepath.Dir(pack), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pack, []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := WriteGascity(gasTree(), dir, "kit", false)
	if !errors.Is(err, ErrNotOurs) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "exists and was not written by ipm — use --force to replace it") {
		t.Errorf("message = %v", err)
	}
	if b, _ := os.ReadFile(pack); string(b) != foreign {
		t.Errorf("file changed: %q", b)
	}
	if _, err := WriteGascity(gasTree(), dir, "kit", true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(pack); !strings.HasPrefix(string(b), GasHeader) {
		t.Errorf("not replaced: %q", b)
	}
}

func TestWriteGascityRefusesRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "kit")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteGascity(gasTree(), dir, "kit", false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "hi" {
		t.Errorf("file changed: %q", b)
	}
	if _, err := WriteGascity(gasTree(), dir, "kit", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "pack.toml")); err != nil {
		t.Error(err)
	}
}

func TestWriteGascityEmptyDirOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "kit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteGascity(gasTree(), dir, "kit", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kit", "pack.toml")); err != nil {
		t.Error(err)
	}
}

func TestWriteGascityStaleTemp(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{".kit.ipm-tmp", ".kit.ipm-old"} {
		if err := os.MkdirAll(filepath.Join(dir, n, "junk"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := WriteGascity(gasTree(), dir, "kit", false); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftovers: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "kit", "junk")); !os.IsNotExist(err) {
		t.Error("stale content leaked into the pack")
	}
}

func TestWriteGascityRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"../evil", "/abs", "a/../../b", ""} {
		files := append(gasTree(), GasFile{Path: p, Data: []byte("x"), Mode: 0o644})
		if _, err := WriteGascity(files, dir, "kit", false); err == nil {
			t.Errorf("path %q accepted", p)
		}
	}
	if _, err := WriteGascity(gasTree(), dir, "../kit", false); err == nil {
		t.Error("bad name accepted")
	}
	if _, err := WriteGascity(gasTree(), filepath.Join(dir, "missing"), "kit", false); err == nil {
		t.Error("missing dir accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("wrote something: %v", entries)
	}
}

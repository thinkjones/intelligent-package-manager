package upstream

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

// realGit runs the git on PATH, as main wires it.
func realGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// remote is a local repo served over file://, standing in for GitHub.
type remote struct {
	t   *testing.T
	dir string
}

func newRemote(t *testing.T) *remote {
	t.Helper()
	r := &remote{t, t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "uploadpack.allowAnySHA1InWant", "true")
	r.commit("one")
	return r
}

func (r *remote) git(args ...string) string {
	r.t.Helper()
	base := []string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", r.dir}
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes VERSION and commits it, returning the new sha.
func (r *remote) commit(version string) string {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, "VERSION"), []byte(version), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", "VERSION")
	r.git("commit", "-q", "-m", version)
	return r.git("rev-parse", "HEAD")
}

func (r *remote) url() string { return "file://" + r.dir }

func cached(t *testing.T, root string, u pack.Upstream, commit string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(Dir(root, u.Repo, commit), "VERSION"))
	if err != nil {
		t.Fatalf("cache for %s: %v", u.Key(), err)
	}
	return string(data)
}

func TestSyncPinsFetchesAndUpgrades(t *testing.T) {
	r := newRemote(t)
	first := r.git("rev-parse", "HEAD")
	root := t.TempDir()
	u := pack.Upstream{Repo: r.url()}

	results, lock := Sync(realGit, root, []pack.Upstream{u}, Lock{}, false)
	if results[0].Err != nil || lock[u.Key()] != first || !results[0].Fetched {
		t.Fatalf("first sync: %+v, lock %v", results[0], lock)
	}
	if v := cached(t, root, u, first); v != "one" {
		t.Fatalf("cache holds %q", v)
	}

	// Upstream moves on; without --upgrade the pin holds and nothing is fetched.
	second := r.commit("two")
	results, lock = Sync(realGit, root, []pack.Upstream{u}, lock, false)
	if results[0].Err != nil || lock[u.Key()] != first || results[0].Fetched {
		t.Fatalf("pinned sync: %+v, lock %v", results[0], lock)
	}

	// --upgrade moves the pin and the cache.
	results, lock = Sync(realGit, root, []pack.Upstream{u}, lock, true)
	if results[0].Err != nil || lock[u.Key()] != second || results[0].Was != first {
		t.Fatalf("upgrade: %+v, lock %v", results[0], lock)
	}
	if v := cached(t, root, u, second); v != "two" {
		t.Fatalf("cache holds %q after upgrade", v)
	}
	if _, err := os.Stat(Dir(root, u.Repo, first)); !os.IsNotExist(err) {
		t.Fatal("the commit no longer locked must be pruned")
	}

	// A locked commit restores the cache to it, e.g. on a fresh checkout.
	if err := os.RemoveAll(CacheRoot(root)); err != nil {
		t.Fatal(err)
	}
	if results, _ = Sync(realGit, root, []pack.Upstream{u}, Lock{u.Key(): first}, false); results[0].Err != nil {
		t.Fatal(results[0].Err)
	}
	if v := cached(t, root, u, first); v != "one" {
		t.Fatalf("cache holds %q, want the locked commit", v)
	}
}

func TestSyncRefsAndShas(t *testing.T) {
	r := newRemote(t)
	first := r.git("rev-parse", "HEAD")
	r.git("tag", "-a", "v1", "-m", "v1") // annotated: ls-remote lists it twice
	r.git("checkout", "-q", "-b", "next")
	next := r.commit("next")
	root := t.TempDir()

	ups := []pack.Upstream{
		{Repo: r.url(), Version: "v1"},
		{Repo: r.url(), Version: "next"},
		{Repo: r.url(), Version: first},
		{Repo: r.url(), Version: "nope"},
	}
	results, lock := Sync(realGit, root, ups, Lock{}, false)
	for i, want := range []string{first, next, first} {
		if results[i].Err != nil || lock[ups[i].Key()] != want {
			t.Errorf("%s: %+v, locked %s, want %s", ups[i].Key(), results[i], lock[ups[i].Key()], want)
		}
	}
	if results[3].Err == nil || !strings.Contains(results[3].Err.Error(), `no version "nope"`) {
		t.Fatalf("a missing ref must fail: %+v", results[3])
	}
	if _, pinned := lock[ups[3].Key()]; pinned {
		t.Fatal("a failed upstream must not be pinned")
	}
}

func TestSyncPrunesUnreferenced(t *testing.T) {
	a, b := newRemote(t), newRemote(t)
	root := t.TempDir()
	ua, ub := pack.Upstream{Repo: a.url()}, pack.Upstream{Repo: b.url()}
	_, lock := Sync(realGit, root, []pack.Upstream{ua, ub}, Lock{}, false)
	gone := Dir(root, ub.Repo, lock[ub.Key()])
	_, lock = Sync(realGit, root, []pack.Upstream{ua}, lock, false)
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatal("a dropped upstream's cache must be removed")
	}
	if _, pinned := lock[ub.Key()]; pinned {
		t.Fatal("a dropped upstream must leave the lock")
	}
}

func TestLockRoundTrip(t *testing.T) {
	root := t.TempDir()
	ups := []pack.Upstream{{Repo: "z/z"}, {Repo: "a/a", Version: "v2"}, {Repo: "unpinned/x"}}
	commit := strings.Repeat("a", 40)
	lock := Lock{"z/z": commit, "a/a@v2": commit}
	if err := WriteLock(root, lock, ups); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, LockFile))
	text := string(data)
	if strings.Index(text, "a/a") > strings.Index(text, "z/z") || strings.Contains(text, "unpinned") || !strings.Contains(text, "version = \"v2\"\ncommit = ") {
		t.Fatalf("lock should be sorted, hold only pins, and record version then commit:\n%s", data)
	}
	got, err := ReadLock(root)
	if err != nil || !reflect.DeepEqual(got, lock) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	if m := got.Missing(ups); len(m) != 1 || m[0].Repo != "unpinned/x" {
		t.Fatalf("missing %v", m)
	}
	if empty, err := ReadLock(t.TempDir()); err != nil || len(empty) != 0 {
		t.Fatalf("no lock file is an empty lock: %v %v", empty, err)
	}
}

func TestCloneURLAndDir(t *testing.T) {
	if got := CloneURL("anthropics/skills"); got != "https://github.com/anthropics/skills.git" {
		t.Error(got)
	}
	if got := CloneURL("https://gitlab.com/a/b.git"); got != "https://gitlab.com/a/b.git" {
		t.Error(got)
	}
	for repo, want := range map[string]string{
		"github/awesome-copilot":               "/r/.aihs/cache/github/awesome-copilot/c0ffee",
		"https://gitlab.com/group/project.git": "/r/.aihs/cache/gitlab.com/group/project/c0ffee",
		"git@github.com:owner/repo.git":        "/r/.aihs/cache/github.com/owner/repo/c0ffee",
		"file:///tmp/x/up":                     "/r/.aihs/cache/tmp/x/up/c0ffee",
	} {
		if got := Dir("/r", repo, "c0ffee"); got != want {
			t.Errorf("Dir(%q) = %q, want %q", repo, got, want)
		}
	}
}

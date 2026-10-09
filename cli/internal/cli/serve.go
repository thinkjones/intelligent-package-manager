package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thinkjones/fs-skills/cli/internal/paths"
	"github.com/thinkjones/fs-skills/cli/internal/search"
	"github.com/thinkjones/fs-skills/cli/internal/typesense"
)

const (
	tsImage     = "typesense/typesense:30.2"
	tsContainer = "ipm-typesense"
	// legacyContainer is the name the search server had while ipm was aihs;
	// it holds port 8108, so serve removes it.
	legacyContainer = "aihs-typesense"
	tsURL           = "http://127.0.0.1:8108"
)

// How often, and for how long, serve waits for Typesense to report healthy.
var (
	healthEvery = 500 * time.Millisecond
	healthFor   = 60 * time.Second
)

// apiKey is the local Typesense key, kept in <cache>/typesense/api-key. With
// create, a missing key is generated: 32 hex characters, mode 0600.
func apiKey(dir string, create bool) (string, error) {
	path := filepath.Join(dir, "api-key")
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !create {
		return "", fmt.Errorf("no search server set up — run `ipm serve`")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := hex.EncodeToString(b)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return key, os.WriteFile(path, []byte(key), 0o600)
}

// tsClient talks to the local Typesense with the stored key.
func tsClient(sys System) (*typesense.Client, error) {
	p, err := ipmPaths(sys)
	if err != nil {
		return nil, err
	}
	key, err := apiKey(p.Typesense(), false)
	if err != nil {
		return nil, err
	}
	return &typesense.Client{BaseURL: tsURL, APIKey: key, HTTP: sys.HTTP}, nil
}

// tsError turns a client error into what the user should do next.
func tsError(err error) error {
	switch {
	case errors.Is(err, typesense.ErrUnreachable):
		return fmt.Errorf("the search server is not running — run `ipm serve`")
	case errors.Is(err, typesense.ErrUnauthorized):
		return fmt.Errorf("the search server rejected the API key — another Typesense may be using port 8108 (check `docker ps`)")
	case errors.Is(err, typesense.ErrTimeout):
		return fmt.Errorf("the search server did not answer in time — it may still be busy (see `docker logs %s`)", tsContainer)
	case errors.Is(err, typesense.ErrNoCollection):
		return fmt.Errorf("the search index is empty — run `ipm load`")
	}
	return err
}

func serveFlags() (*flag.FlagSet, func(System) error) {
	var stop bool
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.BoolVar(&stop, "stop", false, "stop the server (its data is kept)")
	return fs, func(sys System) error { return runServe(sys, stop) }
}

func dockerReady(sys System) error {
	if _, err := sys.LookPath("docker"); err != nil {
		return fmt.Errorf("ipm serve runs Typesense in Docker — install Docker (or OrbStack) first")
	}
	if out, err := sys.Output("", "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("Docker is not running — start it, then rerun: %s", strings.TrimSpace(out))
	}
	return nil
}

// runServe starts (or creates) the Typesense container and waits for it to
// be healthy; with stop, it stops it.
func runServe(sys System, stop bool) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	if err := dockerReady(sys); err != nil {
		return err
	}
	if stop {
		if out, err := sys.Output("", "docker", "stop", tsContainer); err != nil {
			return fmt.Errorf("docker stop: %v: %s", err, strings.TrimSpace(out))
		}
		fmt.Fprintf(sys.Stdout, "%s %s stopped (data kept in %s)\n", ok, tsContainer, p.Typesense())
		return nil
	}
	key, err := apiKey(p.Typesense(), true)
	if err != nil {
		return err
	}
	if out, _ := sys.Output("", "docker", "ps", "-a", "--filter", "name=^"+legacyContainer+"$", "--format", "{{.Names}}"); strings.TrimSpace(out) == legacyContainer {
		fmt.Fprintf(sys.Stdout, "%s removing the old %s container (ipm was called aihs); run `ipm load` after this\n", warn, legacyContainer)
		if out, err := sys.Output("", "docker", "rm", "-f", legacyContainer); err != nil {
			return fmt.Errorf("docker rm %s: %v: %s", legacyContainer, err, strings.TrimSpace(out))
		}
	}
	out, err := sys.Output("", "docker", "inspect", "-f", "{{.State.Running}}", tsContainer)
	was := state(out, err)
	switch was {
	case "running":
	case "stopped":
		if out, err := sys.Output("", "docker", "start", tsContainer); err != nil {
			return fmt.Errorf("docker start: %v: %s", err, strings.TrimSpace(out))
		}
	default:
		if err := dockerRun(sys, p, key); err != nil {
			return err
		}
	}
	c := &typesense.Client{BaseURL: tsURL, APIKey: key, HTTP: sys.HTTP}
	if err := waitHealthy(c); err != nil {
		return err
	}
	n, err := c.NumDocuments(search.Collection)
	if errors.Is(err, typesense.ErrUnauthorized) && was != "missing" {
		// Our container was started with a key we no longer hold (e.g.
		// cache dir was cleaned): recreate it with the current one. Typesense
		// takes the bootstrap key from its command line, so data survives.
		fmt.Fprintf(sys.Stdout, "%s %s holds an old API key — recreating it\n", warn, tsContainer)
		if out, err := sys.Output("", "docker", "rm", "-f", tsContainer); err != nil {
			return fmt.Errorf("docker rm: %v: %s", err, strings.TrimSpace(out))
		}
		if err := dockerRun(sys, p, key); err != nil {
			return err
		}
		if err := waitHealthy(c); err != nil {
			return err
		}
		n, err = c.NumDocuments(search.Collection)
	}
	if err != nil && !errors.Is(err, typesense.ErrNoCollection) {
		return tsError(err)
	}
	fmt.Fprintf(sys.Stdout, "%s Typesense running at %s (container %s)\n", ok, tsURL, tsContainer)
	if n == 0 {
		fmt.Fprintln(sys.Stdout, "Next: ipm load")
	}
	return nil
}

// state reads `docker inspect` of the container: running, stopped or missing.
func state(out string, err error) string {
	switch s := strings.TrimSpace(out); {
	case err == nil && s == "true":
		return "running"
	case err == nil && s == "false":
		return "stopped"
	}
	return "missing"
}

func dockerRun(sys System, p paths.Paths, key string) error {
	data := filepath.Join(p.Typesense(), "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	if out, err := sys.Output("", "docker", "run", "-d", "--name", tsContainer, "-p", "127.0.0.1:8108:8108",
		"-v", data+":/data", tsImage, "--data-dir", "/data", "--api-key", key); err != nil {
		return fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func waitHealthy(c *typesense.Client) error {
	deadline := time.Now().Add(healthFor)
	for {
		err := c.Health()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Typesense did not become healthy within %s (%v) — see `docker logs %s`", healthFor, err, tsContainer)
		}
		time.Sleep(healthEvery)
	}
}

// searchHealth is doctor's docker and typesense rows: [mark, detail]. Both
// are warnings at worst — search is optional.
func searchHealth(sys System) ([2]string, [2]string) {
	if err := dockerReady(sys); err != nil {
		return [2]string{warn, err.Error()}, [2]string{warn, "not running — run `ipm serve`"}
	}
	dockerRow := [2]string{ok, "available"}
	c, err := tsClient(sys)
	if err != nil {
		return dockerRow, [2]string{warn, err.Error()}
	}
	n, err := c.NumDocuments(search.Collection)
	if err != nil {
		return dockerRow, [2]string{warn, tsError(err).Error()}
	}
	if n == 0 {
		return dockerRow, [2]string{warn, "running, but the index is empty — run `ipm load`"}
	}
	return dockerRow, [2]string{ok, fmt.Sprintf("%s, %d document(s)", tsURL, n)}
}

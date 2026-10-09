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

	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/search"
	"github.com/thinkjones/fs-skills/cli/internal/typesense"
)

const (
	tsImage     = "typesense/typesense:30.2"
	tsContainer = "aihs-typesense"
	tsURL       = "http://127.0.0.1:8108"
)

// How often, and for how long, serve waits for Typesense to report healthy.
var (
	healthEvery = 500 * time.Millisecond
	healthFor   = 60 * time.Second
)

func tsDir(root string) string { return filepath.Join(root, ".aihs", "typesense") }

// apiKey is the local Typesense key, kept in .aihs/typesense/api-key. With
// create, a missing key is generated: 32 hex characters, mode 0600.
func apiKey(root string, create bool) (string, error) {
	path := filepath.Join(tsDir(root), "api-key")
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !create {
		return "", fmt.Errorf("no search server set up — run `aihs serve`")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	key := hex.EncodeToString(b)
	if err := os.MkdirAll(tsDir(root), 0o755); err != nil {
		return "", err
	}
	return key, os.WriteFile(path, []byte(key), 0o600)
}

// tsClient talks to the local Typesense with the stored key.
func tsClient(sys System, root string) (*typesense.Client, error) {
	key, err := apiKey(root, false)
	if err != nil {
		return nil, err
	}
	return &typesense.Client{BaseURL: tsURL, APIKey: key, HTTP: sys.HTTP}, nil
}

// tsError turns a client error into what the user should do next.
func tsError(err error) error {
	switch {
	case errors.Is(err, typesense.ErrUnreachable):
		return fmt.Errorf("the search server is not running — run `aihs serve`")
	case errors.Is(err, typesense.ErrUnauthorized):
		return fmt.Errorf("the search server rejected the API key — another Typesense may be using port 8108 (check `docker ps`)")
	case errors.Is(err, typesense.ErrNoCollection):
		return fmt.Errorf("the search index is empty — run `aihs load`")
	}
	return err
}

func serveFlags() (*flag.FlagSet, func(System) error) {
	var stop bool
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.BoolVar(&stop, "stop", false, "stop the server (its data is kept)")
	return fs, func(sys System) error { return runServe(sys, stop) }
}

func dockerReady(sys System, root string) error {
	if _, err := sys.LookPath("docker"); err != nil {
		return fmt.Errorf("aihs serve runs Typesense in Docker — install Docker (or OrbStack) first")
	}
	if out, err := sys.Output(root, "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("Docker is not running — start it, then rerun: %s", strings.TrimSpace(out))
	}
	return nil
}

// runServe starts (or creates) the Typesense container and waits for it to
// be healthy; with stop, it stops it.
func runServe(sys System, stop bool) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	if err := dockerReady(sys, cfg.Root); err != nil {
		return err
	}
	if stop {
		if out, err := sys.Output(cfg.Root, "docker", "stop", tsContainer); err != nil {
			return fmt.Errorf("docker stop: %v: %s", err, strings.TrimSpace(out))
		}
		fmt.Fprintf(sys.Stdout, "%s %s stopped (data kept in %s)\n", ok, tsContainer, tsDir(cfg.Root))
		return nil
	}
	key, err := apiKey(cfg.Root, true)
	if err != nil {
		return err
	}
	out, err := sys.Output(cfg.Root, "docker", "inspect", "-f", "{{.State.Running}}", tsContainer)
	switch state := strings.TrimSpace(out); {
	case err == nil && state == "true":
	case err == nil && state == "false":
		if out, err := sys.Output(cfg.Root, "docker", "start", tsContainer); err != nil {
			return fmt.Errorf("docker start: %v: %s", err, strings.TrimSpace(out))
		}
	default:
		data := filepath.Join(tsDir(cfg.Root), "data")
		if err := os.MkdirAll(data, 0o755); err != nil {
			return err
		}
		if out, err := sys.Output(cfg.Root, "docker", "run", "-d", "--name", tsContainer, "-p", "127.0.0.1:8108:8108",
			"-v", data+":/data", tsImage, "--data-dir", "/data", "--api-key", key); err != nil {
			return fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(out))
		}
	}
	c := &typesense.Client{BaseURL: tsURL, APIKey: key, HTTP: sys.HTTP}
	deadline := time.Now().Add(healthFor)
	for {
		err := c.Health()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Typesense did not become healthy within %s (%v) — see `docker logs %s`", healthFor, err, tsContainer)
		}
		time.Sleep(healthEvery)
	}
	fmt.Fprintf(sys.Stdout, "%s Typesense running at %s (container %s)\n", ok, tsURL, tsContainer)
	if n, err := c.NumDocuments(search.Collection); err != nil || n == 0 {
		fmt.Fprintln(sys.Stdout, "Next: aihs load")
	}
	return nil
}

// searchHealth is doctor's docker and typesense rows: [mark, detail]. Both
// are warnings at worst — search is optional.
func searchHealth(sys System, root string) ([2]string, [2]string) {
	if err := dockerReady(sys, root); err != nil {
		return [2]string{warn, err.Error()}, [2]string{warn, "not running — run `aihs serve`"}
	}
	dockerRow := [2]string{ok, "available"}
	c, err := tsClient(sys, root)
	if err != nil {
		return dockerRow, [2]string{warn, err.Error()}
	}
	n, err := c.NumDocuments(search.Collection)
	if err != nil {
		return dockerRow, [2]string{warn, tsError(err).Error()}
	}
	return dockerRow, [2]string{ok, fmt.Sprintf("%s, %d document(s)", tsURL, n)}
}

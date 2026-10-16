// Package paths says where ipm keeps things, following pnpm: a home for your
// sources and the pnpm store, and a cache for what can be rebuilt — the
// catalog and search data. IPM_HOME and IPM_CACHE_DIR override; otherwise the
// platform's conventions apply (macOS ~/Library, XDG on Linux, LOCALAPPDATA
// on Windows).
package paths

import (
	"fmt"
	"path/filepath"
)

// Paths are ipm's two roots.
type Paths struct {
	// Home holds what you'd lose: sources (package.json, lock), the pin store
	// and pnpm's store.
	Home string
	// Cache holds what ipm can rebuild: the catalog and search data.
	Cache string
}

// Sources is the pnpm project listing your repos of interest.
func (p Paths) Sources() string { return filepath.Join(p.Home, "sources") }

// Store is pnpm's content-addressable store, dedicated to ipm.
func (p Paths) Store() string { return filepath.Join(p.Home, "store") }

// Pinned is the pin store: one pnpm project holding every repo a pack build
// needed at a commit the sources were not at, shared by every project.
func (p Paths) Pinned() string { return filepath.Join(p.Home, "pinned") }

// Catalog is where `ipm catalog rebuild` and `ipm marketplace stats` write.
func (p Paths) Catalog() string { return filepath.Join(p.Cache, "catalog") }

// Typesense holds the search server's data and API key.
func (p Paths) Typesense() string { return filepath.Join(p.Cache, "typesense") }

// Resolve works out the roots from the environment and the OS (runtime.GOOS).
func Resolve(getenv func(string) string, goos string) (Paths, error) {
	home, err := dir(getenv, "IPM_HOME")
	if err != nil {
		return Paths{}, err
	}
	cache, err := dir(getenv, "IPM_CACHE_DIR")
	if err != nil {
		return Paths{}, err
	}
	if cache == "" && getenv("XDG_CACHE_HOME") != "" {
		cache = filepath.Join(getenv("XDG_CACHE_HOME"), "ipm")
	}
	if home != "" && cache != "" {
		return Paths{Home: home, Cache: cache}, nil
	}
	user := getenv("HOME")
	if goos == "windows" {
		user = getenv("USERPROFILE")
	}
	if user == "" {
		return Paths{}, fmt.Errorf("can't find your home directory: set HOME (or IPM_HOME and IPM_CACHE_DIR)")
	}
	var defHome, defCache string
	switch goos {
	case "darwin":
		defHome = filepath.Join(user, "Library", "ipm")
		defCache = filepath.Join(user, "Library", "Caches", "ipm")
	case "windows":
		local := getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(user, "AppData", "Local")
		}
		defHome = filepath.Join(local, "ipm")
		defCache = filepath.Join(local, "ipm", "cache")
	default:
		data := getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(user, ".local", "share")
		}
		defHome = filepath.Join(data, "ipm")
		defCache = filepath.Join(user, ".cache", "ipm")
	}
	if home == "" && getenv("XDG_DATA_HOME") != "" {
		home = filepath.Join(getenv("XDG_DATA_HOME"), "ipm") // as pnpm does, on every OS
	}
	if home == "" {
		home = defHome
	}
	if cache == "" {
		cache = defCache
	}
	return Paths{Home: home, Cache: cache}, nil
}

// dir reads an override, which must be absolute.
func dir(getenv func(string) string, key string) (string, error) {
	v := getenv(key)
	if v != "" && !filepath.IsAbs(v) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", key, v)
	}
	return v, nil
}

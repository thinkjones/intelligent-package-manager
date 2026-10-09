package catalog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Write writes every type's file under dir, records sorted by ID, one per
// line. A type with no records gets an empty file. github.jsonl, written by
// `aihs stats`, is left alone.
func Write(dir string, recs []Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	byType := map[string][]Record{}
	for _, r := range recs {
		if _, ok := Files[r.Type]; !ok {
			return fmt.Errorf("record %s has unknown type %q", r.ID, r.Type)
		}
		byType[r.Type] = append(byType[r.Type], r)
	}
	// Encode everything, then write every temp file, then rename: an error
	// before the renames leaves the previous catalog whole.
	data := map[string][]byte{}
	for _, typ := range Types {
		rs := byType[typ]
		sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
		buf := []byte{}
		for _, r := range rs {
			line, err := r.MarshalJSON()
			if err != nil {
				return fmt.Errorf("%s: %v", r.ID, err)
			}
			buf = append(append(buf, line...), '\n')
		}
		data[typ] = buf
	}
	for _, typ := range Types {
		if err := os.WriteFile(filepath.Join(dir, Files[typ])+".tmp", data[typ], 0o644); err != nil {
			return err
		}
	}
	for _, typ := range Types {
		path := filepath.Join(dir, Files[typ])
		if err := os.Rename(path+".tmp", path); err != nil {
			return err
		}
	}
	return nil
}

// Load reads one type's records as plain JSON objects.
func Load(dir, typ string) ([]map[string]any, error) {
	path := filepath.Join(dir, Files[typ])
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist — run `aihs index`", path)
		}
		return nil, err
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		out = append(out, m)
	}
	return out, sc.Err()
}

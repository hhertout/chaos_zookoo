package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entries maps module kind names to their raw YAML config documents.
type Entries map[string][][]byte

type kindHeader struct {
	Kind string `yaml:"kind"`
}

// LoadEntries reads module configurations from path, which may be a directory
// of YAML files or a single file (optionally containing multiple --- separated documents).
func LoadEntries(path string) (Entries, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("accessing config path %s: %w", path, err)
	}
	if !info.IsDir() {
		return loadFile(path)
	}
	return loadDir(path)
}

func loadFile(path string) (Entries, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	entries := make(Entries)
	if err := appendDocuments(entries, data, path); err != nil {
		return nil, err
	}
	return entries, nil
}

func loadDir(dir string) (Entries, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config dir %s: %w", dir, err)
	}

	entries := make(Entries)
	for _, f := range files {
		if f.IsDir() || !isYAML(f.Name()) {
			continue
		}
		path := filepath.Join(dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name(), err)
		}
		if err := appendDocuments(entries, data, f.Name()); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// appendDocuments splits a raw YAML payload on the `---` document separator
// and appends each non-empty document to entries under its declared kind.
func appendDocuments(entries Entries, data []byte, source string) error {
	for i, doc := range bytes.Split(data, []byte("\n---")) {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		kind, err := extractKind(doc)
		if err != nil {
			return fmt.Errorf("%s (document %d): %w", source, i+1, err)
		}
		entries[kind] = append(entries[kind], doc)
	}
	return nil
}

func extractKind(data []byte) (string, error) {
	var h kindHeader
	if err := yaml.Unmarshal(data, &h); err != nil {
		return "", fmt.Errorf("parsing kind: %w", err)
	}
	if h.Kind == "" {
		return "", fmt.Errorf("missing kind field")
	}
	return h.Kind, nil
}

func isYAML(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

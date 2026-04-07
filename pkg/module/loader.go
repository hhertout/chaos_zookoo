package module

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type kindHeader struct {
	Kind string `yaml:"kind"`
}

type Entries map[string][][]byte

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
	docs := bytes.Split(data, []byte("\n---"))
	for i, doc := range docs {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		var header kindHeader
		if err := yaml.Unmarshal(doc, &header); err != nil {
			return nil, fmt.Errorf("parsing kind from %s (document %d): %w", path, i+1, err)
		}
		if header.Kind == "" {
			return nil, fmt.Errorf("missing kind in %s (document %d)", path, i+1)
		}

		entries[header.Kind] = append(entries[header.Kind], doc)
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
		if f.IsDir() {
			continue
		}
		ext := filepath.Ext(f.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name(), err)
		}

		var header kindHeader
		if err := yaml.Unmarshal(data, &header); err != nil {
			return nil, fmt.Errorf("parsing kind from %s: %w", f.Name(), err)
		}
		if header.Kind == "" {
			return nil, fmt.Errorf("missing kind in %s", f.Name())
		}

		entries[header.Kind] = append(entries[header.Kind], data)
	}

	return entries, nil
}

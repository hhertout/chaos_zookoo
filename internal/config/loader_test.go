package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func TestLoadEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "killing.yaml", "kind: Killing\nnamespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: my-app\n")
	writeFile(t, dir, "rollout.yml", "kind: Rollout\nnamespace: staging\ninterval: 1h\nmatchers:\n  deploymentName: my-deploy\n")
	writeFile(t, dir, "readme.txt", "ignore")

	entries, err := LoadEntries(dir)
	require.NoError(t, err)

	assert.Len(t, entries, 2)
	assert.Len(t, entries["Killing"], 1)
	assert.Len(t, entries["Rollout"], 1)
}

func TestLoadEntries_MultipleOfSameKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "k1.yaml", "kind: Killing\nnamespace: ns1\ninterval: 30s\nmatchers:\n  labels:\n    app: a\n")
	writeFile(t, dir, "k2.yaml", "kind: Killing\nnamespace: ns2\ninterval: 60s\nmatchers:\n  labels:\n    app: b\n")

	entries, err := LoadEntries(dir)
	require.NoError(t, err)
	assert.Len(t, entries["Killing"], 2)
}

func TestLoadEntries_MissingKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.yaml", "interval: 60s\n")

	_, err := LoadEntries(dir)
	assert.Error(t, err)
}

func TestLoadEntries_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "killing.yaml")
	require.NoError(t, os.WriteFile(file, []byte("kind: Killing\nnamespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: my-app\n"), 0o644))

	entries, err := LoadEntries(file)
	require.NoError(t, err)

	assert.Len(t, entries, 1)
	assert.Len(t, entries["Killing"], 1)
}

func TestLoadEntries_SingleFileMultiDocument(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "all.yaml")

	content := "kind: Killing\nnamespace: default\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: my-app\n---\nkind: Rollout\nnamespace: staging\ninterval: 1h\nmatchers:\n  deploymentName: my-deploy\n---\nkind: Killing\nnamespace: other\ninterval: 30s\nmatchers:\n  labels:\n    app: other-app\n"
	require.NoError(t, os.WriteFile(file, []byte(content), 0o644))

	entries, err := LoadEntries(file)
	require.NoError(t, err)

	assert.Len(t, entries, 2)
	assert.Len(t, entries["Killing"], 2)
	assert.Len(t, entries["Rollout"], 1)
}

func TestLoadEntries_DirWithMultiDocumentFile(t *testing.T) {
	dir := t.TempDir()
	content := "kind: Killing\nnamespace: a\nminAvailable: 1\ninterval: 60s\nmatchers:\n  labels:\n    app: a\n---\nkind: Killing\nnamespace: b\nminAvailable: 0\ninterval: 60s\nmatchers:\n  labels:\n    app: b\n"
	writeFile(t, dir, "killing.yaml", content)

	entries, err := LoadEntries(dir)
	require.NoError(t, err)

	assert.Len(t, entries["Killing"], 2, "both documents in the file must be loaded")
}

func TestLoadEntries_InvalidDir(t *testing.T) {
	_, err := LoadEntries("/nonexistent")
	assert.Error(t, err)
}

func TestLoadEntries_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.yaml", "{{invalid")

	_, err := LoadEntries(dir)
	assert.Error(t, err)
}

package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEntries(t *testing.T) {
	dir := t.TempDir()

	killing := `kind: Killing
namespace: default
minAvailable: 1
interval: 60s
matchers:
  labels:
    app: my-app
`
	rollout := `kind: Rollout
namespace: staging
interval: 1h
matchers:
  deploymentName: my-deploy
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "killing.yaml"), []byte(killing), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rollout.yml"), []byte(rollout), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0o644))

	entries, err := LoadEntries(dir)
	require.NoError(t, err)

	assert.Len(t, entries, 2)
	assert.Len(t, entries["Killing"], 1)
	assert.Len(t, entries["Rollout"], 1)
}

func TestLoadEntries_MultipleOfSameKind(t *testing.T) {
	dir := t.TempDir()

	f1 := `kind: Killing
namespace: ns1
interval: 30s
matchers:
  labels:
    app: a
`
	f2 := `kind: Killing
namespace: ns2
interval: 60s
matchers:
  labels:
    app: b
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k1.yaml"), []byte(f1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k2.yaml"), []byte(f2), 0o644))

	entries, err := LoadEntries(dir)
	require.NoError(t, err)
	assert.Len(t, entries["Killing"], 2)
}

func TestLoadEntries_MissingKind(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("interval: 60s\n"), 0o644))

	_, err := LoadEntries(dir)
	assert.Error(t, err)
}

func TestLoadEntries_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "killing.yaml")

	killing := `kind: Killing
namespace: default
minAvailable: 1
interval: 60s
matchers:
  labels:
    app: my-app
`
	require.NoError(t, os.WriteFile(file, []byte(killing), 0o644))

	entries, err := LoadEntries(file)
	require.NoError(t, err)

	assert.Len(t, entries, 1)
	assert.Len(t, entries["Killing"], 1)
}

func TestLoadEntries_SingleFileMultiDocument(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "all.yaml")

	content := `kind: Killing
namespace: default
minAvailable: 1
interval: 60s
matchers:
  labels:
    app: my-app
---
kind: Rollout
namespace: staging
interval: 1h
matchers:
  deploymentName: my-deploy
---
kind: Killing
namespace: other
interval: 30s
matchers:
  labels:
    app: other-app
`
	require.NoError(t, os.WriteFile(file, []byte(content), 0o644))

	entries, err := LoadEntries(file)
	require.NoError(t, err)

	assert.Len(t, entries, 2)
	assert.Len(t, entries["Killing"], 2)
	assert.Len(t, entries["Rollout"], 1)
}

func TestLoadEntries_InvalidDir(t *testing.T) {
	_, err := LoadEntries("/nonexistent")
	assert.Error(t, err)
}

func TestLoadEntries_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{invalid"), 0o644))

	_, err := LoadEntries(dir)
	assert.Error(t, err)
}

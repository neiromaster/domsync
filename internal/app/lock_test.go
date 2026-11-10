package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockPathIsHiddenAndPerConfig(t *testing.T) {
	dir := t.TempDir()
	domsync := filepath.Join(dir, "domsync.yaml")
	if got, want := lockPath(domsync), filepath.Join(dir, ".domsync.lock"); got != want {
		t.Fatalf("lockPath(%q) = %q, want %q", domsync, got, want)
	}
	other := filepath.Join(dir, "work.yaml")
	if got, want := lockPath(other), filepath.Join(dir, ".work.lock"); got != want {
		t.Fatalf("lockPath(%q) = %q, want %q", other, got, want)
	}
	if lockPath(domsync) == lockPath(other) {
		t.Fatal("different configs in one directory share a lock")
	}
}

func TestLockPathHandlesConfigsWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "router")
	if got, want := lockPath(cfg), filepath.Join(dir, ".router.lock"); got != want {
		t.Fatalf("lockPath(%q) = %q, want %q", cfg, got, want)
	}
	hidden := filepath.Join(dir, ".domsync")
	if got, want := lockPath(hidden), filepath.Join(dir, ".domsync.lock"); got != want {
		t.Fatalf("lockPath(%q) = %q, want %q", hidden, got, want)
	}
}

func TestAcquireRunLockCreatesHiddenLockFile(t *testing.T) {
	dir := t.TempDir()
	lock, err := acquireRunLock(lockPath(filepath.Join(dir, "domsync.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.release() }()
	if _, err := os.Stat(filepath.Join(dir, ".domsync.lock")); err != nil {
		t.Fatalf("hidden lock file: %v", err)
	}
}

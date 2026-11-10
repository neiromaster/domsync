package update

import (
	"path/filepath"
	"testing"
)

func TestNewCheckerPlacesCacheUnderCacheDir(t *testing.T) {
	dir, err := CacheDir()
	if err != nil {
		t.Skip("user cache dir unavailable:", err)
	}
	checker := NewChecker()
	if want := filepath.Join(dir, "update.json"); checker.CachePath != want {
		t.Fatalf("CachePath = %q, want %q", checker.CachePath, want)
	}
}

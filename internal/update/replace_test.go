package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exercised by the Windows CI job")
	}
	target := filepath.Join(t.TempDir(), "domsync")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceExecutable(target, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "domsync" {
		t.Fatalf("leftover files in target directory: %v", entries)
	}
}

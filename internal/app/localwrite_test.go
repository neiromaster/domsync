package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteLocalAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.txt")
	if err := writeLocalAtomic(path, []byte("+.a.com\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "+.a.com\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteLocalAtomicFollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalAtomic(link, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	if got, _ := os.ReadFile(target); string(got) != "new\n" {
		t.Fatalf("target = %q", got)
	}
}

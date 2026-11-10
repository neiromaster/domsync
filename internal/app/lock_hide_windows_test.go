package app

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAcquireRunLockHidesTheLockFile(t *testing.T) {
	path := lockPath(filepath.Join(t.TempDir(), "domsync.yaml"))
	lock, err := acquireRunLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.release() }()
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		t.Fatal(err)
	}
	if attrs&windows.FILE_ATTRIBUTE_HIDDEN == 0 {
		t.Fatalf("attributes = %#x, want the hidden bit", attrs)
	}
}

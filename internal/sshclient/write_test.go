package sshclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/testutil/sshtest"
)

func dial(t *testing.T) *Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	c, err := Dial(context.Background(), Options{Host: env.Host, Port: env.Port, User: "u", KeyPath: env.KeyPath, KnownHosts: env.KnownHostsPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestWriteFileAtomicNewFile(t *testing.T) {
	c := dial(t)
	path := filepath.Join(t.TempDir(), "remote.txt")
	res, err := c.WriteFileAtomic(context.Background(), path, []byte("+.a.com\n"), WriteOptions{Mode: 0o644, Backup: true, BackupSuffix: ".bak"})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath != "" {
		t.Fatalf("backup = %q, want none", res.BackupPath)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "+.a.com\n" {
		t.Fatalf("content = %q", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o", fi.Mode().Perm())
	}
}

func TestWriteFileAtomicBackupAndReadBack(t *testing.T) {
	c := dial(t)
	path := filepath.Join(t.TempDir(), "remote.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := c.WriteFileAtomic(context.Background(), path, []byte("new\n"), WriteOptions{Mode: 0o640, Backup: true, BackupSuffix: ".domsync.bak"})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath != path+".domsync.bak" {
		t.Fatalf("backup = %q", res.BackupPath)
	}
	if got, _ := os.ReadFile(path + ".domsync.bak"); string(got) != "old\n" {
		t.Fatalf("backup content = %q", got)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteFileAtomicQuotesContent(t *testing.T) {
	c := dial(t)
	path := filepath.Join(t.TempDir(), "q.txt")
	data := []byte("a'b\"c\n$(touch pwn)\n")
	if _, err := c.WriteFileAtomic(context.Background(), path, data, WriteOptions{Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(data) {
		t.Fatalf("content = %q, want %q", got, data)
	}
}

func TestWriteFileAtomicFailureKeepsOriginal(t *testing.T) {
	c := dial(t)
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	if err := os.MkdirAll(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ro, "remote.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if probe, err := os.Create(filepath.Join(ro, "probe")); err == nil {
		_ = probe.Close()
		t.Skip("directory stayed writable (running as root)")
	}
	if _, err := c.WriteFileAtomic(context.Background(), path, []byte("new\n"), WriteOptions{Mode: 0o644}); err == nil {
		t.Fatal("expected failure")
	}
	if got, _ := os.ReadFile(path); string(got) != "old\n" {
		t.Fatalf("original changed: %q", got)
	}
}

func TestWriteFileAtomicFollowsSymlink(t *testing.T) {
	c := dial(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	res, err := c.WriteFileAtomic(context.Background(), link, []byte("new\n"), WriteOptions{Mode: 0o644, Backup: true, BackupSuffix: ".bak"})
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	if got, _ := os.ReadFile(target); string(got) != "new\n" {
		t.Fatalf("target content = %q", got)
	}
	if res.BackupPath != link+".bak" {
		t.Fatalf("backup = %q", res.BackupPath)
	}
	if got, _ := os.ReadFile(link + ".bak"); string(got) != "old\n" {
		t.Fatalf("backup content = %q", got)
	}
}

func TestWriteFileAtomicBackupFailureKeepsOriginal(t *testing.T) {
	c := dial(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backup := path + ".domsync.bak"
	if err := os.Mkdir(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backup, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backup, 0o755) })
	if probe, err := os.Create(filepath.Join(backup, "probe")); err == nil {
		_ = probe.Close()
		t.Skip("backup directory stayed writable (running as root)")
	}
	if _, err := c.WriteFileAtomic(context.Background(), path, []byte("new\n"), WriteOptions{Mode: 0o644, Backup: true, BackupSuffix: ".domsync.bak"}); err == nil {
		t.Fatal("expected failure")
	}
	if got, _ := os.ReadFile(path); string(got) != "old\n" {
		t.Fatalf("original changed: %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".domsync.") {
			t.Fatalf("temp left behind: %s", e.Name())
		}
	}
}

func TestWriteFileAtomicDirectoryTarget(t *testing.T) {
	c := dial(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "isdir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := c.WriteFileAtomic(context.Background(), target, []byte("new\n"), WriteOptions{Mode: 0o644})
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("error = %v, want directory message", err)
	}
	entries, readErr := os.ReadDir(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, e := range entries {
		t.Fatalf("stray file in target directory: %s", e.Name())
	}
}

func TestWriteFileAtomicNoBackupLeavesCwdAlone(t *testing.T) {
	c := dial(t)
	path := filepath.Join(t.TempDir(), "remote.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	t.Chdir(cwd)
	if _, err := c.WriteFileAtomic(context.Background(), path, []byte("new\n"), WriteOptions{Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Fatalf("content = %q", got)
	}
	if _, err := os.Stat(path + ".domsync.bak"); !os.IsNotExist(err) {
		t.Fatalf("backup exists: %v", err)
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Fatalf("cwd polluted: %s", e.Name())
	}
}

func TestWriteFileAtomicHostilePath(t *testing.T) {
	c := dial(t)
	path := filepath.Join(t.TempDir(), "a b'c$d`e.txt")
	data := []byte("+.a.com\n")
	if _, err := c.WriteFileAtomic(context.Background(), path, data, WriteOptions{Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("content = %q, want %q", got, data)
	}
}

var _ = errors.Is

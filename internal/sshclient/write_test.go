package sshclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

var _ = errors.Is

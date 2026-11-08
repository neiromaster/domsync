package sshclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neiromaster/domsync/internal/testutil/sshtest"
)

func TestReadFileAndRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(path, []byte("+.a.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client, err := Dial(context.Background(), Options{
		Host: env.Host, Port: env.Port, User: "tester",
		KeyPath: env.KeyPath, KnownHosts: env.KnownHostsPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got, err := client.ReadFile(context.Background(), path)
	if err != nil || string(got) != "+.a.com\n" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := client.ReadFile(context.Background(), filepath.Join(dir, "missing")); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing = %v, want ErrNotExist", err)
	}
	res, err := client.Run(context.Background(), "printf hi; printf err >&2; exit 3", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.Stdout != "hi" || !strings.Contains(res.Stderr, "err") {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	client, err := Dial(context.Background(), Options{Host: env.Host, Port: env.Port, User: "u", KeyPath: env.KeyPath, KnownHosts: env.KnownHostsPath})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := client.Run(ctx, "sleep 5", 0); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestUnknownHostKeyRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	empty := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Dial(context.Background(), Options{Host: env.Host, Port: env.Port, User: "u", KeyPath: env.KeyPath, KnownHosts: empty})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "known") {
		t.Fatalf("error = %v, want host key error", err)
	}
}

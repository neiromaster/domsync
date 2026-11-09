package sshclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neiromaster/domsync/internal/testutil/sshtest"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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
	defer func() { _ = client.Close() }()

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
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := client.Run(ctx, "sleep 5", 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
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
	if err == nil {
		t.Fatal("expected host key error")
	}
	presented := ssh.FingerprintSHA256(knownHostKey(t, env.KnownHostsPath))
	if !strings.Contains(err.Error(), "SHA256:") || !strings.Contains(err.Error(), presented) {
		t.Fatalf("error = %v, want presented fingerprint %s", err, presented)
	}
	if !strings.Contains(err.Error(), "ssh-keyscan") {
		t.Fatalf("error = %v, want ssh-keyscan hint", err)
	}
	if !errors.As(err, new(*knownhosts.KeyError)) {
		t.Fatalf("error = %v, want *knownhosts.KeyError in chain", err)
	}
}

func TestChangedHostKeyReportsFingerprints(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	_, wrongPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner, err := ssh.NewSignerFromKey(wrongPrivate)
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{env.Addr}, wrongSigner.PublicKey()) + "\n"
	if err := os.WriteFile(changed, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Dial(context.Background(), Options{Host: env.Host, Port: env.Port, User: "u", KeyPath: env.KeyPath, KnownHosts: changed})
	if err == nil {
		t.Fatal("expected host key error")
	}
	presented := ssh.FingerprintSHA256(knownHostKey(t, env.KnownHostsPath))
	accepted := ssh.FingerprintSHA256(wrongSigner.PublicKey())
	if !strings.Contains(err.Error(), presented) || !strings.Contains(err.Error(), accepted) {
		t.Fatalf("error = %v, want fingerprints %s and %s", err, presented, accepted)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "changed") {
		t.Fatalf("error = %v, want changed-key message", err)
	}
	if !errors.As(err, new(*knownhosts.KeyError)) {
		t.Fatalf("error = %v, want *knownhosts.KeyError in chain", err)
	}
}

func knownHostKey(t *testing.T, path string) ssh.PublicKey {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		t.Fatalf("known_hosts = %q", data)
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(fields[1:], " ")))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

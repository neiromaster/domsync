package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/testutil/sshtest"
)

func TestE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	dir := t.TempDir()
	remoteDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(remoteDir, "domsync.txt")
	if err := os.WriteFile(remote, []byte("+.bitru.org\n+.old.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(local, []byte("[media]\n+.bitru.org\n+.new.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nsource:\n  domains: ./domains.txt\ntarget:\n  host: " + env.Host +
		"\n  port: " + itoa(env.Port) + "\n  user: u\n  ssh_key: ./key\n  known_hosts: ./known_hosts\n  path: " + remote + "\n"
	if err := os.WriteFile(filepath.Join(dir, "domsync.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), mustRead(t, env.KeyPath), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), mustRead(t, env.KnownHostsPath), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"--config", filepath.Join(dir, "domsync.yaml"), "--no-update-check"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	gotRemote, _ := os.ReadFile(remote)
	wantRemote := "# [media]\n+.bitru.org\n+.new.example\n# [other]\n+.old.example\n"
	if string(gotRemote) != wantRemote {
		t.Fatalf("remote = %q, want %q", gotRemote, wantRemote)
	}
	gotLocal, _ := os.ReadFile(local)
	if !strings.Contains(string(gotLocal), "[other]\n+.old.example") {
		t.Fatalf("local = %q", gotLocal)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"--config", filepath.Join(dir, "domsync.yaml"), "--no-update-check"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("second exit = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "изменений нет") {
		t.Fatalf("second run output = %q", out.String())
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

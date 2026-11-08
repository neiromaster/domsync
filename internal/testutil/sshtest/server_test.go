package sshtest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestExecForwardsStdin(t *testing.T) {
	env := Start(t)
	client := dial(t, env)
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	var stdout bytes.Buffer
	session.Stdin = strings.NewReader("forwarded\n")
	session.Stdout = &stdout
	if err := session.Run("cat"); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "forwarded\n" {
		t.Fatalf("stdout = %q, want %q", got, "forwarded\n")
	}
}

func TestExecStdinWritesFile(t *testing.T) {
	env := Start(t)
	client := dial(t, env)
	defer func() { _ = client.Close() }()

	path := filepath.Join(t.TempDir(), "written.txt")
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	session.Stdin = strings.NewReader("+.example.com\n")
	if err := session.Run("cat > " + path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "+.example.com\n" {
		t.Fatalf("file = %q, want %q", data, "+.example.com\n")
	}
}

func dial(t *testing.T, env *Env) *ssh.Client {
	t.Helper()
	keyData, err := os.ReadFile(env.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		t.Fatal(err)
	}
	hostKey, err := knownhosts.New(env.KnownHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ssh.Dial("tcp", env.Addr, &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKey,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

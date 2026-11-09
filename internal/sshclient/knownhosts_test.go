package sshclient

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func mustSigner(t *testing.T, key any) ssh.Signer {
	t.Helper()
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func TestKnownHostAlgorithmsMatchesHostAndSkipsOthers(t *testing.T) {
	_, edPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPrivate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "known_hosts")
	body := knownhosts.Line([]string{"192.168.0.1"}, mustSigner(t, edPrivate).PublicKey()) + "\n" +
		knownhosts.Line([]string{"[192.168.0.1]:2222"}, mustSigner(t, ecPrivate).PublicKey()) + "\n" +
		knownhosts.Line([]string{"other.example"}, mustSigner(t, edPrivate).PublicKey()) + "\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := knownHostAlgorithms(file, "192.168.0.1", 22); !reflect.DeepEqual(got, []string{"ssh-ed25519"}) {
		t.Fatalf("port 22 = %v", got)
	}
	if got := knownHostAlgorithms(file, "192.168.0.1", 2222); !reflect.DeepEqual(got, []string{"ecdsa-sha2-nistp256"}) {
		t.Fatalf("port 2222 = %v", got)
	}
	if got := knownHostAlgorithms(filepath.Join(t.TempDir(), "missing"), "192.168.0.1", 22); got != nil {
		t.Fatalf("missing file = %v", got)
	}
}

func TestKnownHostAlgorithmsHashedPattern(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hashed := knownhosts.HashHostname("192.168.0.1")
	file := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{hashed}, mustSigner(t, private).PublicKey()) + "\n"
	if err := os.WriteFile(file, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := knownHostAlgorithms(file, "192.168.0.1", 22); !reflect.DeepEqual(got, []string{"ssh-ed25519"}) {
		t.Fatalf("algorithms = %v", got)
	}
	if got := knownHostAlgorithms(file, "192.168.0.2", 22); got != nil {
		t.Fatalf("other host = %v", got)
	}
}

func TestHostPatternMatchesWildcardAndNegation(t *testing.T) {
	if !hostPatternMatches("*.example,192.168.0.1", "192.168.0.1") {
		t.Fatal("expected match")
	}
	if !hostPatternMatches("*.example,!bad.example", "good.example") {
		t.Fatal("expected wildcard match")
	}
	if hostPatternMatches("*.example,!bad.example", "bad.example") {
		t.Fatal("negation must reject the host")
	}
	if hostPatternMatches("192.168.0.1", "192.168.0.2") {
		t.Fatal("unexpected match")
	}
}

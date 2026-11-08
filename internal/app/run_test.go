package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "domsync dev (commit none, built unknown)\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--nope"}, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error on stderr")
	}
}

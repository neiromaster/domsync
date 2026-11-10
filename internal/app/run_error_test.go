package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/testutil/sshtest"
)

type runDocument struct {
	OK      bool              `json:"ok"`
	Error   *report.ErrorInfo `json:"error"`
	Servers []struct {
		OK    bool              `json:"ok"`
		Files []runFileDocument `json:"files"`
		Error *report.ErrorInfo `json:"error"`
	} `json:"servers"`
}

type runFileDocument struct {
	OK    bool              `json:"ok"`
	Error *report.ErrorInfo `json:"error"`
}

func writeRunConfig(t *testing.T, env *sshtest.Env, dir, remote, logging string) string {
	t.Helper()
	cfg := "version: 2\nservers:\n  - name: router\n    host: " + env.Host +
		"\n    port: " + strconv.Itoa(env.Port) +
		"\n    user: u\n    ssh_key: ./key\n    known_hosts: ./known_hosts\n" +
		"    files:\n      - source: ./domains.txt\n        target: " + remote + "\n"
	if logging != "" {
		cfg += "logging:\n  file: " + logging + "\n"
	}
	cfgPath := filepath.Join(dir, "domsync.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), mustRead(t, env.KeyPath), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), mustRead(t, env.KnownHostsPath), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func decodeRunDocument(t *testing.T, out []byte) runDocument {
	t.Helper()
	var doc runDocument
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return doc
}

func TestRunJSONOmitsTopLevelErrorForPairFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(remote, []byte("+.a.com # inline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "domains.txt"), []byte("[media]\n+.a.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeRunConfig(t, env, dir, remote, "")

	var out, errOut bytes.Buffer
	code := Run([]string{"--config", cfgPath, "--no-update-check", "--json"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut.String())
	}
	doc := decodeRunDocument(t, out.Bytes())
	if doc.Error != nil {
		t.Fatalf("top-level error = %+v in %s", doc.Error, out.String())
	}
	if doc.OK || len(doc.Servers) != 1 || doc.Servers[0].OK || doc.Servers[0].Error != nil {
		t.Fatalf("servers = %+v in %s", doc.Servers, out.String())
	}
	files := doc.Servers[0].Files
	if len(files) != 1 || files[0].OK || files[0].Error == nil || files[0].Error.Code != 2 {
		t.Fatalf("files = %+v in %s", files, out.String())
	}
}

func TestRunJSONReportsLogFailureAtTopLevel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	env := sshtest.Start(t)
	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.txt")
	if err := os.WriteFile(remote, []byte("+.a.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "domains.txt"), []byte("+.a.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "logs")
	if err := os.Mkdir(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeRunConfig(t, env, dir, remote, logDir)

	var out, errOut bytes.Buffer
	code := Run([]string{"--config", cfgPath, "--no-update-check", "--json"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %q, stdout = %q", code, errOut.String(), out.String())
	}
	doc := decodeRunDocument(t, out.Bytes())
	if doc.Error == nil || doc.Error.Code != 1 || doc.Error.Message == "" {
		t.Fatalf("top-level error = %+v in %s", doc.Error, out.String())
	}
	if len(doc.Servers) != 1 || !doc.Servers[0].OK {
		t.Fatalf("servers = %+v in %s", doc.Servers, out.String())
	}
}

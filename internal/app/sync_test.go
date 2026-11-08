package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/sshclient"
)

type fakeRunner struct {
	files    map[string][]byte
	writes   []string
	commands []string
	writeErr error
	cmdErr   error
}

func (f *fakeRunner) ReadFile(_ context.Context, p string) ([]byte, error) {
	if b, ok := f.files[p]; ok {
		return b, nil
	}
	return nil, sshclient.ErrNotExist
}
func (f *fakeRunner) WriteFileAtomic(_ context.Context, p string, data []byte, _ sshclient.WriteOptions) (sshclient.WriteResult, error) {
	if f.writeErr != nil {
		return sshclient.WriteResult{}, f.writeErr
	}
	f.files[p] = data
	f.writes = append(f.writes, p)
	return sshclient.WriteResult{}, nil
}
func (f *fakeRunner) Run(_ context.Context, cmd string, _ time.Duration) (sshclient.CommandResult, error) {
	f.commands = append(f.commands, cmd)
	return sshclient.CommandResult{ExitCode: 0}, f.cmdErr
}
func (f *fakeRunner) Close() error { return nil }

func setup(t *testing.T, local, remote string) (string, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	kh := filepath.Join(dir, "known_hosts")
	for _, p := range []string{key, kh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "domains.txt"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nsource:\n  domains: ./domains.txt\ntarget:\n  host: h\n  user: u\n  ssh_key: ./key\n  known_hosts: ./known_hosts\n  path: /remote/domains.txt\n  post_write:\n    commands:\n      - /etc/init.d/openclash restart\n"
	if err := os.WriteFile(filepath.Join(dir, "domsync.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{files: map[string][]byte{"/remote/domains.txt": []byte(remote)}}
	return filepath.Join(dir, "domsync.yaml"), f
}

func TestSyncWritesAndReports(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n+.new.com\n", "+.a.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Changed || len(f.writes) != 1 {
		t.Fatalf("report=%+v writes=%v", rep, f.writes)
	}
	if got := string(f.files["/remote/domains.txt"]); got != "# [media]\n+.a.com\n+.new.com\n" {
		t.Fatalf("remote = %q", got)
	}
	if !strings.Contains(rep.Router.Added[0], "+.new.com") {
		t.Fatalf("router added = %v", rep.Router.Added)
	}
}

func TestSyncNoChangesSkipsWriteAndCommand(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	f.files["/remote/domains.txt"] = []byte("# [media]\n+.a.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed || len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("report=%+v writes=%v commands=%v", rep, f.writes, f.commands)
	}
}

func TestSyncDryRunDoesNotWrite(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "")
	if _, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath, DryRun: true}, func(context.Context, config.Config) (Runner, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("writes=%v commands=%v", f.writes, f.commands)
	}
}

func TestSyncStrictEscalatesWarning(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n-+.gone.com\n+.a.com\n", "")
	_, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath, Strict: true}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err == nil || !strings.Contains(err.Error(), "not found") || ExitCode(err) != 2 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
}

func TestSyncWritesLog(t *testing.T) {
	dir := t.TempDir()
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "# [other]\n.b.com\n")
	logPath := filepath.Join(dir, "domsync.log")
	body, _ := os.ReadFile(cfgPath)
	body = append(body, []byte("logging:\n  file: "+logPath+"\n")...)
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"changed=true", "router_added=1", "local_added=1"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("log = %q, want %q", logged, want)
		}
	}
}

func TestSyncCommandFailure(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("exit 1")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("error = %v, report=%+v", err, rep)
	}
}

func TestSyncTransportCommandFailureReported(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("connection lost")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 5 || rep == nil {
		t.Fatalf("error = %v, report=%+v", err, rep)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if !strings.Contains(human.String(), "ошибка: connection lost") || strings.Contains(human.String(), " — ok") {
		t.Fatalf("human = %q", human.String())
	}
	var jsonBuf bytes.Buffer
	if err := report.JSON(&jsonBuf, rep); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Commands []report.Command `json:"commands"`
	}
	if err := json.Unmarshal(jsonBuf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Commands) != 1 || doc.Commands[0].Error != "connection lost" {
		t.Fatalf("commands = %+v in %s", doc.Commands, jsonBuf.String())
	}
}

func TestSyncWriteFailureDoesNotClaimWrite(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.new.com\n", "")
	f.writeErr = errors.New("connection lost")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 3 || rep == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Fatalf("error = %v (code %d), report=%+v", err, ExitCode(err), rep)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if strings.Contains(human.String(), ", записан") {
		t.Fatalf("human claims a write: %q", human.String())
	}
	if !strings.Contains(human.String(), "роутер: +1 −0") {
		t.Fatalf("human = %q", human.String())
	}
}

func TestSyncSuccessRendersWritten(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.new.com\n", "")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if !strings.Contains(human.String(), "роутер: +1 −0, записан") {
		t.Fatalf("human = %q", human.String())
	}
}

func TestSyncCommandFailureStillRendersWritten(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("exit 1")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 5 || rep == nil {
		t.Fatalf("error = %v (code %d), report=%+v", err, ExitCode(err), rep)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if !strings.Contains(human.String(), "роутер: +1 −0, записан") {
		t.Fatalf("human = %q", human.String())
	}
}

func TestSyncLocalOnlyRewriteClaimsLocalWrite(t *testing.T) {
	cfgPath, f := setup(t, "[media]\n+.b.com\n+.a.com\n", "# [media]\n+.a.com\n+.b.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Config) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rep.Router.Changed || rep.Router.Written || !rep.Local.Changed || !rep.Local.Written {
		t.Fatalf("report = %+v", rep)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if !strings.Contains(human.String(), "локально: +0 −0, записан") {
		t.Fatalf("human = %q", human.String())
	}
}

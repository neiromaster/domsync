package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/sshclient"
)

type fakeRunner struct {
	files    map[string][]byte
	readErrs map[string]error
	writes   []string
	commands []string
	writeErr error
	cmdErr   error
}

func (f *fakeRunner) ReadFile(_ context.Context, p string) ([]byte, error) {
	if err, ok := f.readErrs[p]; ok {
		return nil, err
	}
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

type pairSpec struct {
	source string
	local  string
	target string
	remote string
}

type serverSpec struct {
	name     string
	files    []pairSpec
	postRun  string
	disabled bool
}

func dialRunners(t *testing.T, runners map[string]*fakeRunner) func(context.Context, config.Server) (Runner, error) {
	t.Helper()
	return func(_ context.Context, srv config.Server) (Runner, error) {
		runner, ok := runners[srv.Name]
		if !ok {
			return nil, fmt.Errorf("no fake runner for server %q", srv.Name)
		}
		return runner, nil
	}
}

func dialNever(t *testing.T) func(context.Context, config.Server) (Runner, error) {
	t.Helper()
	return func(context.Context, config.Server) (Runner, error) {
		t.Fatal("dial was called for a server that must be skipped")
		return nil, nil
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setupServers(t *testing.T, servers []serverSpec) (string, map[string]*fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "key"), "x")
	writeTestFile(t, filepath.Join(dir, "known_hosts"), "x")
	runners := make(map[string]*fakeRunner, len(servers))
	var body strings.Builder
	body.WriteString("version: 2\nservers:\n")
	for _, server := range servers {
		runner := &fakeRunner{files: map[string][]byte{}, readErrs: map[string]error{}}
		runners[server.name] = runner
		fmt.Fprintf(&body, "  - name: %s\n    host: %s.example\n    user: u\n    ssh_key: ./key\n    known_hosts: ./known_hosts\n", server.name, server.name)
		if server.disabled {
			body.WriteString("    enabled: false\n")
		}
		body.WriteString("    post_write:\n")
		if server.postRun != "" {
			fmt.Fprintf(&body, "      run: %s\n", server.postRun)
		}
		body.WriteString("      commands:\n        - /etc/init.d/openclash restart\n")
		body.WriteString("    files:\n")
		for _, pair := range server.files {
			writeTestFile(t, filepath.Join(dir, pair.source), pair.local)
			fmt.Fprintf(&body, "      - source: ./%s\n        target: %s\n", pair.source, pair.target)
			runner.files[pair.target] = []byte(pair.remote)
		}
	}
	cfgPath := filepath.Join(dir, "domsync.yaml")
	writeTestFile(t, cfgPath, body.String())
	return cfgPath, runners
}

func setupServer(t *testing.T, name string, files []pairSpec) (string, map[string]*fakeRunner) {
	t.Helper()
	return setupServers(t, []serverSpec{{name: name, files: files}})
}

func setupOne(t *testing.T, local, remote string) (string, *fakeRunner) {
	t.Helper()
	cfgPath, runners := setupServer(t, "router", []pairSpec{{source: "domains.txt", local: local, target: "/remote/domains.txt", remote: remote}})
	return cfgPath, runners["router"]
}

func setup(t *testing.T, local, remote string) (string, *fakeRunner) {
	t.Helper()
	return setupOne(t, local, remote)
}

func withLogging(t *testing.T, cfgPath, logPath string) {
	t.Helper()
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("logging:\n  file: "+logPath+"\n")...)
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileResult(t *testing.T, rep *report.Report, server, file int) report.FileResult {
	t.Helper()
	if server >= len(rep.Servers) {
		t.Fatalf("no server %d in %+v", server, rep.Servers)
	}
	if file >= len(rep.Servers[server].Files) {
		t.Fatalf("no file %d on server %d in %+v", file, server, rep.Servers[server])
	}
	return rep.Servers[server].Files[file]
}

func onlyFile(t *testing.T, rep *report.Report) report.FileResult {
	t.Helper()
	if len(rep.Servers) != 1 || len(rep.Servers[0].Files) != 1 {
		t.Fatalf("want one server with one file, got %+v", rep.Servers)
	}
	return rep.Servers[0].Files[0]
}

func TestSyncWritesAndReports(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n+.new.com\n", "+.a.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Changed || len(f.writes) != 1 {
		t.Fatalf("report=%+v writes=%v", rep, f.writes)
	}
	if got := string(f.files["/remote/domains.txt"]); got != "# [media]\n+.a.com\n+.new.com\n" {
		t.Fatalf("remote = %q", got)
	}
	file := onlyFile(t, rep)
	if !strings.Contains(file.Router.Added[0], "+.new.com") {
		t.Fatalf("router added = %v", file.Router.Added)
	}
	if !file.OK || !file.Changed || !rep.Servers[0].OK || rep.Total != 2 {
		t.Fatalf("file = %+v server = %+v", file, rep.Servers[0])
	}
}

func TestSyncNoChangesSkipsWriteAndCommand(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed || len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("report=%+v writes=%v commands=%v", rep, f.writes, f.commands)
	}
}

func TestSyncDryRunDoesNotWrite(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "")
	if _, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath, DryRun: true}, func(context.Context, config.Server) (Runner, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("writes=%v commands=%v", f.writes, f.commands)
	}
}

func TestSyncStrictEscalatesWarning(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n-+.gone.com\n+.a.com\n", "")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath, Strict: true}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err == nil || !strings.Contains(err.Error(), "not found") || ExitCode(err) != 2 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	if file := onlyFile(t, rep); file.OK || file.Error == nil || file.Error.Code != 2 {
		t.Fatalf("file = %+v", file)
	}
	if len(f.writes) != 0 {
		t.Fatalf("strict failure still wrote: %v", f.writes)
	}
}

func TestSyncWritesLog(t *testing.T) {
	dir := t.TempDir()
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "# [other]\n.b.com\n")
	logPath := filepath.Join(dir, "domsync.log")
	withLogging(t, cfgPath, logPath)
	if _, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ok=true", "changed=true", "router_added=1", "local_added=1", "servers=router=ok"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("log = %q, want %q", logged, want)
		}
	}
}

func TestSyncLogsFailuresAndKeepsTheFirstExitCode(t *testing.T) {
	dir := t.TempDir()
	cfgPath, runners := setupServers(t, []serverSpec{
		{name: "dead", files: []pairSpec{{source: "dead.txt", local: "[media]\n+.a.com\n", target: "/dead/domains.txt"}}},
		{name: "bad", files: []pairSpec{{source: "bad.txt", local: "not a valid line\n", target: "/bad/domains.txt"}}},
	})
	logPath := filepath.Join(dir, "domsync.log")
	withLogging(t, cfgPath, logPath)
	dial := func(_ context.Context, srv config.Server) (Runner, error) {
		if srv.Name == "dead" {
			return nil, errors.New("dial tcp: connection refused")
		}
		return runners[srv.Name], nil
	}
	_, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dial)
	if ExitCode(err) != 3 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, want := range []string{"ok=false", "servers=dead=fail,bad=fail"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("log = %q, want %q", logged, want)
		}
	}
}

func TestSyncLogFailureWarnsAlongsidePairFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath, f := setupOne(t, "not a valid line\n", "")
	logDir := filepath.Join(dir, "logs")
	if err := os.Mkdir(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	withLogging(t, cfgPath, logDir)
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	if rep.OK {
		t.Fatalf("report OK = true: %+v", rep)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "не удалось записать журнал") {
		t.Fatalf("warnings = %v", rep.Warnings)
	}
}

func TestSyncCommandFailure(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("exit 1")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("error = %v, report=%+v", err, rep)
	}
	server := rep.Servers[0]
	if server.OK || server.Error != nil {
		t.Fatalf("server = %+v", server)
	}
	if len(server.Commands) != 1 || server.Commands[0].Error == "" {
		t.Fatalf("commands = %+v", server.Commands)
	}
}

func TestSyncTransportCommandFailureReported(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("connection lost")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
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
		Servers []struct {
			Commands []report.Command `json:"commands"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(jsonBuf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Servers) != 1 || len(doc.Servers[0].Commands) != 1 || doc.Servers[0].Commands[0].Error != "connection lost" {
		t.Fatalf("servers = %+v in %s", doc.Servers, jsonBuf.String())
	}
}

func TestSyncWriteFailureDoesNotClaimWrite(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.new.com\n", "")
	f.writeErr = errors.New("connection lost")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
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
	cfgPath, f := setupOne(t, "[media]\n+.new.com\n", "")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
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
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "")
	f.cmdErr = errors.New("exit 1")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err == nil || ExitCode(err) != 5 || rep == nil {
		t.Fatalf("error = %v (code %d), report=%+v", err, ExitCode(err), rep)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if !strings.Contains(human.String(), "роутер: +1 −0, записан") {
		t.Fatalf("human = %q", human.String())
	}
}

func TestSyncCreatesMissingLocalFileFromRouter(t *testing.T) {
	cfgPath, f := setupOne(t, "", "+.b.com\n+.a.com\n")
	localPath := filepath.Join(filepath.Dir(cfgPath), "domains.txt")
	if err := os.Remove(localPath); err != nil {
		t.Fatal(err)
	}
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "+.a.com\n+.b.com\n" {
		t.Fatalf("local = %q", got)
	}
	file := onlyFile(t, rep)
	if !file.Local.Written || !reflect.DeepEqual(file.Local.Added, []string{"+.a.com", "+.b.com"}) || rep.Total != 2 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSyncEmptyLocalAndRouterIsANoop(t *testing.T) {
	cfgPath, f := setupOne(t, "", "")
	localPath := filepath.Join(filepath.Dir(cfgPath), "domains.txt")
	if err := os.Remove(localPath); err != nil {
		t.Fatal(err)
	}
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed || len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("report=%+v writes=%v commands=%v", rep, f.writes, f.commands)
	}
	if _, err := os.Stat(localPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("local file was created in an empty initial state: %v", err)
	}
}

func TestSyncLocalOnlyRewriteDoesNotRenderWritten(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.b.com\n+.a.com\n", "# [media]\n+.a.com\n+.b.com\n")
	rep, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	file := onlyFile(t, rep)
	if file.Router.Changed || file.Router.Written || !file.Local.Changed || !file.Local.Written {
		t.Fatalf("file = %+v", file)
	}
	var human bytes.Buffer
	report.Human(&human, rep)
	if strings.Contains(human.String(), ", записан") {
		t.Fatalf("human claims a write: %q", human.String())
	}
	if !strings.Contains(human.String(), "локально: +0 −0") {
		t.Fatalf("human = %q", human.String())
	}
}

func TestSyncRefusesWhenAnotherRunHoldsTheConfigLock(t *testing.T) {
	cfgPath, _ := setupOne(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	held, err := acquireRunLock(lockPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.release() }()
	_, err = Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, dialNever(t))
	if err == nil || ExitCode(err) != 1 {
		t.Fatalf("code=%d err=%v", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "another domsync run is in progress") {
		t.Fatalf("err=%v", err)
	}
}

func TestSyncReleasesTheLock(t *testing.T) {
	cfgPath, f := setupOne(t, "+.a.com\n", "")
	for i := range 2 {
		if _, err := Sync(context.Background(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil }); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestSyncContinuesAfterOneServerFailsToDial(t *testing.T) {
	cfgPath, runners := setupServers(t, []serverSpec{
		{name: "dead", files: []pairSpec{{source: "dead.txt", local: "[media]\n+.a.com\n", target: "/dead/domains.txt"}}},
		{name: "live", files: []pairSpec{{source: "live.txt", local: "[media]\n+.a.com\n", target: "/live/domains.txt"}}},
	})
	dial := func(_ context.Context, srv config.Server) (Runner, error) {
		if srv.Name == "dead" {
			return nil, errors.New("dial tcp 10.0.0.1:22: connection refused")
		}
		return runners[srv.Name], nil
	}
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dial)
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	if len(rep.Servers) != 2 {
		t.Fatalf("servers = %+v", rep.Servers)
	}
	dead := rep.Servers[0]
	if dead.Name != "dead" || dead.OK || dead.Skipped || len(dead.Files) != 0 || dead.Error == nil || dead.Error.Code != 3 || dead.SkippedPairs != 1 {
		t.Fatalf("dead = %+v", dead)
	}
	live := rep.Servers[1]
	if live.Name != "live" || !live.OK || !live.Changed || len(live.Files) != 1 || len(live.Commands) != 1 {
		t.Fatalf("live = %+v", live)
	}
	if got := string(runners["live"].files["/live/domains.txt"]); got != "# [media]\n+.a.com\n" {
		t.Fatalf("live remote = %q", got)
	}
	if rep.OK || !rep.Changed || rep.Total != 1 || rep.Sections != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSyncContinuesAfterOnePairFails(t *testing.T) {
	cfgPath, runners := setupServer(t, "router", []pairSpec{
		{source: "one.txt", local: "[media]\n+.a.com\n", target: "/srv/one.txt", remote: "+.a.com # inline\n"},
		{source: "two.txt", local: "[media]\n+.b.com\n", target: "/srv/two.txt"},
	})
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dialRunners(t, runners))
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	server := rep.Servers[0]
	if server.OK || len(server.Files) != 2 || server.SkippedPairs != 0 {
		t.Fatalf("server = %+v", server)
	}
	broken := fileResult(t, rep, 0, 0)
	if broken.OK || broken.Error == nil || broken.Error.Code != 2 {
		t.Fatalf("broken = %+v", broken)
	}
	healthy := fileResult(t, rep, 0, 1)
	if !healthy.OK || !healthy.Changed || healthy.Error != nil {
		t.Fatalf("healthy = %+v", healthy)
	}
	if got := string(runners["router"].files["/srv/two.txt"]); got != "# [media]\n+.b.com\n" {
		t.Fatalf("second remote = %q", got)
	}
	if writes := runners["router"].writes; !reflect.DeepEqual(writes, []string{"/srv/two.txt"}) {
		t.Fatalf("writes = %v", writes)
	}
}

func TestSyncStrictFailureIsScopedToThePair(t *testing.T) {
	cfgPath, runners := setupServer(t, "router", []pairSpec{
		{source: "one.txt", local: "[media]\n-+.gone.com\n+.a.com\n", target: "/srv/one.txt"},
		{source: "two.txt", local: "[media]\n+.b.com\n", target: "/srv/two.txt"},
	})
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath, Strict: true}, dialRunners(t, runners))
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	broken := fileResult(t, rep, 0, 0)
	if broken.OK || broken.Error == nil || broken.Error.Code != 2 {
		t.Fatalf("broken = %+v", broken)
	}
	healthy := fileResult(t, rep, 0, 1)
	if !healthy.OK || !healthy.Changed {
		t.Fatalf("healthy = %+v", healthy)
	}
	if got := string(runners["router"].files["/srv/two.txt"]); got != "# [media]\n+.b.com\n" {
		t.Fatalf("second remote = %q", got)
	}
}

func TestSyncDisabledServerIsSkippedWithoutDialing(t *testing.T) {
	cfgPath, _ := setupServers(t, []serverSpec{{
		name:     "parked",
		disabled: true,
		files:    []pairSpec{{source: "parked.txt", local: "[media]\n+.a.com\n", target: "/parked/domains.txt"}},
	}})
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dialNever(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Servers) != 1 {
		t.Fatalf("servers = %+v", rep.Servers)
	}
	server := rep.Servers[0]
	if server.Name != "parked" || !server.OK || !server.Skipped || server.Files == nil || len(server.Files) != 0 || server.Error != nil {
		t.Fatalf("server = %+v", server)
	}
	if rep.Changed || !rep.OK || rep.Total != 0 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSyncPostWriteRunsWhenOnlyLocalChanged(t *testing.T) {
	cfgPath, f := setupOne(t, "[media]\n+.b.com\n+.a.com\n", "# [media]\n+.a.com\n+.b.com\n")
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	file := onlyFile(t, rep)
	if file.Router.Changed || !file.Local.Changed || !file.Changed {
		t.Fatalf("file = %+v", file)
	}
	if len(f.commands) != 1 || len(rep.Servers[0].Commands) != 1 {
		t.Fatalf("commands = %v, report = %+v", f.commands, rep.Servers[0].Commands)
	}
}

func TestSyncPostWriteNeverAndAlways(t *testing.T) {
	t.Run("never", func(t *testing.T) {
		cfgPath, runners := setupServers(t, []serverSpec{{
			name:    "router",
			postRun: "never",
			files:   []pairSpec{{source: "domains.txt", local: "[media]\n+.a.com\n", target: "/remote/domains.txt"}},
		}})
		f := runners["router"]
		rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dialRunners(t, runners))
		if err != nil {
			t.Fatal(err)
		}
		if len(f.writes) != 1 || len(f.commands) != 0 || len(rep.Servers[0].Commands) != 0 {
			t.Fatalf("writes=%v commands=%v report=%+v", f.writes, f.commands, rep.Servers[0].Commands)
		}
	})
	t.Run("always", func(t *testing.T) {
		cfgPath, runners := setupServers(t, []serverSpec{{
			name:    "router",
			postRun: "always",
			files:   []pairSpec{{source: "domains.txt", local: "[media]\n+.a.com\n", target: "/remote/domains.txt", remote: "# [media]\n+.a.com\n"}},
		}})
		f := runners["router"]
		rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dialRunners(t, runners))
		if err != nil {
			t.Fatal(err)
		}
		if rep.Changed || len(f.commands) != 1 || len(rep.Servers[0].Commands) != 1 {
			t.Fatalf("changed=%v commands=%v report=%+v", rep.Changed, f.commands, rep.Servers[0].Commands)
		}
	})
}

func TestSyncDryRunWritesNothingAndRunsNoCommands(t *testing.T) {
	dir := t.TempDir()
	cfgPath, f := setupOne(t, "[media]\n+.a.com\n", "# [other]\n+.b.com\n")
	logPath := filepath.Join(dir, "domsync.log")
	withLogging(t, cfgPath, logPath)
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath, DryRun: true}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	file := onlyFile(t, rep)
	if !rep.DryRun || !rep.Changed || !file.Changed || !file.Router.Changed || !file.Local.Changed {
		t.Fatalf("report = %+v file = %+v", rep, file)
	}
	if !file.Router.Skipped || !file.Local.Skipped || file.Router.Written || file.Local.Written {
		t.Fatalf("deltas = %+v / %+v", file.Router, file.Local)
	}
	if len(f.writes) != 0 || len(f.commands) != 0 {
		t.Fatalf("writes=%v commands=%v", f.writes, f.commands)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry run wrote a log line: %v", err)
	}
}

func TestSyncReportsServerLevelErrorShape(t *testing.T) {
	cfgPath, _ := setupOne(t, "[media]\n+.a.com\n", "")
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, func(context.Context, config.Server) (Runner, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	if ExitCode(err) != 3 || rep == nil {
		t.Fatalf("error = %v (code %d), report = %+v", err, ExitCode(err), rep)
	}
	var buf bytes.Buffer
	if err := report.JSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OK      bool `json:"ok"`
		Servers []struct {
			OK      bool              `json:"ok"`
			Skipped bool              `json:"skipped"`
			Files   []json.RawMessage `json:"files"`
			Error   *report.ErrorInfo `json:"error"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OK || len(doc.Servers) != 1 {
		t.Fatalf("doc = %+v in %s", doc, buf.String())
	}
	server := doc.Servers[0]
	if server.OK || server.Skipped || server.Files == nil || len(server.Files) != 0 {
		t.Fatalf("server = %+v in %s", server, buf.String())
	}
	if server.Error == nil || server.Error.Code != 3 || !strings.Contains(server.Error.Message, "connection refused") {
		t.Fatalf("server error = %+v in %s", server.Error, buf.String())
	}
}

func TestSyncExitCodeIsFirstFailureInOrder(t *testing.T) {
	cfgPath, runners := setupServer(t, "router", []pairSpec{
		{source: "one.txt", local: "[media]\n+.a.com\n", target: "/srv/one.txt", remote: "+.a.com # inline\n"},
		{source: "two.txt", local: "[media]\n+.b.com\n", target: "/srv/two.txt"},
	})
	runners["router"].readErrs["/srv/two.txt"] = errors.New("connection lost")
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath}, dialRunners(t, runners))
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v (code %d)", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("first error lost: %v", err)
	}
	later := fileResult(t, rep, 0, 1)
	if later.OK || later.Error == nil || later.Error.Code != 3 {
		t.Fatalf("later = %+v", later)
	}
}

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/testutil/sshtest"
	"github.com/neiromaster/domsync/internal/update"
	"github.com/neiromaster/domsync/internal/version"
)

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func newTagServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func stubChecker(t *testing.T, srv *httptest.Server) *update.Checker {
	t.Helper()
	return &update.Checker{BaseURL: srv.URL, Repo: "o/r", Client: srv.Client(), CachePath: filepath.Join(t.TempDir(), "c.json")}
}

func TestRunUpdateRefusesBrewAndNPM(t *testing.T) {
	old := detectInstall
	defer func() { detectInstall = old }()
	detectInstall = func(string) update.InstallKind { return update.InstallBrew }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{}}, &out, &errOut)
	if code != 4 || !strings.Contains(errOut.String(), "brew upgrade --cask domsync") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	detectInstall = func(string) update.InstallKind { return update.InstallNPM }
	out.Reset()
	errOut.Reset()
	code = runUpdate(t.Context(), CLI{Update: &UpdateCmd{}}, &out, &errOut)
	if code != 4 || !strings.Contains(errOut.String(), "npm i -g @domsync/domsync@latest") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestRunUpdateRefusalJSONReportsError(t *testing.T) {
	old := detectInstall
	defer func() { detectInstall = old }()
	detectInstall = func(string) update.InstallKind { return update.InstallBrew }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{}, JSON: true}, &out, &errOut)
	if code != 4 || !strings.Contains(errOut.String(), "brew upgrade --cask domsync") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	var doc struct {
		OK    bool              `json:"ok"`
		Error *report.ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OK || doc.Error == nil || doc.Error.Code != 4 || !strings.Contains(doc.Error.Message, "brew upgrade") {
		t.Fatalf("doc = %+v in %s", doc, out.String())
	}
}

func TestRunUpdateCheckPrintsAvailability(t *testing.T) {
	oldDetect, oldNew := detectInstall, newChecker
	defer func() { detectInstall, newChecker = oldDetect, oldNew }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	srv := newTagServer(t, "v1.1.0")
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{Check: true}}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if want := "Текущая версия: v1.0.0\nДоступна v1.1.0\n"; out.String() != want {
		t.Fatalf("out=%q want=%q", out.String(), want)
	}
}

func TestRunUpdateCheckNoUpdate(t *testing.T) {
	oldDetect, oldNew := detectInstall, newChecker
	defer func() { detectInstall, newChecker = oldDetect, oldNew }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.1.0")
	srv := newTagServer(t, "v1.1.0")
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{Check: true}}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if want := "Текущая версия: v1.1.0\nОбновлений нет\n"; out.String() != want {
		t.Fatalf("out=%q want=%q", out.String(), want)
	}
}

func TestRunUpdateCheckError(t *testing.T) {
	oldDetect, oldNew := detectInstall, newChecker
	defer func() { detectInstall, newChecker = oldDetect, oldNew }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{Check: true}}, &out, &errOut)
	if code != 4 || errOut.Len() == 0 || out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunUpdateCheckJSON(t *testing.T) {
	oldDetect, oldNew := detectInstall, newChecker
	defer func() { detectInstall, newChecker = oldDetect, oldNew }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	srv := newTagServer(t, "v1.1.0")
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{Check: true}, JSON: true}, &out, &errOut)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	var doc struct {
		OK     bool               `json:"ok"`
		Update *report.UpdateInfo `json:"update"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.OK || doc.Update == nil || !doc.Update.Available || doc.Update.Latest != "v1.1.0" || doc.Update.Current != "v1.0.0" {
		t.Fatalf("doc = %+v in %s", doc, out.String())
	}
}

func TestRunDispatchesUpdateCommand(t *testing.T) {
	oldDetect, oldNew := detectInstall, newChecker
	defer func() { detectInstall, newChecker = oldDetect, oldNew }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	srv := newTagServer(t, "v1.1.0")
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	var out, errOut bytes.Buffer
	code := Run([]string{"update", "--check"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, errOut.String())
	}
	if want := "Текущая версия: v1.0.0\nДоступна v1.1.0\n"; out.String() != want {
		t.Fatalf("out=%q want=%q", out.String(), want)
	}
}

func TestRunUpdateApplySuccess(t *testing.T) {
	oldDetect, oldApply := detectInstall, applyUpdate
	defer func() { detectInstall, applyUpdate = oldDetect, oldApply }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	var gotOpts update.ApplyOptions
	applyUpdate = func(_ context.Context, opts update.ApplyOptions) (string, error) {
		gotOpts = opts
		return "v1.1.0", nil
	}
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{}}, &out, &errOut)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if want := "domsync обновлён до v1.1.0\n"; out.String() != want {
		t.Fatalf("out=%q want=%q", out.String(), want)
	}
	if gotOpts.BaseURL != updateBaseURL || gotOpts.Repo != updateRepo ||
		gotOpts.Current != "v1.0.0" || gotOpts.GOOS != runtime.GOOS || gotOpts.GOARCH != runtime.GOARCH ||
		gotOpts.Target == "" {
		t.Fatalf("opts = %+v", gotOpts)
	}
}

func TestRunUpdateApplyFailure(t *testing.T) {
	oldDetect, oldApply := detectInstall, applyUpdate
	defer func() { detectInstall, applyUpdate = oldDetect, oldApply }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	applyUpdate = func(context.Context, update.ApplyOptions) (string, error) {
		return "", errors.New("checksum mismatch")
	}
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{}}, &out, &errOut)
	if code != 4 || out.Len() != 0 || !strings.Contains(errOut.String(), "checksum mismatch") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunUpdateApplyJSONSuccess(t *testing.T) {
	oldDetect, oldApply := detectInstall, applyUpdate
	defer func() { detectInstall, applyUpdate = oldDetect, oldApply }()
	detectInstall = func(string) update.InstallKind { return update.InstallStandalone }
	withVersion(t, "v1.0.0")
	applyUpdate = func(context.Context, update.ApplyOptions) (string, error) { return "v1.1.0", nil }
	var out, errOut bytes.Buffer
	code := runUpdate(t.Context(), CLI{Update: &UpdateCmd{}, JSON: true}, &out, &errOut)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	var doc struct {
		OK     bool               `json:"ok"`
		Update *report.UpdateInfo `json:"update"`
		Error  *report.ErrorInfo  `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.OK || doc.Error != nil || doc.Update == nil ||
		doc.Update.Current != "v1.1.0" || doc.Update.Latest != "v1.1.0" || doc.Update.Available {
		t.Fatalf("doc = %+v in %s", doc, out.String())
	}
}

func TestSyncUpdateCheckAttachesUpdateInfo(t *testing.T) {
	withVersion(t, "v1.0.0")
	srv := newTagServer(t, "v1.1.0")
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	rep, err := Sync(t.Context(), SyncOptions{
		ConfigPath:  cfgPath,
		CheckUpdate: true,
		Checker:     stubChecker(t, srv),
	}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rep.Update == nil || !rep.Update.Available || rep.Update.Latest != "v1.1.0" || rep.Update.Current != "v1.0.0" {
		t.Fatalf("update = %+v", rep.Update)
	}
}

func TestSyncUpdateCheckRunsOnDryRun(t *testing.T) {
	withVersion(t, "v1.0.0")
	srv := newTagServer(t, "v1.1.0")
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "")
	rep, err := Sync(t.Context(), SyncOptions{
		ConfigPath:  cfgPath,
		DryRun:      true,
		CheckUpdate: true,
		Checker:     stubChecker(t, srv),
	}, func(context.Context, config.Server) (Runner, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 || rep.Update == nil || !rep.Update.Available {
		t.Fatalf("writes=%v update=%+v", f.writes, rep.Update)
	}
}

func TestSyncUpdateCheckSkippedWhenDisabled(t *testing.T) {
	withVersion(t, "v1.0.0")
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.1.0"})
	}))
	defer srv.Close()
	checker := &update.Checker{BaseURL: srv.URL, Repo: "o/r", Client: srv.Client(), CachePath: filepath.Join(t.TempDir(), "c.json")}
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("update:\n  check: false\n")...)
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	dial := func(context.Context, config.Server) (Runner, error) { return f, nil }
	rep, err := Sync(t.Context(), SyncOptions{ConfigPath: cfgPath, CheckUpdate: true, Checker: checker}, dial)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Update != nil || hits != 0 {
		t.Fatalf("config-disabled: update=%+v hits=%d", rep.Update, hits)
	}
	rep, err = Sync(t.Context(), SyncOptions{ConfigPath: cfgPath, Checker: checker}, dial)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Update != nil || hits != 0 {
		t.Fatalf("flag-disabled: update=%+v hits=%d", rep.Update, hits)
	}
	rep, err = Sync(t.Context(), SyncOptions{ConfigPath: cfgPath, CheckUpdate: true}, dial)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Update != nil || hits != 0 {
		t.Fatalf("nil-checker: update=%+v hits=%d", rep.Update, hits)
	}
}

func TestSyncUpdateCheckErrorWarnsOnlyInVerbose(t *testing.T) {
	withVersion(t, "v1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	cfgPath, f := setup(t, "[media]\n+.a.com\n", "# [media]\n+.a.com\n")
	dial := func(context.Context, config.Server) (Runner, error) { return f, nil }
	rep, err := Sync(t.Context(), SyncOptions{
		ConfigPath:  cfgPath,
		CheckUpdate: true,
		Verbose:     true,
		Checker:     stubChecker(t, srv),
	}, dial)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "500") || rep.Update != nil {
		t.Fatalf("warnings=%v update=%+v", rep.Warnings, rep.Update)
	}
	rep, err = Sync(t.Context(), SyncOptions{
		ConfigPath:  cfgPath,
		CheckUpdate: true,
		Checker:     stubChecker(t, srv),
	}, dial)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 0 || rep.Update != nil {
		t.Fatalf("quiet warnings=%v update=%+v", rep.Warnings, rep.Update)
	}
}

func TestUpdateNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	oldDetect, oldNew := detectInstall, newChecker
	oldVersion := version.Version
	defer func() {
		detectInstall, newChecker = oldDetect, oldNew
		version.Version = oldVersion
	}()
	version.Version = "v1.0.0"
	srv := newTagServer(t, "v1.1.0")
	newChecker = func() *update.Checker { return stubChecker(t, srv) }
	env := sshtest.Start(t)
	dir := t.TempDir()
	remoteDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(remoteDir, "domsync.txt")
	if err := os.WriteFile(remote, []byte("+.a.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(dir, "domains.txt")
	if err := os.WriteFile(local, []byte("[media]\n+.a.com\n+.b.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 2\nservers:\n  - name: router\n    host: " + env.Host +
		"\n    port: " + itoa(env.Port) + "\n    user: u\n    ssh_key: ./key\n    known_hosts: ./known_hosts\n    files:\n      - source: ./domains.txt\n        target: " + remote + "\n"
	if err := os.WriteFile(filepath.Join(dir, "domsync.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), mustRead(t, env.KeyPath), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), mustRead(t, env.KnownHostsPath), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(extra ...string) (string, string, int) {
		var out, errOut bytes.Buffer
		args := append([]string{"--config", filepath.Join(dir, "domsync.yaml")}, extra...)
		code := Run(args, strings.NewReader(""), &out, &errOut)
		return out.String(), errOut.String(), code
	}
	_, errOut, code := run()
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if want := "domsync: доступна v1.1.0 (у вас v1.0.0). Обновление: domsync update\n"; errOut != want {
		t.Fatalf("stderr = %q, want %q", errOut, want)
	}
	_, errOut, code = run("--quiet")
	if code != 0 || strings.Contains(errOut, "доступна") {
		t.Fatalf("quiet: exit=%d stderr=%q", code, errOut)
	}
	out, errOut, code := run("--json")
	if code != 0 || strings.Contains(errOut, "доступна") {
		t.Fatalf("json: exit=%d stderr=%q out=%q", code, errOut, out)
	}
	var doc struct {
		Update *report.UpdateInfo `json:"update"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Update == nil || !doc.Update.Available || doc.Update.Latest != "v1.1.0" {
		t.Fatalf("json update = %+v in %s", doc.Update, out)
	}
}

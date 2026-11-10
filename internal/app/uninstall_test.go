package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/update"
)

type uninstallEnv struct {
	binary string
	cache  string
	calls  []string
}

func newUninstallEnv(t *testing.T, kind update.InstallKind, goos string) *uninstallEnv {
	t.Helper()
	dir := t.TempDir()
	env := &uninstallEnv{
		binary: filepath.Join(dir, "domsync"),
		cache:  filepath.Join(dir, "cache", "domsync"),
	}
	if err := os.WriteFile(env.binary, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.cache, "update.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldResolve, oldDetect, oldCacheDir := resolveExecutable, detectInstall, userCacheDir
	oldRun, oldTerminal, oldGOOS := runExternal, isTerminal, currentGOOS
	t.Cleanup(func() {
		resolveExecutable, detectInstall, userCacheDir = oldResolve, oldDetect, oldCacheDir
		runExternal, isTerminal, currentGOOS = oldRun, oldTerminal, oldGOOS
	})

	resolveExecutable = func() (string, error) { return env.binary, nil }
	detectInstall = func(string) update.InstallKind { return kind }
	userCacheDir = func() (string, error) { return env.cache, nil }
	isTerminal = func(io.Reader) bool { return true }
	currentGOOS = goos
	runExternal = func(_ context.Context, _, _ io.Writer, name string, args ...string) error {
		env.calls = append(env.calls, name+" "+strings.Join(args, " "))
		return nil
	}
	return env
}

func (e *uninstallEnv) exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRunUninstallStandaloneRemovesBinaryAndCache(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if env.exists(env.binary) {
		t.Fatal("binary still exists")
	}
	if env.exists(env.cache) {
		t.Fatal("cache still exists")
	}
	if !strings.Contains(out.String(), env.binary) || !strings.Contains(out.String(), env.cache) {
		t.Fatalf("out=%q", out.String())
	}
}

func TestRunUninstallCancelsOnEnter(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{}}, strings.NewReader("\n"), &out, &errOut)
	if code != 4 {
		t.Fatalf("code=%d", code)
	}
	if !env.exists(env.binary) || !env.exists(env.cache) {
		t.Fatal("cancel removed files")
	}
	if !strings.Contains(errOut.String(), "[y/N]") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestRunUninstallConfirmsOnYes(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{}}, strings.NewReader("y\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if env.exists(env.binary) {
		t.Fatal("binary still exists")
	}
}

func TestRunUninstallRequiresYesWithoutTerminal(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	isTerminal = func(io.Reader) bool { return false }
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{}}, strings.NewReader(""), &out, &errOut)
	if code != 4 {
		t.Fatalf("code=%d", code)
	}
	if !env.exists(env.binary) {
		t.Fatal("binary removed without confirmation")
	}
	if !strings.Contains(errOut.String(), "--yes") {
		t.Fatalf("err=%q", errOut.String())
	}
}

func TestRunUninstallBrewRunsPackageManager(t *testing.T) {
	env := newUninstallEnv(t, update.InstallBrew, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	want := "brew uninstall --cask --zap domsync"
	if len(env.calls) != 1 || env.calls[0] != want {
		t.Fatalf("calls=%v want [%q]", env.calls, want)
	}
	if env.exists(env.cache) {
		t.Fatal("cache still exists")
	}
	if !env.exists(env.binary) {
		t.Fatal("uninstall removed a file owned by brew")
	}
}

func TestRunUninstallNPMRunsPackageManager(t *testing.T) {
	env := newUninstallEnv(t, update.InstallNPM, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	want := "npm uninstall -g @domsync/cli"
	if len(env.calls) != 1 || env.calls[0] != want {
		t.Fatalf("calls=%v want [%q]", env.calls, want)
	}
	if env.exists(env.cache) {
		t.Fatal("cache still exists")
	}
}

func TestRunUninstallNPMOnWindowsPrintsCommand(t *testing.T) {
	env := newUninstallEnv(t, update.InstallNPM, "windows")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 4 {
		t.Fatalf("code=%d", code)
	}
	if len(env.calls) != 0 {
		t.Fatalf("calls=%v, want none", env.calls)
	}
	if !strings.Contains(errOut.String(), "npm uninstall -g @domsync/cli") {
		t.Fatalf("err=%q", errOut.String())
	}
	if env.exists(env.cache) {
		t.Fatal("cache still exists")
	}
}

func TestRunUninstallStopsWhenPackageManagerMissing(t *testing.T) {
	env := newUninstallEnv(t, update.InstallBrew, "darwin")
	runExternal = func(context.Context, io.Writer, io.Writer, string, ...string) error {
		return &exec.Error{Name: "brew", Err: exec.ErrNotFound}
	}
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 4 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "brew uninstall --cask --zap domsync") {
		t.Fatalf("err=%q", errOut.String())
	}
	if env.exists(env.cache) {
		t.Fatal("cache still exists")
	}
}

func TestRunUninstallDryRunKeepsEverything(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{DryRun: true, Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if !env.exists(env.binary) || !env.exists(env.cache) {
		t.Fatal("dry run removed files")
	}
	if !strings.Contains(out.String(), env.binary) {
		t.Fatalf("out=%q", out.String())
	}
}

func TestRunUninstallJSONSendsPackageManagerOutputToStderr(t *testing.T) {
	newUninstallEnv(t, update.InstallBrew, "darwin")
	var forwarded io.Writer
	runExternal = func(_ context.Context, out, _ io.Writer, _ string, _ ...string) error {
		forwarded = out
		return nil
	}
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{JSON: true, Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if forwarded != &errOut {
		t.Fatal("package manager output would corrupt the JSON report")
	}
}

func TestRunUninstallJSONReportsRemoved(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := runUninstall(t.Context(), CLI{JSON: true, Uninstall: &UninstallCmd{Yes: true}}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	var rep report.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("json: %v (%q)", err, out.String())
	}
	if !rep.OK {
		t.Fatalf("ok=false: %+v", rep)
	}
	if len(rep.Removed) != 2 || rep.Removed[0] != env.binary || rep.Removed[1] != env.cache {
		t.Fatalf("removed=%v", rep.Removed)
	}
}

func TestRunDispatchesUninstall(t *testing.T) {
	env := newUninstallEnv(t, update.InstallStandalone, "darwin")
	var out, errOut bytes.Buffer
	code := Run([]string{"uninstall", "--yes"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	if env.exists(env.binary) {
		t.Fatal("binary still exists")
	}
}

package version

import (
	"runtime/debug"
	"testing"
)

func setBuild(t *testing.T, v, commit, date string) {
	t.Helper()
	oldV, oldC, oldD := Version, Commit, Date
	Version, Commit, Date = v, commit, date
	t.Cleanup(func() { Version, Commit, Date = oldV, oldC, oldD })
}

func buildInfo(mainVersion string, settings ...debug.BuildSetting) *debug.BuildInfo {
	return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}, Settings: settings}
}

func TestResolveUsesModuleVersionFromGoInstall(t *testing.T) {
	setBuild(t, "dev", "none", "unknown")
	resolve(buildInfo("v1.1.0",
		debug.BuildSetting{Key: "vcs.revision", Value: "b37de848d19f1c8d29b6ca378fd1b7a5147c311a"},
		debug.BuildSetting{Key: "vcs.time", Value: "2026-11-09T20:11:54Z"},
	), true)
	if Version != "1.1.0" {
		t.Fatalf("Version = %q", Version)
	}
	if Commit != "b37de848d19f1c8d29b6ca378fd1b7a5147c311a" {
		t.Fatalf("Commit = %q", Commit)
	}
	if Date != "2026-11-09T20:11:54Z" {
		t.Fatalf("Date = %q", Date)
	}
}

func TestResolveKeepsLdflagsValues(t *testing.T) {
	setBuild(t, "1.2.3", "deadbeef", "2026-01-02T03:04:05Z")
	resolve(buildInfo("v9.9.9",
		debug.BuildSetting{Key: "vcs.revision", Value: "other"},
		debug.BuildSetting{Key: "vcs.time", Value: "other"},
	), true)
	if Version != "1.2.3" || Commit != "deadbeef" || Date != "2026-01-02T03:04:05Z" {
		t.Fatalf("Version, Commit, Date = %q, %q, %q", Version, Commit, Date)
	}
}

func TestResolveLeavesVersionDevForDevelBuild(t *testing.T) {
	setBuild(t, "dev", "none", "unknown")
	resolve(buildInfo("(devel)",
		debug.BuildSetting{Key: "vcs.revision", Value: "abc123"},
	), true)
	if Version != "dev" {
		t.Fatalf("Version = %q", Version)
	}
	if Commit != "abc123" {
		t.Fatalf("Commit = %q", Commit)
	}
	if Date != "unknown" {
		t.Fatalf("Date = %q", Date)
	}
}

func TestResolveWithoutBuildInfo(t *testing.T) {
	setBuild(t, "dev", "none", "unknown")
	resolve(nil, false)
	resolve(nil, true)
	if Version != "dev" || Commit != "none" || Date != "unknown" {
		t.Fatalf("Version, Commit, Date = %q, %q, %q", Version, Commit, Date)
	}
}

func TestStringHidesUnknownCommitAndDate(t *testing.T) {
	setBuild(t, "1.1.0", "none", "unknown")
	if got := String(); got != "1.1.0" {
		t.Fatalf("String() = %q", got)
	}
}

func TestStringIncludesCommitAndDate(t *testing.T) {
	setBuild(t, "1.1.0", "b37de848", "2026-11-09T20:11:54Z")
	want := "1.1.0 (commit b37de848, built 2026-11-09T20:11:54Z)"
	if got := String(); got != want {
		t.Fatalf("String() = %q", got)
	}
}

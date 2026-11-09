package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	info, ok := debug.ReadBuildInfo()
	resolve(info, ok)
}

func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}

func resolve(info *debug.BuildInfo, ok bool) {
	if !ok || info == nil {
		return
	}
	if Version == "dev" {
		if module := strings.TrimPrefix(info.Main.Version, "v"); module != "" && module != "(devel)" {
			Version = module
		}
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if Commit == "none" {
				Commit = setting.Value
			}
		case "vcs.time":
			if Date == "unknown" {
				Date = setting.Value
			}
		}
	}
}

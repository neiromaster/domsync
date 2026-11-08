package update

import (
	"os"
	"path/filepath"
	"strings"
)

type InstallKind int

const (
	InstallStandalone InstallKind = iota
	InstallBrew
	InstallNPM
)

var brewMarkers = []string{
	"/Caskroom/",
	"/Cellar/",
	"/opt/homebrew/",
	"/usr/local/Cellar/",
	"/home/linuxbrew/.linuxbrew/",
}

func DetectInstall(execPath string) InstallKind {
	p := filepath.ToSlash(execPath)
	for _, marker := range brewMarkers {
		if strings.Contains(p, marker) {
			return InstallBrew
		}
	}
	if strings.Contains(p, "node_modules") {
		return InstallNPM
	}
	return InstallStandalone
}

func ResolveExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func AssetName(version, goos, goarch string) string {
	name := "domsync_" + strings.TrimPrefix(version, "v") + "_" + goos + "_" + goarch
	if goos == "windows" {
		return name + ".zip"
	}
	return name + ".tar.gz"
}

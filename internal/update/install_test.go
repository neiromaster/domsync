package update

import "testing"

func TestDetectInstall(t *testing.T) {
	cases := map[string]InstallKind{
		"/opt/homebrew/Caskroom/domsync/1.0/domsync": InstallBrew,
		"/usr/local/Caskroom/domsync/1.0/domsync":    InstallBrew,
		"/usr/local/Cellar/domsync/1.0/bin/domsync":  InstallBrew,
		"/home/user/node_modules/domsync/bin/x":      InstallNPM,
		"/usr/local/bin/domsync":                     InstallStandalone,
		"/home/u/.local/bin/domsync":                 InstallStandalone,
	}
	for path, want := range cases {
		if got := DetectInstall(path); got != want {
			t.Errorf("DetectInstall(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got, want := AssetName("v1.2.3", "darwin", "arm64"), "domsync_1.2.3_darwin_arm64.tar.gz"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := AssetName("1.2.3", "windows", "amd64"), "domsync_1.2.3_windows_amd64.zip"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

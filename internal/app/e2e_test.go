package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/neiromaster/domsync/internal/testutil/sshtest"
)

func TestE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test SSH server executes sh")
	}
	router := sshtest.Start(t)
	vps := sshtest.Start(t)

	dir := t.TempDir()
	remoteDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	routerRemote := filepath.Join(remoteDir, "router.txt")
	vpsRemote := filepath.Join(remoteDir, "vps.txt")
	writeTestFile(t, routerRemote, "+.router-remote.example\n")
	writeTestFile(t, vpsRemote, "+.vps-remote.example\n")

	routerLocal := filepath.Join(dir, "router.txt")
	vpsLocal := filepath.Join(dir, "vps.txt")
	writeTestFile(t, routerLocal, "[media]\n+.router-local.example\n+.shared.example\n")
	writeTestFile(t, vpsLocal, "[ai]\n+.vps-local.example\n")

	writeTestFile(t, filepath.Join(dir, "router-key"), string(mustRead(t, router.KeyPath)))
	writeTestFile(t, filepath.Join(dir, "router-known-hosts"), string(mustRead(t, router.KnownHostsPath)))
	writeTestFile(t, filepath.Join(dir, "vps-key"), string(mustRead(t, vps.KeyPath)))
	writeTestFile(t, filepath.Join(dir, "vps-known-hosts"), string(mustRead(t, vps.KnownHostsPath)))

	cfgPath := filepath.Join(dir, "domsync.yaml")
	cfg := fmt.Sprintf(`version: 2
servers:
  - name: router
    host: %s
    port: %d
    user: u
    ssh_key: ./router-key
    known_hosts: ./router-known-hosts
    files:
      - source: ./router.txt
        target: %s
  - name: vps-1
    enabled: true
    host: %s
    port: %d
    user: u
    ssh_key: ./vps-key
    known_hosts: ./vps-known-hosts
    files:
      - source: ./vps.txt
        target: %s
`, router.Host, router.Port, routerRemote, vps.Host, vps.Port, vpsRemote)
	writeTestFile(t, cfgPath, cfg)

	args := []string{"--config", cfgPath, "--no-update-check"}
	var out, errOut bytes.Buffer
	if code := Run(args, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q", errOut.String())
	}
	for _, want := range []string{"router (" + router.Host + ")", "vps-1 (" + vps.Host + ")", "записан"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output = %q, want %q", out.String(), want)
		}
	}

	if got, want := string(mustRead(t, routerRemote)), "# [media]\n+.router-local.example\n+.shared.example\n# [other]\n+.router-remote.example\n"; got != want {
		t.Fatalf("router remote = %q, want %q", got, want)
	}
	if got, want := string(mustRead(t, vpsRemote)), "# [ai]\n+.vps-local.example\n# [other]\n+.vps-remote.example\n"; got != want {
		t.Fatalf("vps remote = %q, want %q", got, want)
	}
	if got := string(mustRead(t, routerLocal)); !strings.Contains(got, "[other]\n+.router-remote.example") {
		t.Fatalf("router local = %q", got)
	}
	if got := string(mustRead(t, vpsLocal)); !strings.Contains(got, "[other]\n+.vps-remote.example") {
		t.Fatalf("vps local = %q", got)
	}

	out.Reset()
	errOut.Reset()
	if code := Run(args, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("second exit = %d, stderr = %s", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("second stderr = %q", errOut.String())
	}
	wantOut := fmt.Sprintf("router (%s)\n  %s → %s: без изменений\nvps-1 (%s)\n  %s → %s: без изменений\ndomsync: изменений нет (5 доменов, 4 секции)\n",
		router.Host, routerLocal, routerRemote, vps.Host, vpsLocal, vpsRemote)
	if out.String() != wantOut {
		t.Fatalf("second run output = %q, want %q", out.String(), wantOut)
	}
	if got, want := string(mustRead(t, routerRemote)), "# [media]\n+.router-local.example\n+.shared.example\n# [other]\n+.router-remote.example\n"; got != want {
		t.Fatalf("router remote after second run = %q, want %q", got, want)
	}
	if got, want := string(mustRead(t, vpsRemote)), "# [ai]\n+.vps-local.example\n# [other]\n+.vps-remote.example\n"; got != want {
		t.Fatalf("vps remote after second run = %q, want %q", got, want)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

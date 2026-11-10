package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"key", "known_hosts", "domains.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "domsync.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadServerAndPairDefaults(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /etc/openclash/rule_provider/media.txt
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.Servers[0]
	if !srv.IsEnabled() || srv.Port != 22 || srv.PostWrite.Run != "on_change" {
		t.Fatalf("server defaults = %+v", srv)
	}
	if got := time.Duration(srv.PostWrite.Timeout); got != 120*time.Second {
		t.Fatalf("timeout = %s", got)
	}
	pair := srv.Files[0]
	if pair.OtherSection != "other" || pair.Mode != "0644" || pair.BackupSuffix != ".domsync.bak" {
		t.Fatalf("pair defaults = %+v", pair)
	}
	if !pair.BackupEnabled() || pair.FileMode() != 0o644 {
		t.Fatalf("pair helpers = %v %v", pair.BackupEnabled(), pair.FileMode())
	}
	if pair.Source != filepath.Join(filepath.Dir(path), "domains.txt") {
		t.Fatalf("source = %q", pair.Source)
	}
	if srv.KnownHosts != filepath.Join(filepath.Dir(path), "known_hosts") {
		t.Fatalf("known_hosts = %q", srv.KnownHosts)
	}
}

func TestLoadMultipleServersKeepOrder(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: a
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
  - name: vps-1
    enabled: false
    host: b
    user: u
    ssh_key: ./missing-key
    files:
      - source: ./other.txt
        target: /b.txt
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 2 || cfg.Servers[0].Name != "router" || cfg.Servers[1].Name != "vps-1" {
		t.Fatalf("servers = %+v", cfg.Servers)
	}
	if cfg.Servers[1].IsEnabled() {
		t.Fatal("vps-1 should be disabled")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servers[1].KnownHosts != filepath.Join(home, ".ssh", "known_hosts") {
		t.Fatalf("known_hosts default = %q", cfg.Servers[1].KnownHosts)
	}
}

func TestLoadRejectsVersionOne(t *testing.T) {
	path := writeConfig(t, `version: 1
source:
  domains: ./domains.txt
target:
  host: 192.168.0.1
  path: /etc/openclash/rule_provider/pure_list.txt
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "version must be 2") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "domsync.example.yaml") {
		t.Fatalf("err = %v, want a pointer to domsync.example.yaml", err)
	}
}

func TestLoadRejectsFutureVersionWithUnknownFields(t *testing.T) {
	path := writeConfig(t, `version: 3
servers: []
retries: 5
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "version must be 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsMissingVersion(t *testing.T) {
	path := writeConfig(t, "servers: []\n")
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "version is missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
nonsense: true
`)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDuration(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    post_write:
      timeout: 90s
    files:
      - source: ./domains.txt
        target: /a.txt
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(cfg.Servers[0].PostWrite.Timeout); got != 90*time.Second {
		t.Fatalf("timeout = %v", got)
	}
}

func TestLoadDurationErrorNamesLineAndValue(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    post_write:
      timeout: abc
    files:
      - source: ./domains.txt
        target: /a.txt
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected a duration error")
	}
	message := err.Error()
	if !strings.Contains(message, "line 9") {
		t.Fatalf("error does not name the line: %v", err)
	}
	if !strings.Contains(message, `"abc"`) {
		t.Fatalf("error does not name the value: %v", err)
	}
}

func TestLoadShapeValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no servers",
			body: "version: 2\n",
			want: "servers must not be empty",
		},
		{
			name: "missing name",
			body: `version: 2
servers:
  - host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0].name must not be empty",
		},
		{
			name: "invalid name",
			body: `version: 2
servers:
  - name: Router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: `servers[0] (Router).name must match [a-z0-9][a-z0-9_-]*`,
		},
		{
			name: "name too long",
			body: `version: 2
servers:
  - name: ` + strings.Repeat("a", 33) + `
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (" + strings.Repeat("a", 33) + ").name must be at most 32 characters",
		},
		{
			name: "duplicate name",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./key
        target: /b.txt
`,
			want: `servers[1] (router).name "router" is already used by servers[0] (router)`,
		},
		{
			name: "missing host",
			body: `version: 2
servers:
  - name: router
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (router).host must not be empty",
		},
		{
			name: "missing user",
			body: `version: 2
servers:
  - name: router
    host: h
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (router).user must not be empty",
		},
		{
			name: "port out of range",
			body: `version: 2
servers:
  - name: router
    host: h
    port: 70000
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (router).port must be between 1 and 65535, got 70000",
		},
		{
			name: "invalid post_write.run",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    post_write:
      run: sometimes
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (router).post_write.run must be one of on_change, always, never; got \"sometimes\"",
		},
		{
			name: "negative timeout",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    post_write:
      timeout: -1s
    files:
      - source: ./domains.txt
        target: /a.txt
`,
			want: "servers[0] (router).post_write.timeout must be positive, got -1s",
		},
		{
			name: "no files",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files: []
`,
			want: "servers[0] (router).files must not be empty",
		},
		{
			name: "disabled server without files",
			body: `version: 2
servers:
  - name: router
    enabled: false
    host: h
    user: u
    ssh_key: ./missing-key
    known_hosts: ./missing-known-hosts
    files: []
`,
			want: "servers[0] (router).files must not be empty",
		},
		{
			name: "empty source",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ""
        target: /a.txt
`,
			want: "servers[0] (router).files[0].source must not be empty",
		},
		{
			name: "relative target",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: relative/a.txt
`,
			want: `servers[0] (router).files[0].target must be absolute, got "relative/a.txt"`,
		},
		{
			name: "target trailing slash",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt/
`,
			want: `servers[0] (router).files[0].target must not end with '/', got "/a.txt/"`,
		},
		{
			name: "other_section bracket",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
        other_section: "a[b"
`,
			want: `servers[0] (router).files[0].other_section must not contain '[' or ']', got "a[b"`,
		},
		{
			name: "other_section control character",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
        other_section: "a\nb"
`,
			want: "servers[0] (router).files[0].other_section must not contain control characters",
		},
		{
			name: "other_section too long",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
        other_section: ` + strings.Repeat("a", 65) + `
`,
			want: "servers[0] (router).files[0].other_section must be at most 64 runes, got 65",
		},
		{
			name: "zero mode",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
        mode: "0000"
`,
			want: `servers[0] (router).files[0].mode must be octal between 0001 and 0777, got "0000"`,
		},
		{
			name: "invalid mode",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
        mode: "999"
`,
			want: `servers[0] (router).files[0].mode must be octal between 0001 and 0777, got "999"`,
		},
		{
			name: "duplicate source",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
      - source: ./domains.txt
        target: /b.txt
`,
			want: "servers[0] (router).files[1].source",
		},
		{
			name: "duplicate source across servers",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
  - name: vps-1
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /b.txt
`,
			want: "servers[1] (vps-1).files[0].source",
		},
		{
			name: "duplicate target within server",
			body: `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
      - source: ./key
        target: /a.txt
`,
			want: "servers[0] (router).files[1].target",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, tc.body)
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLoadSameTargetOnDifferentServersIsAllowed(t *testing.T) {
	path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /shared.txt
  - name: vps-1
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./key
        target: /shared.txt
`)
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFilesystemChecks(t *testing.T) {
	t.Run("missing ssh key", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./missing-key
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "servers[0] (router).ssh_key") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("agent mode requires SSH_AUTH_SOCK", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "SSH_AUTH_SOCK") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("agent mode allowed with SSH_AUTH_SOCK", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    known_hosts: ./known_hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`)
		if _, err := Load(path); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("missing known_hosts", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./missing-known-hosts
    files:
      - source: ./domains.txt
        target: /a.txt
`)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "servers[0] (router).known_hosts") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("known_hosts is a directory", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./khdir
    files:
      - source: ./domains.txt
        target: /a.txt
`)
		if err := os.Mkdir(filepath.Join(filepath.Dir(path), "khdir"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("absent source is allowed", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./absent.txt
        target: /a.txt
`)
		if _, err := Load(path); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("source is a directory", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    host: h
    user: u
    ssh_key: ./key
    known_hosts: ./known_hosts
    files:
      - source: ./sourcedir
        target: /a.txt
`)
		if err := os.Mkdir(filepath.Join(filepath.Dir(path), "sourcedir"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "servers[0] (router).files[0].source") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("disabled server skips filesystem checks", func(t *testing.T) {
		path := writeConfig(t, `version: 2
servers:
  - name: router
    enabled: false
    host: h
    user: u
    ssh_key: ./missing-key
    known_hosts: ./missing-known-hosts
    files:
      - source: ./missing-source.txt
        target: /a.txt
`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Servers[0].IsEnabled() {
			t.Fatal("server must stay disabled")
		}
	})
}

func TestDiscoverOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Discover(path); err != nil || got != path {
		t.Fatalf("explicit = %q, %v", got, err)
	}
	t.Setenv("DOMSYNC_CONFIG", path)
	if got, err := Discover(""); err != nil || got != path {
		t.Fatalf("env = %q, %v", got, err)
	}
	t.Setenv("DOMSYNC_CONFIG", "")
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "domsync.yaml"), []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Discover(""); err != nil || got != filepath.Join(dir, "domsync.yaml") {
		t.Fatalf("cwd = %q, %v", got, err)
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMinimal(t *testing.T, dir string) (configPath, keyPath, khPath string) {
	t.Helper()
	keyPath = filepath.Join(dir, "id_ed25519")
	khPath = filepath.Join(dir, "known_hosts")
	for _, p := range []string{keyPath, khPath} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configPath = filepath.Join(dir, "domsync.yaml")
	body := "version: 1\ntarget:\n  host: 192.0.2.1\n  user: root\n  ssh_key: ./id_ed25519\n  known_hosts: ./known_hosts\n  path: /etc/openclash/rule_provider/x.txt\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return
}

func TestLoadDefaultsAndPaths(t *testing.T) {
	dir := t.TempDir()
	configPath, keyPath, khPath := writeMinimal(t, dir)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.Domains != filepath.Join(dir, "domains.txt") {
		t.Fatalf("domains = %q", cfg.Source.Domains)
	}
	if cfg.Target.Port != 22 || cfg.Target.OtherSection != "other" || cfg.Target.Mode != "0644" {
		t.Fatalf("defaults = %+v", cfg.Target)
	}
	if !cfg.Target.BackupEnabled() || !cfg.Update.CheckEnabled() {
		t.Fatal("backup and update check must default to true")
	}
	if cfg.Target.SSHKey != keyPath || cfg.Target.KnownHosts != khPath {
		t.Fatalf("paths not resolved: %q %q", cfg.Target.SSHKey, cfg.Target.KnownHosts)
	}
	if cfg.Target.FileMode() != 0o644 {
		t.Fatalf("mode = %o", cfg.Target.FileMode())
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	_, _, _ = writeMinimal(t, dir)
	path := filepath.Join(dir, "domsync.yaml")
	body, _ := os.ReadFile(path)
	body = append(body, []byte("nonsense: true\n")...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := []string{
		"version: 2\n",
		"version: 1\ntarget:\n  host: \"\"\n  user: root\n  path: /x\n",
		"version: 1\ntarget:\n  host: h\n  user: root\n  path: relative/x\n",
		"version: 1\ntarget:\n  host: h\n  user: root\n  path: /x\n  mode: \"999\"\n",
		"version: 1\ntarget:\n  host: h\n  user: root\n  path: /x\n  post_write:\n    run: sometimes\n",
	}
	for _, body := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, "domsync.yaml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
		if _, err := Load(path); err == nil {
			t.Errorf("config %q loaded, want error", body)
		}
	}
}

func TestLoadOtherSectionValidation(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"closing bracket", `"oth]er"`, true},
		{"opening bracket", `"a[b"`, true},
		{"newline", `"a\nb"`, true},
		{"too long", strings.Repeat("a", 65), true},
		{"valid", "misc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath, _, _ := writeMinimal(t, dir)
			body, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			body = append(body, []byte("  other_section: "+tc.value+"\n")...)
			if err := os.WriteFile(configPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = Load(configPath)
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "target.other_section")) {
				t.Fatalf("error = %v, want a target.other_section error", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("error = %v, want none", err)
			}
		})
	}
}

func TestLoadRejectsZeroMode(t *testing.T) {
	dir := t.TempDir()
	configPath, _, _ := writeMinimal(t, dir)
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("  mode: \"0000\"\n")...)
	if err := os.WriteFile(configPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "target.mode") {
		t.Fatalf("error = %v, want a target.mode error", err)
	}
}

func TestLoadDuration(t *testing.T) {
	dir := t.TempDir()
	configPath, _, _ := writeMinimal(t, dir)
	body, _ := os.ReadFile(configPath)
	body = []byte(strings.Replace(string(body), "\n  path:", "\n  post_write:\n    timeout: 90s\n  path:", 1))
	if err := os.WriteFile(configPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Target.PostWrite.Timeout != 90*1e9 {
		t.Fatalf("timeout = %v", cfg.Target.PostWrite.Timeout)
	}
}

func TestLoadDurationErrorNamesLineAndValue(t *testing.T) {
	dir := t.TempDir()
	configPath, _, _ := writeMinimal(t, dir)
	body, _ := os.ReadFile(configPath)
	body = []byte(strings.Replace(string(body), "\n  path:", "\n  post_write:\n    timeout: abc\n  path:", 1))
	if err := os.WriteFile(configPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected a duration error")
	}
	message := err.Error()
	if !strings.Contains(message, "line 8") {
		t.Fatalf("error does not name the line: %v", err)
	}
	if !strings.Contains(message, `"abc"`) {
		t.Fatalf("error does not name the value: %v", err)
	}
}

func TestLoadAgentModeRequiresKnownHosts(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	path := filepath.Join(dir, "domsync.yaml")
	body := "version: 1\ntarget:\n  host: h\n  user: root\n  path: /x\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "known_hosts") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsKnownHostsDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath, _, khPath := writeMinimal(t, dir)
	if err := os.Remove(khPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(khPath, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Load(configPath)
	if err == nil || !strings.Contains(err.Error(), "known_hosts") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(dir, "domsync.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Discover(""); err != nil || got != filepath.Join(dir, "domsync.yaml") {
		t.Fatalf("cwd = %q, %v", got, err)
	}
}

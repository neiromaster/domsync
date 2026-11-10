package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	maxConfigSize        = 1 << 20
	maxOtherSectionRunes = 64
	maxServerNameLen     = 32
)

type Config struct {
	Version int      `yaml:"version"`
	Logging Logging  `yaml:"logging"`
	Update  Update   `yaml:"update"`
	Servers []Server `yaml:"servers"`
	path    string
}

type Server struct {
	Name       string    `yaml:"name"`
	Enabled    *bool     `yaml:"enabled"`
	Host       string    `yaml:"host"`
	Port       int       `yaml:"port"`
	User       string    `yaml:"user"`
	SSHKey     string    `yaml:"ssh_key"`
	KnownHosts string    `yaml:"known_hosts"`
	PostWrite  PostWrite `yaml:"post_write"`
	Files      []Pair    `yaml:"files"`
}

type Pair struct {
	Source       string `yaml:"source"`
	Target       string `yaml:"target"`
	OtherSection string `yaml:"other_section"`
	Mode         string `yaml:"mode"`
	Backup       *bool  `yaml:"backup"`
	BackupSuffix string `yaml:"backup_suffix"`
}

type PostWrite struct {
	Run      string   `yaml:"run"`
	Timeout  Duration `yaml:"timeout"`
	Commands []string `yaml:"commands"`
}

type Update struct {
	Check *bool `yaml:"check"`
}

type Logging struct {
	File string `yaml:"file"`
}

func (c *Config) Path() string { return c.path }

func (s Server) IsEnabled() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

func (p Pair) BackupEnabled() bool {
	if p.Backup == nil {
		return true
	}
	return *p.Backup
}

func (p Pair) FileMode() fs.FileMode {
	parsed, err := strconv.ParseUint(p.Mode, 8, 32)
	if err != nil {
		return 0
	}
	return fs.FileMode(parsed)
}

func (u Update) CheckEnabled() bool {
	if u.Check == nil {
		return true
	}
	return *u.Check
}

func Discover(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if fromEnv := os.Getenv("DOMSYNC_CONFIG"); fromEnv != "" {
		return filepath.Abs(fromEnv)
	}
	const localName = "domsync.yaml"
	if info, err := os.Stat(localName); err == nil && !info.IsDir() {
		return filepath.Abs(localName)
	}
	return "", errors.New("config not found: pass --config, set DOMSYNC_CONFIG, or create domsync.yaml")
}

func Load(explicit string) (*Config, error) {
	configPath, err := Discover(explicit)
	if err != nil {
		return nil, err
	}
	data, err := readConfig(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("config %s: file is empty", configPath)
		}
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("config %s: %w", configPath, err)
		}
		return nil, fmt.Errorf("config %s: multiple YAML documents are not supported", configPath)
	}
	cfg.path = configPath
	cfg.applyDefaults()
	if err := cfg.resolvePaths(); err != nil {
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	return &cfg, nil
}

func readConfig(configPath string) ([]byte, error) {
	file, err := os.Open(configPath) //nolint:gosec // configPath is the user-supplied CLI argument
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("config %s: larger than %d bytes", configPath, maxConfigSize)
	}
	return data, nil
}

func (c *Config) applyDefaults() {
	for i := range c.Servers {
		server := &c.Servers[i]
		if server.Port == 0 {
			server.Port = 22
		}
		if server.KnownHosts == "" {
			server.KnownHosts = "~/.ssh/known_hosts"
		}
		if server.PostWrite.Run == "" {
			server.PostWrite.Run = "on_change"
		}
		if server.PostWrite.Timeout == 0 {
			server.PostWrite.Timeout = Duration(120 * time.Second)
		}
		for j := range server.Files {
			pair := &server.Files[j]
			if pair.OtherSection == "" {
				pair.OtherSection = "other"
			}
			if pair.Mode == "" {
				pair.Mode = "0644"
			}
			if pair.BackupSuffix == "" {
				pair.BackupSuffix = ".domsync.bak"
			}
		}
	}
}

func (c *Config) resolvePaths() error {
	base := filepath.Dir(c.path)
	for i := range c.Servers {
		server := &c.Servers[i]
		label := serverLabel(i, *server)
		for _, field := range []struct {
			name  string
			value *string
		}{
			{"ssh_key", &server.SSHKey},
			{"known_hosts", &server.KnownHosts},
		} {
			resolved, err := expandPath(base, *field.value)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", label, field.name, err)
			}
			*field.value = resolved
		}
		for j := range server.Files {
			resolved, err := expandPath(base, server.Files[j].Source)
			if err != nil {
				return fmt.Errorf("%s.files[%d].source: %w", label, j, err)
			}
			server.Files[j].Source = resolved
		}
	}
	resolved, err := expandPath(base, c.Logging.File)
	if err != nil {
		return fmt.Errorf("logging.file: %w", err)
	}
	c.Logging.File = resolved
	return nil
}

func expandPath(base, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve %q: %w", value, err)
		}
		if value == "~" {
			return home, nil
		}
		return filepath.Join(home, value[2:]), nil
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(base, value), nil
}

func (c *Config) validate() error {
	if c.Version != 2 {
		return fmt.Errorf("version must be 2, got %d", c.Version)
	}
	if err := c.validateShape(); err != nil {
		return err
	}
	return c.validateFilesystem()
}

func (c *Config) validateShape() error {
	if len(c.Servers) == 0 {
		return errors.New("servers must not be empty")
	}
	names := make(map[string]string, len(c.Servers))
	sources := make(map[string]string)
	for i := range c.Servers {
		server := &c.Servers[i]
		label := serverLabel(i, *server)
		if server.Name == "" {
			return fmt.Errorf("%s.name must not be empty", label)
		}
		if len(server.Name) > maxServerNameLen {
			return fmt.Errorf("%s.name must be at most %d characters, got %d", label, maxServerNameLen, len(server.Name))
		}
		if !validServerName(server.Name) {
			return fmt.Errorf("%s.name must match [a-z0-9][a-z0-9_-]*, got %q", label, server.Name)
		}
		if previous, ok := names[server.Name]; ok {
			return fmt.Errorf("%s.name %q is already used by %s", label, server.Name, previous)
		}
		names[server.Name] = label
		if server.Host == "" {
			return fmt.Errorf("%s.host must not be empty", label)
		}
		if server.User == "" {
			return fmt.Errorf("%s.user must not be empty", label)
		}
		if server.Port < 1 || server.Port > 65535 {
			return fmt.Errorf("%s.port must be between 1 and 65535, got %d", label, server.Port)
		}
		switch server.PostWrite.Run {
		case "on_change", "always", "never":
		default:
			return fmt.Errorf("%s.post_write.run must be one of on_change, always, never; got %q", label, server.PostWrite.Run)
		}
		if server.PostWrite.Timeout <= 0 {
			return fmt.Errorf("%s.post_write.timeout must be positive, got %s", label, time.Duration(server.PostWrite.Timeout))
		}
		if len(server.Files) == 0 {
			return fmt.Errorf("%s.files must not be empty", label)
		}
		targets := make(map[string]string, len(server.Files))
		for j := range server.Files {
			pairLabel := fmt.Sprintf("%s.files[%d]", label, j)
			if err := validatePair(pairLabel, server.Files[j]); err != nil {
				return err
			}
			pair := server.Files[j]
			if previous, ok := sources[pair.Source]; ok {
				return fmt.Errorf("%s.source %q is already used by %s", pairLabel, pair.Source, previous)
			}
			sources[pair.Source] = pairLabel + ".source"
			if previous, ok := targets[pair.Target]; ok {
				return fmt.Errorf("%s.target %q is already used by %s", pairLabel, pair.Target, previous)
			}
			targets[pair.Target] = pairLabel + ".target"
		}
	}
	return nil
}

func validatePair(label string, pair Pair) error {
	if pair.Source == "" {
		return fmt.Errorf("%s.source must not be empty", label)
	}
	if !path.IsAbs(pair.Target) {
		return fmt.Errorf("%s.target must be absolute, got %q", label, pair.Target)
	}
	if strings.HasSuffix(pair.Target, "/") {
		return fmt.Errorf("%s.target must not end with '/', got %q", label, pair.Target)
	}
	if pair.OtherSection == "" {
		return fmt.Errorf("%s.other_section must not be empty", label)
	}
	if strings.ContainsAny(pair.OtherSection, "[]") {
		return fmt.Errorf("%s.other_section must not contain '[' or ']', got %q", label, pair.OtherSection)
	}
	if utf8.RuneCountInString(pair.OtherSection) > maxOtherSectionRunes {
		return fmt.Errorf("%s.other_section must be at most %d runes, got %d", label, maxOtherSectionRunes, utf8.RuneCountInString(pair.OtherSection))
	}
	for _, r := range pair.OtherSection {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s.other_section must not contain control characters, got %q", label, pair.OtherSection)
		}
	}
	mode, err := strconv.ParseUint(pair.Mode, 8, 32)
	if err != nil || mode == 0 || mode > 0o777 {
		return fmt.Errorf("%s.mode must be octal between 0001 and 0777, got %q", label, pair.Mode)
	}
	return nil
}

func (c *Config) validateFilesystem() error {
	for i := range c.Servers {
		server := &c.Servers[i]
		if !server.IsEnabled() {
			continue
		}
		label := serverLabel(i, *server)
		if server.SSHKey != "" {
			if err := requireRegularFile(label+".ssh_key", server.SSHKey); err != nil {
				return err
			}
		} else if os.Getenv("SSH_AUTH_SOCK") == "" {
			return fmt.Errorf("%s.ssh_key is empty and SSH_AUTH_SOCK is not set", label)
		}
		if err := requireRegularFile(label+".known_hosts", server.KnownHosts); err != nil {
			return err
		}
		for j := range server.Files {
			if err := requireRegularFileOrAbsent(fmt.Sprintf("%s.files[%d].source", label, j), server.Files[j].Source); err != nil {
				return err
			}
		}
	}
	return nil
}

func serverLabel(index int, server Server) string {
	if server.Name == "" {
		return fmt.Sprintf("servers[%d]", index)
	}
	return fmt.Sprintf("servers[%d] (%s)", index, server.Name)
}

func validServerName(name string) bool {
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return name != ""
}

func requireRegularFile(key, name string) error {
	info, err := os.Stat(name)
	if err != nil {
		return fmt.Errorf("%s %q: %w", key, name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %q is not a regular file", key, name)
	}
	return nil
}

func requireRegularFileOrAbsent(key, name string) error {
	info, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s %q: %w", key, name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %q is not a regular file", key, name)
	}
	return nil
}

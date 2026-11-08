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

	"go.yaml.in/yaml/v3"
)

const maxConfigSize = 1 << 20

type Config struct {
	Version int     `yaml:"version"`
	Source  Source  `yaml:"source"`
	Target  Target  `yaml:"target"`
	Update  Update  `yaml:"update"`
	Logging Logging `yaml:"logging"`
	path    string
}

type Source struct {
	Domains string `yaml:"domains"`
}

type Target struct {
	Host         string    `yaml:"host"`
	Port         int       `yaml:"port"`
	User         string    `yaml:"user"`
	SSHKey       string    `yaml:"ssh_key"`
	KnownHosts   string    `yaml:"known_hosts"`
	Path         string    `yaml:"path"`
	OtherSection string    `yaml:"other_section"`
	Mode         string    `yaml:"mode"`
	BackupSuffix string    `yaml:"backup_suffix"`
	Backup       *bool     `yaml:"backup"`
	PostWrite    PostWrite `yaml:"post_write"`
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

func (t Target) BackupEnabled() bool {
	if t.Backup == nil {
		return true
	}
	return *t.Backup
}

func (t Target) FileMode() fs.FileMode {
	parsed, err := strconv.ParseUint(t.Mode, 8, 32)
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
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", configPath, err)
	}
	defer file.Close()
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
	if c.Source.Domains == "" {
		c.Source.Domains = "domains.txt"
	}
	if c.Target.Port == 0 {
		c.Target.Port = 22
	}
	if c.Target.KnownHosts == "" {
		c.Target.KnownHosts = "~/.ssh/known_hosts"
	}
	if c.Target.OtherSection == "" {
		c.Target.OtherSection = "other"
	}
	if c.Target.Mode == "" {
		c.Target.Mode = "0644"
	}
	if c.Target.BackupSuffix == "" {
		c.Target.BackupSuffix = ".domsync.bak"
	}
	if c.Target.PostWrite.Run == "" {
		c.Target.PostWrite.Run = "on_change"
	}
	if c.Target.PostWrite.Timeout == 0 {
		c.Target.PostWrite.Timeout = Duration(120 * time.Second)
	}
}

func (c *Config) resolvePaths() error {
	base := filepath.Dir(c.path)
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"source.domains", &c.Source.Domains},
		{"target.ssh_key", &c.Target.SSHKey},
		{"target.known_hosts", &c.Target.KnownHosts},
		{"logging.file", &c.Logging.File},
	} {
		resolved, err := expandPath(base, *field.value)
		if err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
		*field.value = resolved
	}
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
	if c.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", c.Version)
	}
	if c.Target.Host == "" {
		return errors.New("target.host must not be empty")
	}
	if c.Target.User == "" {
		return errors.New("target.user must not be empty")
	}
	if c.Target.Port < 1 || c.Target.Port > 65535 {
		return fmt.Errorf("target.port must be between 1 and 65535, got %d", c.Target.Port)
	}
	if !path.IsAbs(c.Target.Path) {
		return fmt.Errorf("target.path must be absolute, got %q", c.Target.Path)
	}
	if strings.HasSuffix(c.Target.Path, "/") {
		return fmt.Errorf("target.path must not end with '/', got %q", c.Target.Path)
	}
	if c.Target.OtherSection == "" {
		return errors.New("target.other_section must not be empty")
	}
	mode, err := strconv.ParseUint(c.Target.Mode, 8, 32)
	if err != nil || mode > 0o777 {
		return fmt.Errorf("target.mode must be octal and no greater than 0777, got %q", c.Target.Mode)
	}
	switch c.Target.PostWrite.Run {
	case "on_change", "always", "never":
	default:
		return fmt.Errorf("target.post_write.run must be one of on_change, always, never; got %q", c.Target.PostWrite.Run)
	}
	if c.Target.PostWrite.Timeout <= 0 {
		return fmt.Errorf("target.post_write.timeout must be positive, got %s", time.Duration(c.Target.PostWrite.Timeout))
	}
	if c.Target.SSHKey != "" {
		if err := requireRegularFile("target.ssh_key", c.Target.SSHKey); err != nil {
			return err
		}
	} else if os.Getenv("SSH_AUTH_SOCK") == "" {
		return errors.New("target.ssh_key is empty and SSH_AUTH_SOCK is not set")
	}
	return requireRegularFile("target.known_hosts", c.Target.KnownHosts)
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

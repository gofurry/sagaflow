package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App    AppConfig    `yaml:"app"`
	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`
	Jobs   JobsConfig   `yaml:"jobs"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	Env      string `yaml:"env"`
	Version  string `yaml:"-"`
	DataDir  string `yaml:"data_dir"`
	LogLevel string `yaml:"log_level"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type AuthConfig struct {
	SessionTTL   string `yaml:"session_ttl"`
	CookieName   string `yaml:"cookie_name"`
	CookieSecure bool   `yaml:"cookie_secure"`
}

type JobsConfig struct {
	Concurrency  int    `yaml:"concurrency"`
	PollInterval string `yaml:"poll_interval"`
	Lease        string `yaml:"lease"`
}

func Default() Config {
	return Config{
		App: AppConfig{
			Name:     "sagaflow",
			Env:      "production",
			Version:  "dev",
			DataDir:  defaultDataDir(),
			LogLevel: "info",
		},
		Server: ServerConfig{Host: "127.0.0.1", Port: 8080},
		Auth: AuthConfig{
			SessionTTL: "720h",
			CookieName: "sagaflow_session",
		},
		Jobs: JobsConfig{
			Concurrency:  2,
			PollInterval: "750ms",
			Lease:        "2h",
		},
	}
}

// Load returns usable defaults when no configuration file exists. An explicit
// path is strict so command-line mistakes never silently start another
// instance with a different data directory.
func Load(path string) (Config, error) {
	cfg := Default()
	resolved, explicit := resolvePath(path, cfg.App.DataDir)
	if data, err := os.ReadFile(resolved); err == nil {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %q: %w", resolved, err)
		}
	} else if explicit || !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config %q: %w", resolved, err)
	}
	cfg.applyEnv()
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func WriteDefault(path string, force bool) error {
	cfg := Default()
	if path == "" {
		path = DefaultConfigPath(cfg.App.DataDir)
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists at %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func DefaultConfigPath(dataDir string) string { return filepath.Join(dataDir, "config.yaml") }
func (c Config) Address() string              { return net.JoinHostPort(c.Server.Host, strconv.Itoa(c.Server.Port)) }
func (c Config) DatabasePath() string         { return filepath.Join(c.App.DataDir, "sagaflow.db") }
func (c Config) ObjectDir() string            { return filepath.Join(c.App.DataDir, "objects") }
func (c Config) TempDir() string              { return filepath.Join(c.App.DataDir, "temp") }
func (c Config) BackupDir() string            { return filepath.Join(c.App.DataDir, "backups") }
func (c Config) SecretDir() string            { return filepath.Join(c.App.DataDir, "secrets") }
func (c Config) MasterKeyPath() string        { return filepath.Join(c.SecretDir(), "master.key") }

func (c Config) SessionTTL() time.Duration {
	d, _ := time.ParseDuration(c.Auth.SessionTTL)
	return d
}

func (c Config) JobPollInterval() time.Duration {
	d, _ := time.ParseDuration(c.Jobs.PollInterval)
	return d
}

func (c Config) JobLease() time.Duration {
	d, _ := time.ParseDuration(c.Jobs.Lease)
	return d
}

func (c Config) IsLoopback() bool {
	host := strings.TrimSpace(c.Server.Host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) EnsureRuntimeDirs() error {
	for _, path := range []string{c.App.DataDir, c.ObjectDir(), c.TempDir(), c.BackupDir(), c.SecretDir()} {
		mode := os.FileMode(0o755)
		if path == c.SecretDir() {
			mode = 0o700
		}
		if err := os.MkdirAll(path, mode); err != nil {
			return fmt.Errorf("create runtime dir %s: %w", path, err)
		}
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.App.DataDir) == "" {
		return errors.New("app.data_dir is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Server.Host) == "" {
		return errors.New("server.host is required")
	}
	if _, err := time.ParseDuration(c.Auth.SessionTTL); err != nil {
		return fmt.Errorf("auth.session_ttl is invalid: %w", err)
	}
	if c.Jobs.Concurrency <= 0 || c.Jobs.Concurrency > 32 {
		return errors.New("jobs.concurrency must be between 1 and 32")
	}
	if _, err := time.ParseDuration(c.Jobs.PollInterval); err != nil {
		return fmt.Errorf("jobs.poll_interval is invalid: %w", err)
	}
	if _, err := time.ParseDuration(c.Jobs.Lease); err != nil {
		return fmt.Errorf("jobs.lease is invalid: %w", err)
	}
	return nil
}

func (c *Config) normalize() {
	if c.App.Name == "" {
		c.App.Name = "sagaflow"
	}
	if c.App.Env == "" {
		c.App.Env = "production"
	}
	if c.App.Version == "" {
		c.App.Version = "dev"
	}
	if c.App.DataDir == "" {
		c.App.DataDir = defaultDataDir()
	}
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.Server.Host == "" {
		c.Server.Host = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Auth.SessionTTL == "" {
		c.Auth.SessionTTL = "720h"
	}
	if c.Auth.CookieName == "" {
		c.Auth.CookieName = "sagaflow_session"
	}
	if c.Jobs.Concurrency == 0 {
		c.Jobs.Concurrency = 2
	}
	if c.Jobs.PollInterval == "" {
		c.Jobs.PollInterval = "750ms"
	}
	if c.Jobs.Lease == "" {
		c.Jobs.Lease = "2h"
	}
	c.App.DataDir = filepath.Clean(c.App.DataDir)
}

func (c *Config) applyEnv() {
	setString(&c.App.Env, "SAGAFLOW_ENV")
	setString(&c.App.DataDir, "SAGAFLOW_DATA_DIR")
	setString(&c.App.LogLevel, "SAGAFLOW_LOG_LEVEL")
	setString(&c.Server.Host, "SAGAFLOW_HOST")
	setInt(&c.Server.Port, "SAGAFLOW_PORT")
	setString(&c.Auth.SessionTTL, "SAGAFLOW_SESSION_TTL")
	setBool(&c.Auth.CookieSecure, "SAGAFLOW_COOKIE_SECURE")
	setInt(&c.Jobs.Concurrency, "SAGAFLOW_JOB_CONCURRENCY")
}

func resolvePath(path, dataDir string) (string, bool) {
	if path != "" {
		return path, true
	}
	if envPath := os.Getenv("SAGAFLOW_CONFIG"); envPath != "" {
		return envPath, true
	}
	return DefaultConfigPath(dataDir), false
}

func defaultDataDir() string {
	if configured := strings.TrimSpace(os.Getenv("SAGAFLOW_DATA_DIR")); configured != "" {
		return configured
	}
	executable, _ := os.Executable()
	cwd, _ := os.Getwd()
	return portableDataDir(executable, cwd, os.TempDir())
}

func portableDataDir(executable, cwd, tempDir string) string {
	// A released binary owns a sibling data directory so the whole SagaFlow
	// installation can be moved or backed up as one folder. `go run` builds in
	// an ephemeral go-build directory, so development falls back to the current
	// checkout instead of writing beside the temporary executable.
	if executable = strings.TrimSpace(executable); executable != "" && !isGoRunExecutable(executable, tempDir) {
		return filepath.Join(filepath.Dir(executable), "data")
	}
	if cwd = strings.TrimSpace(cwd); cwd != "" {
		return filepath.Join(cwd, "data")
	}
	return "data"
}

func isGoRunExecutable(executable, tempDir string) bool {
	executable = strings.ToLower(filepath.ToSlash(filepath.Clean(executable)))
	tempDir = strings.ToLower(strings.TrimSuffix(filepath.ToSlash(filepath.Clean(tempDir)), "/"))
	return tempDir != "" && strings.HasPrefix(executable, tempDir+"/") && strings.Contains(executable, "/go-build")
}

func setString(target *string, key string) {
	if value := os.Getenv(key); value != "" {
		*target = value
	}
}

func setInt(target *int, key string) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			*target = parsed
		}
	}
}

func setBool(target *bool, key string) {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			*target = parsed
		}
	}
}

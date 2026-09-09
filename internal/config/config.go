package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRefreshSeconds = 2
	DefaultQueryTimeout   = 5
)

type PostgresTarget struct {
	Name string `yaml:"name"`
	DSN  string `yaml:"dsn"`
}

type RedisTarget struct {
	Name     string `yaml:"name"`
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type Config struct {
	RefreshSeconds      int              `yaml:"refresh_seconds"`
	QueryTimeoutSeconds int              `yaml:"query_timeout_seconds"`
	Postgres            []PostgresTarget `yaml:"postgres"`
	Redis               []RedisTarget    `yaml:"redis"`
}

func Default() Config {
	return Config{
		RefreshSeconds:      DefaultRefreshSeconds,
		QueryTimeoutSeconds: DefaultQueryTimeout,
	}
}

func Resolve(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if env := os.Getenv("PRODTOP_CONFIG"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err == nil {
		candidate := filepath.Join(home, ".config", "prodtop", "config.yaml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	return "prodtop.yaml"
}

func Load(flagPath string) (Config, bool, error) {
	path := Resolve(flagPath)
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, true, nil
		}
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.RefreshSeconds == 0 {
		cfg.RefreshSeconds = DefaultRefreshSeconds
	}
	if cfg.QueryTimeoutSeconds == 0 {
		cfg.QueryTimeoutSeconds = DefaultQueryTimeout
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, err
	}
	return cfg, false, nil
}

func (c Config) Validate() error {
	if c.RefreshSeconds < 1 || c.RefreshSeconds > 60 {
		return fmt.Errorf("refresh_seconds must be between 1 and 60, got %d", c.RefreshSeconds)
	}
	if c.QueryTimeoutSeconds < 1 || c.QueryTimeoutSeconds > 30 {
		return fmt.Errorf("query_timeout_seconds must be between 1 and 30, got %d", c.QueryTimeoutSeconds)
	}
	seen := map[string]struct{}{}
	for _, pg := range c.Postgres {
		if pg.Name == "" {
			return fmt.Errorf("postgres target with empty name")
		}
		if pg.DSN == "" {
			return fmt.Errorf("postgres target %q has empty dsn", pg.Name)
		}
		if _, dup := seen["pg/"+pg.Name]; dup {
			return fmt.Errorf("duplicate target name %q", pg.Name)
		}
		seen["pg/"+pg.Name] = struct{}{}
	}
	for _, r := range c.Redis {
		if r.Name == "" {
			return fmt.Errorf("redis target with empty name")
		}
		if r.Addr == "" {
			return fmt.Errorf("redis target %q has empty addr", r.Name)
		}
		if _, dup := seen["redis/"+r.Name]; dup {
			return fmt.Errorf("duplicate target name %q", r.Name)
		}
		seen["redis/"+r.Name] = struct{}{}
	}
	return nil
}

func RedactedDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.User == nil {
		return dsn
	}
	parsed.User = url.UserPassword(parsed.User.Username(), "***")
	return parsed.String()
}

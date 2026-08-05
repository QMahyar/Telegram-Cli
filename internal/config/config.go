// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"telegram-cli/internal/cliutil"
)

type Config struct {
	BaseURL       string            `toml:"base_url,omitempty"`
	AuthHeaderVal string            `toml:"auth_header,omitempty"`
	Headers       map[string]string `toml:"headers,omitempty"`
	AuthSource    string            `toml:"-"`
	Path          string            `toml:"-"`
}

func Load(configPath string) (*Config, error) {
	cfg, err := LoadForEdit(configPath)
	if err != nil {
		return nil, err
	}

	// Env var overrides

	// Base URL override (used by the verify harness to point at mock/test servers)
	if v := os.Getenv("TELEGRAM_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	return cfg, nil
}

// LoadForEdit loads configuration exactly like Load, but without environment
// overrides. Write flows (config set/unset) must go through this so an env
// override is never persisted into the config file.
func LoadForEdit(configPath string) (*Config, error) {
	cfg := &Config{}

	// Resolve config path
	path, explicitConfigFile, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	cfg.Path = path

	if explicitConfigFile {
		if err := readConfigFile(path, cfg, "config-kind path"); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		legacyPath, err := LegacyConfigPath()
		if err != nil {
			return nil, err
		}
		data, sourcePath, err := cliutil.ReadFileWithLegacyFallback(path, legacyPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			owner := "config-kind path"
			if sourcePath == legacyPath {
				owner = "legacy config path"
			}
			parsed := *cfg
			if err := parseConfigData(data, &parsed, sourcePath, owner); err != nil {
				if sourcePath == legacyPath {
					fmt.Fprintf(os.Stderr, "warning: legacy config parse skipped for %s: %v\n", sourcePath, err)
				} else {
					return nil, err
				}
			} else {
				*cfg = parsed
			}
		}
	}
	cfg.Path = path
	return cfg, nil
}

// Save writes cfg to path atomically (temp file + fsync + rename) with
// owner-only permissions, creating the parent directory if needed. Runtime
// fields (Path, AuthSource) and empty headers are not serialized.
func Save(path string, cfg *Config) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("save config: empty path")
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("creating temporary config file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("hardening config file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("installing config: %w", err)
	}
	return nil
}

func resolveConfigPath(configPath string) (string, bool, error) {
	if strings.TrimSpace(configPath) != "" {
		return configPath, true, nil
	}
	if path := os.Getenv("TELEGRAM_CONFIG"); path != "" {
		return path, true, nil
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(dir, "config.toml"), false, nil
}

func LegacyConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve legacy config path: %w", err)
	}
	return filepath.Join(home, ".config", "telegram-cli", "config.toml"), nil
}

func readConfigFile(path string, cfg *Config, owner string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return parseConfigData(data, cfg, path, owner)
}

func parseConfigData(data []byte, cfg *Config, path string, owner string) error {
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing %s %s: %w", owner, path, err)
	}
	return nil
}

func (c *Config) AuthHeader() string {
	if c.AuthHeaderVal != "" {
		return c.AuthHeaderVal
	}
	return ""
}

// Raw browser-session values count as credentials even when no header
// representation exists; hand-coded flows may also preserve a working header.
func (c *Config) CredentialConfigured() bool {
	if c == nil {
		return false
	}
	return c.AuthHeader() != ""
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadForEdit_NonexistentFile(t *testing.T) {
	cfg, err := LoadForEdit("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("LoadForEdit should not error on nonexistent file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadForEdit_EmptyPath(t *testing.T) {
	cfg, err := LoadForEdit("")
	if err != nil {
		t.Fatalf("LoadForEdit with empty path should not error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestSave_AndLoadForEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	orig := &Config{
		BaseURL:       "https://example.com",
		AuthHeaderVal: "secret-token",
		Headers:       map[string]string{"X-Custom": "value"},
	}

	if err := Save(path, orig); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadForEdit(path)
	if err != nil {
		t.Fatalf("LoadForEdit failed: %v", err)
	}

	if loaded.BaseURL != orig.BaseURL {
		t.Errorf("BaseURL = %q, want %q", loaded.BaseURL, orig.BaseURL)
	}
	if loaded.AuthHeaderVal != orig.AuthHeaderVal {
		t.Errorf("AuthHeaderVal = %q, want %q", loaded.AuthHeaderVal, orig.AuthHeaderVal)
	}
}

func TestSave_EmptyPath(t *testing.T) {
	err := Save("", &Config{})
	if err == nil {
		t.Error("Save with empty path should error")
	}
}

func TestConfig_AuthHeader(t *testing.T) {
	cfg := &Config{AuthHeaderVal: "test-token"}
	if got := cfg.AuthHeader(); got != "test-token" {
		t.Errorf("AuthHeader() = %q, want %q", got, "test-token")
	}

	cfg2 := &Config{}
	if got := cfg2.AuthHeader(); got != "" {
		t.Errorf("AuthHeader() for empty config = %q, want empty", got)
	}
}

func TestConfig_CredentialConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{"nil config", nil, false},
		{"no credentials", &Config{}, false},
		{"with auth header", &Config{AuthHeaderVal: "token"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.CredentialConfigured(); got != tt.want {
				t.Errorf("CredentialConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.toml")

	if err := Save(path, &Config{BaseURL: "https://test.com"}); err != nil {
		t.Fatalf("Save should create parent dirs: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file should exist after Save")
	}
}

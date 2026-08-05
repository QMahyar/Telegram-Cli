// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telegram-cli/internal/cliutil"
	"telegram-cli/internal/cliutil/testenv"
)

// configTestEnv returns an isolated home dir and the config file path inside
// it. Isolation is env-based (testenv.Isolate auto-restores after the test),
// because the --home flag sets a package-global override that would leak into
// sibling tests. --no-learn keeps the teach/recall journal out of the picture.
func configTestEnv(t *testing.T) (home, cfgPath string) {
	t.Helper()
	home = testenv.Isolate(t, cliutil.ConfigDir)
	cfgPath = filepath.Join(home, "config.toml")
	return home, cfgPath
}

func runConfig(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	full := []string{"config"}
	full = append(full, args...)
	full = append(full, "--config", filepath.Join(home, "config.toml"), "--no-learn")
	return runRootArgs(t, full...)
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file %s: %v", path, err)
	}
	return string(data)
}

func TestConfigPathPrintsResolvedFile(t *testing.T) {
	home, cfgPath := configTestEnv(t)
	stdout, stderr, err := runConfig(t, home, "path")
	if err != nil {
		t.Fatalf("config path: %v (stderr=%q)", err, stderr)
	}
	// config path prints the resolved file path exactly; on Windows that
	// means native backslash separators.
	if strings.TrimSpace(stdout) != cfgPath {
		t.Errorf("config path = %q, want %q", stdout, cfgPath)
	}
}

func TestConfigShowEmpty(t *testing.T) {
	home, _ := configTestEnv(t)
	stdout, stderr, err := runConfig(t, home, "--agent")
	if err != nil {
		t.Fatalf("config show: %v (stderr=%q)", err, stderr)
	}
	var env struct {
		Meta    map[string]any `json:"meta"`
		Results map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("show JSON: %v (stdout=%q)", err, stdout)
	}
	if env.Results["base_url"] != "" {
		t.Errorf("base_url = %q, want empty", env.Results["base_url"])
	}
	if _, ok := env.Results["auth_header"]; ok {
		t.Errorf("auth_header must be omitted when unset; got %v", env.Results["auth_header"])
	}
	if _, ok := env.Results["headers"]; ok {
		t.Errorf("headers must be omitted when empty; got %v", env.Results["headers"])
	}
}

func TestConfigSetGetUnsetRoundTrip(t *testing.T) {
	home, cfgPath := configTestEnv(t)

	stdout, stderr, err := runConfig(t, home, "set", "base_url", "https://api.example.com")
	if err != nil {
		t.Fatalf("config set base_url: %v (stderr=%q)", err, stderr)
	}
	if !strings.Contains(stdout, "base_url") {
		t.Errorf("set response = %q, want mention of base_url", stdout)
	}
	if !strings.Contains(readConfigFile(t, cfgPath), "https://api.example.com") {
		t.Errorf("config file missing base_url after set:\n%s", readConfigFile(t, cfgPath))
	}

	got, stderr, err := runConfig(t, home, "get", "base_url")
	if err != nil {
		t.Fatalf("config get base_url: %v (stderr=%q)", err, stderr)
	}
	if strings.TrimSpace(got) != "https://api.example.com" {
		t.Errorf("config get base_url = %q", got)
	}

	_, stderr, err = runConfig(t, home, "unset", "base_url")
	if err != nil {
		t.Fatalf("config unset base_url: %v (stderr=%q)", err, stderr)
	}
	if strings.Contains(readConfigFile(t, cfgPath), "base_url") {
		t.Errorf("config file still has base_url after unset:\n%s", readConfigFile(t, cfgPath))
	}
}

func TestConfigSetHeaders(t *testing.T) {
	home, cfgPath := configTestEnv(t)

	if _, stderr, err := runConfig(t, home, "set", "headers.X-Tenant", "my-tenant"); err != nil {
		t.Fatalf("set header: %v (stderr=%q)", err, stderr)
	}
	if !strings.Contains(readConfigFile(t, cfgPath), "my-tenant") {
		t.Fatalf("header not persisted:\n%s", readConfigFile(t, cfgPath))
	}

	got, _, err := runConfig(t, home, "get", "headers.X-Tenant")
	if err != nil {
		t.Fatalf("get header: %v", err)
	}
	if strings.TrimSpace(got) != "my-tenant" {
		t.Errorf("get header = %q, want my-tenant", got)
	}

	if _, stderr, err := runConfig(t, home, "unset", "headers.X-Tenant"); err != nil {
		t.Fatalf("unset header: %v (stderr=%q)", err, stderr)
	}
	if strings.Contains(readConfigFile(t, cfgPath), "my-tenant") {
		t.Errorf("header still present after unset:\n%s", readConfigFile(t, cfgPath))
	}
}

func TestConfigAuthHeaderNeverEchoed(t *testing.T) {
	home, cfgPath := configTestEnv(t)

	if _, stderr, err := runConfig(t, home, "set", "auth_header", "Bearer supersecret"); err != nil {
		t.Fatalf("set auth_header: %v (stderr=%q)", err, stderr)
	}
	if !strings.Contains(readConfigFile(t, cfgPath), "supersecret") {
		t.Fatalf("auth_header not persisted:\n%s", readConfigFile(t, cfgPath))
	}

	for _, args := range [][]string{{"--agent"}, {"get", "auth_header"}, {"get", "auth_header", "--agent"}} {
		stdout, stderr, err := runConfig(t, home, args...)
		if err != nil {
			t.Fatalf("config %v: %v (stderr=%q)", args, err, stderr)
		}
		if strings.Contains(stdout, "supersecret") {
			t.Errorf("config %v echoed the secret: %q", args, stdout)
		}
		if !strings.Contains(stdout, "redacted") {
			t.Errorf("config %v did not mark auth_header redacted: %q", args, stdout)
		}
	}
}

func TestConfigSetInvalidInputsExitUsage(t *testing.T) {
	home, _ := configTestEnv(t)

	cases := []struct {
		name string
		args []string
	}{
		{"unknown key in set", []string{"set", "bogus", "x"}},
		{"unknown key in get", []string{"get", "bogus"}},
		{"unknown key in unset", []string{"unset", "bogus"}},
		{"empty value", []string{"set", "base_url", " "}},
		{"relative base_url", []string{"set", "base_url", "api.example.com"}},
		{"empty header name", []string{"set", "headers.", "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runConfig(t, home, tc.args...)
			if err == nil {
				t.Fatalf("config %v should fail", tc.args)
			}
			if got := ExitCode(err); got != 2 {
				t.Errorf("config %v exit code = %d, want 2 (usage)", tc.args, got)
			}
		})
	}
}

func TestConfigSetPreservesExistingKeys(t *testing.T) {
	home, cfgPath := configTestEnv(t)

	if _, stderr, err := runConfig(t, home, "set", "base_url", "https://one.example.com"); err != nil {
		t.Fatalf("set base_url: %v (stderr=%q)", err, stderr)
	}
	if _, stderr, err := runConfig(t, home, "set", "headers.X-Keep", "kept"); err != nil {
		t.Fatalf("set header: %v (stderr=%q)", err, stderr)
	}
	content := readConfigFile(t, cfgPath)
	if !strings.Contains(content, "one.example.com") || !strings.Contains(content, "kept") {
		t.Errorf("second set dropped the first key:\n%s", content)
	}
}

func TestConfigEnvOverrideNeverPersisted(t *testing.T) {
	home, cfgPath := configTestEnv(t)

	t.Setenv("TELEGRAM_BASE_URL", "http://env-only.example.com")

	// A read honors the override...
	stdout, stderr, err := runConfig(t, home, "get", "base_url")
	if err != nil {
		t.Fatalf("get base_url: %v (stderr=%q)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "http://env-only.example.com" {
		t.Errorf("get base_url = %q, want env override", stdout)
	}

	// ...but a write must never persist it.
	if _, stderr, err := runConfig(t, home, "set", "headers.X-Env", "y"); err != nil {
		t.Fatalf("set header: %v (stderr=%q)", err, stderr)
	}
	if strings.Contains(readConfigFile(t, cfgPath), "env-only") {
		t.Errorf("env override leaked into the config file:\n%s", readConfigFile(t, cfgPath))
	}
}

func TestConfigSetRequiresValueArgument(t *testing.T) {
	home, _ := configTestEnv(t)
	// The usage-exit-code wrap happens in Execute(), not in the
	// RootCmd().Execute() harness below; assert failure only here (the real
	// binary exits 2 for this shape — covered by the isCobraUsageError
	// classification of cobra's "accepts N arg(s)" errors).
	_, _, err := runConfig(t, home, "set", "base_url")
	if err == nil {
		t.Fatal("config set with one arg should fail")
	}
}

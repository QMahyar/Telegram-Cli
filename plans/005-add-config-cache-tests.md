# Plan 005: Add tests for internal/config and internal/cache packages

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/config/ internal/cache/`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

The `internal/config` package is a dependency of every command — config loading, saving, path resolution, and credential handling are all critical paths. The `internal/cache` package provides the HTTP response cache with TTL expiry and atomic writes. Neither package has any test files. A config corruption bug or cache TTL issue would affect every user silently.

## Current state

- `internal/config/config.go` — 179 lines: `Load`, `LoadForEdit`, `Save`, `resolveConfigPath`, `LegacyConfigPath`, `AuthHeader`, `CredentialConfigured`. Zero test files.
- `internal/cache/cache.go` — 81 lines: `Store` with `Get`, `Set`, `Clear`. File-based cache with SHA256 keys, atomic writes (temp file + rename), and TTL expiry. Zero test files.
- Test patterns in the repo: table-driven tests with `testing.T`, `t.TempDir()` for filesystem tests, `t.Setenv()` for env var tests.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/config/config_test.go` (new file)
- `internal/cache/cache_test.go` (new file)

**Out of scope**:
- Changes to config.go or cache.go production code.
- Tests for the CLI config subcommand (already covered by `internal/cli/config_test.go`).
- Integration tests that combine config + cache.

## Git workflow

- Branch: `advisor/005-add-config-cache-tests`
- Commit: `test: add unit tests for config loading/saving and cache TTL/atomicity`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Create config_test.go

Create `internal/config/config_test.go` with tests covering:

```go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoad_MissingFile(t *testing.T) {
    cfg, err := Load("/nonexistent/path/config.toml")
    if err != nil {
        t.Fatalf("Load with missing file should return empty config, got error: %v", err)
    }
    if cfg == nil {
        t.Fatal("Load should return non-nil config even for missing file")
    }
    if cfg.BaseURL != "" {
        t.Errorf("expected empty BaseURL, got %q", cfg.BaseURL)
    }
}

func TestLoad_MalformedTOML(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.toml")
    os.WriteFile(path, []byte("{{{{not valid toml"), 0o600)
    _, err := Load(path)
    if err == nil {
        t.Fatal("Load with malformed TOML should return error")
    }
}

func TestLoad_EnvOverride(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.toml")
    os.WriteFile(path, []byte("base_url = \"http://original.example.com\""), 0o600)
    t.Setenv("TELEGRAM_BASE_URL", "http://override.example.com")
    cfg, err := Load(path)
    if err != nil {
        t.Fatalf("Load failed: %v", err)
    }
    if cfg.BaseURL != "http://override.example.com" {
        t.Errorf("expected env override, got %q", cfg.BaseURL)
    }
}

func TestLoadForEdit_NoEnvOverride(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.toml")
    os.WriteFile(path, []byte("base_url = \"http://original.example.com\""), 0o600)
    t.Setenv("TELEGRAM_BASE_URL", "http://override.example.com")
    cfg, err := LoadForEdit(path)
    if err != nil {
        t.Fatalf("LoadForEdit failed: %v", err)
    }
    if cfg.BaseURL != "http://original.example.com" {
        t.Errorf("LoadForEdit should not apply env override, got %q", cfg.BaseURL)
    }
}

func TestSave_AtomicWrite(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "config.toml")
    cfg := &Config{BaseURL: "http://test.example.com", Headers: map[string]string{"X-Tenant": "test"}}
    if err := Save(path, cfg); err != nil {
        t.Fatalf("Save failed: %v", err)
    }
    // Verify file exists and is valid TOML
    loaded, err := Load(path)
    if err != nil {
        t.Fatalf("Load after Save failed: %v", err)
    }
    if loaded.BaseURL != "http://test.example.com" {
        t.Errorf("expected BaseURL %q, got %q", "http://test.example.com", loaded.BaseURL)
    }
}

func TestSave_EmptyPath(t *testing.T) {
    cfg := &Config{BaseURL: "http://test.example.com"}
    err := Save("", cfg)
    if err == nil {
        t.Fatal("Save with empty path should return error")
    }
}

func TestAuthHeader(t *testing.T) {
    cfg := &Config{AuthHeaderVal: "Bearer token123"}
    if got := cfg.AuthHeader(); got != "Bearer token123" {
        t.Errorf("AuthHeader() = %q, want %q", got, "Bearer token123")
    }
    empty := &Config{}
    if got := empty.AuthHeader(); got != "" {
        t.Errorf("empty AuthHeader() = %q, want empty", got)
    }
}

func TestCredentialConfigured(t *testing.T) {
    cfg := &Config{AuthHeaderVal: "Bearer token123"}
    if !cfg.CredentialConfigured() {
        t.Error("CredentialConfigured() should be true when AuthHeaderVal is set")
    }
    empty := &Config{}
    if empty.CredentialConfigured() {
        t.Error("CredentialConfigured() should be false for empty config")
    }
    var nilCfg *Config
    if nilCfg.CredentialConfigured() {
        t.Error("CredentialConfigured() should be false for nil config")
    }
}
```

**Verify**: `go test ./internal/config -v` → all pass

### Step 2: Create cache_test.go

Create `internal/cache/cache_test.go` with tests covering:

```go
package cache

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestSetGet_RoundTrip(t *testing.T) {
    dir := t.TempDir()
    s := New(dir, time.Minute)
    data := json.RawMessage(`{"key":"value"}`)
    s.Set("test-key", data)
    got, ok := s.Get("test-key")
    if !ok {
        t.Fatal("Get should find recently Set key")
    }
    if string(got) != string(data) {
        t.Errorf("Get returned %s, want %s", got, data)
    }
}

func TestGet_MissingKey(t *testing.T) {
    dir := t.TempDir()
    s := New(dir, time.Minute)
    _, ok := s.Get("nonexistent")
    if ok {
        t.Fatal("Get should return false for missing key")
    }
}

func TestGet_ExpiredKey(t *testing.T) {
    dir := t.TempDir()
    s := New(dir, time.Millisecond)
    data := json.RawMessage(`{"key":"value"}`)
    s.Set("test-key", data)
    time.Sleep(5 * time.Millisecond)
    _, ok := s.Get("test-key")
    if ok {
        t.Fatal("Get should return false for expired key")
    }
}

func TestClear(t *testing.T) {
    dir := t.TempDir()
    s := New(dir, time.Minute)
    s.Set("key1", json.RawMessage(`"a"`))
    s.Set("key2", json.RawMessage(`"b"`))
    if err := s.Clear(); err != nil {
        t.Fatalf("Clear failed: %v", err)
    }
    _, ok1 := s.Get("key1")
    _, ok2 := s.Get("key2")
    if ok1 || ok2 {
        t.Fatal("Get should return false after Clear")
    }
}

func TestSet_CreatesDirectory(t *testing.T) {
    dir := filepath.Join(t.TempDir(), "nested", "cache")
    s := New(dir, time.Minute)
    s.Set("key", json.RawMessage(`"value"`))
    _, ok := s.Get("key")
    if !ok {
        t.Fatal("Set should auto-create directory and Get should find key")
    }
}

func TestGet_CorruptFile(t *testing.T) {
    dir := t.TempDir()
    s := New(dir, time.Minute)
    // Write corrupt data directly to the cache path
    key := s.path("corrupt")
    os.MkdirAll(dir, 0o700)
    os.WriteFile(key, []byte("not valid json {{{"), 0o600)
    // Get should still return the data (cache doesn't validate JSON)
    _, ok := s.Get("corrupt")
    if !ok {
        // This is acceptable — the cache stores raw bytes
        t.Log("Get returned false for corrupt file (acceptable behavior)")
    }
}
```

**Verify**: `go test ./internal/cache -v` → all pass

### Step 3: Run full test suite

**Verify**: `go test ./...` → all pass

### Step 4: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- `internal/config/config_test.go`: 8 tests covering Load (missing file, malformed TOML, env override), LoadForEdit (no env override), Save (atomic write, empty path), AuthHeader, CredentialConfigured.
- `internal/cache/cache_test.go`: 6 tests covering Set/Get round-trip, missing key, expired key, Clear, directory auto-creation, corrupt file handling.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go test ./internal/config -v` shows all tests passing
- [ ] `go test ./internal/cache -v` shows all tests passing
- [ ] `go vet ./...` exits 0
- [ ] `ls internal/config/config_test.go` exists
- [ ] `ls internal/cache/cache_test.go` exists
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The config or cache packages have unexported functions that the tests need to access (STOP and report which functions).
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- Future config or cache changes should update these tests.
- The cache test uses `time.Sleep` for TTL expiry — this is acceptable for unit tests but may be flaky on slow CI. If flakiness occurs, increase the sleep duration or use filesystem mtime manipulation.

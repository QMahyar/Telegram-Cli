// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telegram-cli/internal/cliutil/testenv"
)

func resetPathEnv(t *testing.T) string {
	t.Helper()
	restore, err := SetHomeOverride("")
	if err != nil {
		t.Fatalf("reset home override: %v", err)
	}
	t.Cleanup(restore)
	return testenv.Isolate(t, ConfigDir, DataDir, StateDir, CacheDir)
}

func TestKindDirDefaultsLiveUnderDotTelegramCLI(t *testing.T) {
	home := resetPathEnv(t)

	tests := []struct {
		kind PathKind
		want string
	}{
		{PathKindConfig, filepath.Join(home, ".telegram-cli", "config")},
		{PathKindData, filepath.Join(home, ".telegram-cli", "data")},
		{PathKindState, filepath.Join(home, ".telegram-cli", "state")},
		{PathKindCache, filepath.Join(home, ".telegram-cli", "cache")},
	}
	for _, tt := range tests {
		got, err := KindDir(tt.kind)
		if err != nil {
			t.Fatalf("KindDir(%s) error = %v", kindName(tt.kind), err)
		}
		if got != tt.want {
			t.Fatalf("KindDir(%s) = %q, want %q", kindName(tt.kind), got, tt.want)
		}
	}
}

func TestKindDirHomeEnvUsesFlatKindLayout(t *testing.T) {
	resetPathEnv(t)
	root := filepath.Join(t.TempDir(), "persist")
	t.Setenv(envPrefix+"_HOME", root)

	tests := map[PathKind]string{
		PathKindConfig: filepath.Join(root, "config"),
		PathKindData:   filepath.Join(root, "data"),
		PathKindState:  filepath.Join(root, "state"),
		PathKindCache:  filepath.Join(root, "cache"),
	}
	for kind, want := range tests {
		got, err := KindDir(kind)
		if err != nil {
			t.Fatalf("KindDir(%s) error = %v", kindName(kind), err)
		}
		if got != want {
			t.Fatalf("KindDir(%s) = %q, want %q", kindName(kind), got, want)
		}
	}
}

func TestKindDirPerKindEnvBeatsHomeEnv(t *testing.T) {
	resetPathEnv(t)
	root := filepath.Join(t.TempDir(), "root")
	data := filepath.Join(t.TempDir(), "secure-data")
	t.Setenv(envPrefix+"_HOME", root)
	t.Setenv(envPrefix+"_DATA_DIR", data)

	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}
	if got != data {
		t.Fatalf("DataDir() = %q, want literal per-kind dir %q", got, data)
	}
	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if want := filepath.Join(root, "config"); configDir != want {
		t.Fatalf("ConfigDir() = %q, want %q", configDir, want)
	}
}

func TestKindDirXDGAddsAppName(t *testing.T) {
	resetPathEnv(t)
	xdg := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdg)

	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}
	if want := filepath.Join(xdg, appName); got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestKindDirDataPrecedencePairs(t *testing.T) {
	home := resetPathEnv(t)
	perKind := filepath.Join(t.TempDir(), "per-kind")
	flagHome := filepath.Join(t.TempDir(), "flag-home")
	envHome := filepath.Join(t.TempDir(), "env-home")
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv(envPrefix+"_DATA_DIR", perKind)
	t.Setenv(envPrefix+"_HOME", envHome)
	t.Setenv("XDG_DATA_HOME", xdg)
	restore, err := SetHomeOverride(flagHome)
	if err != nil {
		t.Fatalf("SetHomeOverride() error = %v", err)
	}
	defer restore()

	assertDataDir(t, perKind)
	t.Setenv(envPrefix+"_DATA_DIR", "")
	assertDataDir(t, filepath.Join(flagHome, "data"))
	restore()
	assertDataDir(t, filepath.Join(envHome, "data"))
	t.Setenv(envPrefix+"_HOME", "")
	assertDataDir(t, filepath.Join(xdg, appName))
	t.Setenv("XDG_DATA_HOME", "")
	assertDataDir(t, filepath.Join(home, ".telegram-cli", "data"))
}

func TestKindDirRelativeOverridesWarnAndFallThrough(t *testing.T) {
	home := resetPathEnv(t)
	t.Setenv(envPrefix+"_HOME", "relative/home")
	t.Setenv("XDG_DATA_HOME", "relative/xdg")

	stderr := captureStderr(t, func() {
		got, err := DataDir()
		if err != nil {
			t.Fatalf("DataDir() error = %v", err)
		}
		if want := filepath.Join(home, ".telegram-cli", "data"); got != want {
			t.Fatalf("DataDir() = %q, want %q", got, want)
		}
	})
	for _, want := range []string{envPrefix + "_HOME", "relative/home", "XDG_DATA_HOME", "relative/xdg"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr %q does not mention %q", stderr, want)
		}
	}
}

func TestSetHomeOverrideRejectsRelative(t *testing.T) {
	resetPathEnv(t)
	if _, err := SetHomeOverride("../elsewhere"); err == nil || !strings.Contains(err.Error(), "--home") {
		t.Fatalf("SetHomeOverride(relative) error = %v, want --home absolute-path error", err)
	}
}

func TestSetHomeOverrideRejectsRegularFile(t *testing.T) {
	resetPathEnv(t)
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("file"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := SetHomeOverride(path); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("SetHomeOverride(file) error = %v, want not-a-directory error", err)
	}
}

func TestSetHomeOverrideExpandsTildeAndCleans(t *testing.T) {
	home := resetPathEnv(t)
	restore, err := SetHomeOverride("~/state/../root")
	if err != nil {
		t.Fatalf("SetHomeOverride() error = %v", err)
	}
	defer restore()
	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	if want := filepath.Join(home, "root", "cache"); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
}

func TestHomeOverrideWinsForDefaultBaseAndTildeExpansion(t *testing.T) {
	resetPathEnv(t)
	override := t.TempDir()
	restore, err := SetHomeOverride(override)
	if err != nil {
		t.Fatalf("SetHomeOverride() error = %v", err)
	}
	defer restore()

	if got, want := expandTilde("~/nested"), filepath.Join(override, "nested"); got != want {
		t.Fatalf("expandTilde() = %q, want %q", got, want)
	}
	if got, err := defaultBase(PathKindData); err != nil {
		t.Fatalf("defaultBase() error = %v", err)
	} else if want := filepath.Join(override, ".telegram-cli", "data"); got != want {
		t.Fatalf("defaultBase() = %q, want %q", got, want)
	}
}

func assertDataDir(t *testing.T, want string) {
	t.Helper()
	got, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}
	if got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestMigrateLegacyLayoutMovesXDGScatterIntoSingleRoot(t *testing.T) {
	home := resetPathEnv(t)

	// Seed legacy XDG-scattered dirs with recognizable files.
	legacyConfig := filepath.Join(home, ".config", appName)
	legacyData := filepath.Join(home, ".local", "share", appName)
	legacyState := filepath.Join(home, ".local", "state", appName)
	legacyCache := filepath.Join(home, ".cache", appName)
	for dir, file := range map[string]string{
		legacyConfig: "config.toml",
		legacyData:   "telegram.db",
		legacyState:  "state.bin",
		legacyCache:  "cache.bin",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o600); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	if err := MigrateLegacyLayout(); err != nil {
		t.Fatalf("MigrateLegacyLayout() error = %v", err)
	}

	want := map[PathKind]string{
		PathKindConfig: filepath.Join(home, ".telegram-cli", "config"),
		PathKindData:   filepath.Join(home, ".telegram-cli", "data"),
		PathKindState:  filepath.Join(home, ".telegram-cli", "state"),
		PathKindCache:  filepath.Join(home, ".telegram-cli", "cache"),
	}
	for kind, dir := range want {
		got, err := KindDir(kind)
		if err != nil {
			t.Fatalf("KindDir(%s) error = %v", kindName(kind), err)
		}
		if got != dir {
			t.Fatalf("KindDir(%s) = %q, want %q", kindName(kind), got, dir)
		}
		// Each seeded marker file must have moved along with its dir.
		var marker string
		switch kind {
		case PathKindConfig:
			marker = "config.toml"
		case PathKindData:
			marker = "telegram.db"
		case PathKindState:
			marker = "state.bin"
		case PathKindCache:
			marker = "cache.bin"
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
			t.Fatalf("marker %s not migrated into %s: %v", marker, dir, err)
		}
		if _, err := os.Stat(legacyBaseFor(home, kind)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy dir for %s still present after migration", kindName(kind))
		}
	}

	// Idempotent: a second run must be a no-op (nothing to move).
	if err := MigrateLegacyLayout(); err != nil {
		t.Fatalf("second MigrateLegacyLayout() error = %v", err)
	}
}

func TestMigrateLegacyLayoutSkipsWhenOverrideInPlay(t *testing.T) {
	resetPathEnv(t)
	legacy := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv(envPrefix+"_DATA_DIR", filepath.Join(t.TempDir(), "explicit"))
	if err := MigrateLegacyLayout(); err != nil {
		t.Fatalf("MigrateLegacyLayout() error = %v", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy dir touched despite env override: %v", err)
	}
}

func legacyBaseFor(home string, kind PathKind) string {
	switch kind {
	case PathKindConfig:
		return filepath.Join(home, ".config", appName)
	case PathKindData:
		return filepath.Join(home, ".local", "share", appName)
	case PathKindState:
		return filepath.Join(home, ".local", "state", appName)
	case PathKindCache:
		return filepath.Join(home, ".cache", appName)
	default:
		return ""
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

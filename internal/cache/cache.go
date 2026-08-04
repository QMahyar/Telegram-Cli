// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

// Package cache provides a file-based response cache with optional embedded database backend.
// The default implementation stores JSON responses as flat files in the CLI cache directory with a TTL.
// For higher-throughput or concurrent-write scenarios, replace the file backend with an
// embedded database such as bolt (go.etcd.io/bbolt), badger (github.com/dgraph-io/badger),
// or sqlite (modernc.org/sqlite).
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Store is a key-value cache backed by the filesystem.
type Store struct {
	Dir string
	TTL time.Duration
}

// New creates a file-based cache store.
func New(dir string, ttl time.Duration) *Store {
	return &Store{Dir: dir, TTL: ttl}
}

// Get retrieves a cached value. Returns nil if not found or expired.
func (s *Store) Get(key string) (json.RawMessage, bool) {
	path := s.path(key)
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > s.TTL {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- app-derived cache path from sha256 cache key.
	if err != nil {
		return nil, false
	}
	return json.RawMessage(data), true
}

// Set stores a value in the cache.
//
// The write is atomic: the payload is written to a temp file in the same
// directory and renamed over the final path, so a crash mid-write can never
// leave a truncated JSON blob that a later Get() would serve (Get only checks
// the file's mtime for freshness).
func (s *Store) Set(key string, value json.RawMessage) {
	path := s.path(key)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(s.Dir, "cache-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	// no-op after a successful rename; best-effort cleanup on failure paths.
	defer os.Remove(tmpName)
	if _, err := tmp.Write(value); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Chmod(tmpName, 0o600)
	_ = os.Rename(tmpName, path)
}

// Clear removes all cached entries.
func (s *Store) Clear() error {
	return os.RemoveAll(s.Dir)
}

func (s *Store) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(s.Dir, hex.EncodeToString(h[:8])+".json")
}

package cache

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 5*time.Minute)
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.Dir != dir {
		t.Errorf("Dir = %q, want %q", s.Dir, dir)
	}
}

func TestSet_And_Get(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 5*time.Minute)

	key := "test-key"
	value := json.RawMessage(`{"hello":"world"}`)

	s.Set(key, value)

	got, ok := s.Get(key)
	if !ok {
		t.Fatal("Get returned false for existing key")
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %s, want %s", got, value)
	}
}

func TestGet_Expired(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 1*time.Millisecond) // very short TTL

	key := "expire-key"
	value := json.RawMessage(`{"expire":"test"}`)

	s.Set(key, value)
	time.Sleep(10 * time.Millisecond) // ensure expiry

	got, ok := s.Get(key)
	if ok {
		t.Errorf("Get() should return false for expired key, got %v", got)
	}
}

func TestGet_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 5*time.Minute)

	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get() should return false for nonexistent key")
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, 5*time.Minute)

	s.Set("key1", json.RawMessage(`{"a":1}`))
	s.Set("key2", json.RawMessage(`{"b":2}`))

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	_, ok1 := s.Get("key1")
	_, ok2 := s.Get("key2")
	if ok1 || ok2 {
		t.Error("Get() should return false after Clear()")
	}
}

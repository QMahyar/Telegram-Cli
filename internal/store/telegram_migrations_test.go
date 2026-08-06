package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEnsureTelegramSchema_FreshDB(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("EnsureTelegramSchema failed: %v", err)
	}

	tables := []string{
		"tg_accounts", "tg_dialogs", "tg_messages", "tg_peers",
		"tg_jobs", "tg_job_results", "tg_templates",
	}
	for _, table := range tables {
		var count int
		err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&count)
		if err != nil {
			t.Errorf("checking table %s: %v", table, err)
		}
		if count == 0 {
			t.Errorf("table %s not found after EnsureTelegramSchema", table)
		}
	}
}

func TestEnsureTelegramSchema_Idempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("first EnsureTelegramSchema failed: %v", err)
	}
	if err := EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("second EnsureTelegramSchema failed: %v", err)
	}
}

func TestEnsureTelegramSchema_FTS5Trigger(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("EnsureTelegramSchema failed: %v", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tg_messages (account, peer_type, peer_id, msg_id, sender, text, date)
		 VALUES ('test', 'user', 123, 1, 'sender', 'hello world', datetime('now'))`,
	)
	if err != nil {
		t.Fatalf("insert into tg_messages failed: %v", err)
	}

	var count int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tg_messages_fts WHERE tg_messages_fts MATCH ?`,
		"hello",
	).Scan(&count)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 FTS match, got %d", count)
	}
}

func TestEnsureTelegramSchema_TgAccounts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer s.Close()

	if err := EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("EnsureTelegramSchema failed: %v", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tg_accounts (alias, user_id, username, phone, session_dir, status)
		 VALUES ('testuser', 12345, 'testphone', '+1234567890', '/tmp/test', 'active')`,
	)
	if err != nil {
		t.Fatalf("insert into tg_accounts failed: %v", err)
	}

	var alias string
	err = s.db.QueryRowContext(ctx,
		`SELECT alias FROM tg_accounts WHERE user_id = ?`, 12345,
	).Scan(&alias)
	if err != nil {
		t.Fatalf("query tg_accounts failed: %v", err)
	}
	if alias != "testuser" {
		t.Errorf("expected alias 'testuser', got %q", alias)
	}
}

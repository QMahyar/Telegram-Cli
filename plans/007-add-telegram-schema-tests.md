# Plan 007: Add tests for Telegram schema migrations

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/store/telegram_migrations.go`
> If the in-scope file changed since this plan was written, compare the
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

`EnsureTelegramSchema` creates 12 Telegram-specific tables with FTS5 virtual tables, triggers, and composite primary keys. It is called by every mirror command (sync, search, stats, digest, since, inbox). The base store migrations (v1-v9) have thorough test coverage in `schema_version_test.go`, but the Telegram-specific schema has zero dedicated tests. A migration syntax error on a specific SQLite version would break all mirror commands.

## Current state

- `internal/store/telegram_migrations.go` — `EnsureTelegramSchema` creates tables: `tg_accounts`, `tg_dialogs`, `tg_messages`, `tg_peers`, `tg_jobs`, `tg_job_results`, `tg_templates`, plus FTS5 virtual tables and triggers.
- `internal/store/schema_version_test.go` — tests base migrations extensively but does NOT test `EnsureTelegramSchema`.
- `internal/store/store.go` — `Store` struct with `Open` and `OpenWithContext`.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/store/telegram_migrations_test.go` (new file)

**Out of scope**:
- Changes to `telegram_migrations.go` production code.
- Changes to the base store migration tests.

## Git workflow

- Branch: `advisor/007-add-telegram-schema-tests`
- Commit: `test: add tests for EnsureTelegramSchema and FTS5 triggers`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Create telegram_migrations_test.go

Create `internal/store/telegram_migrations_test.go` with tests covering:

```go
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

    // Verify all expected tables exist
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

    // Run twice — should not error
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

    // Insert a message and verify FTS trigger fires
    _, err = s.db.ExecContext(ctx,
        `INSERT INTO tg_messages (account, peer_type, peer_id, msg_id, sender, text, date)
         VALUES ('test', 'user', 123, 1, 'sender', 'hello world', datetime('now'))`,
    )
    if err != nil {
        t.Fatalf("insert into tg_messages failed: %v", err)
    }

    // Search via FTS — should find the message
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

    // Insert and read back an account
    _, err = s.db.ExecContext(ctx,
        `INSERT INTO tg_accounts (alias, user_id, username, phone, status)
         VALUES ('testuser', 12345, 'testphone', '+1234567890', 'active')`,
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
```

**Verify**: `go test ./internal/store -run TestEnsureTelegramSchema -v` → all pass

### Step 2: Run full test suite

**Verify**: `go test ./...` → all pass

### Step 3: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- `internal/store/telegram_migrations_test.go`: 4 tests covering fresh DB table creation, idempotency, FTS5 trigger functionality, and tg_accounts insert/query.
- Existing test suite continues passing.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go test ./internal/store -run TestEnsureTelegramSchema` passes all 4 tests
- [ ] `go vet ./...` exits 0
- [ ] `ls internal/store/telegram_migrations_test.go` exists
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- `EnsureTelegramSchema` has unexported dependencies that the tests cannot access.
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- If new tables are added to `EnsureTelegramSchema`, add corresponding verification to `TestEnsureTelegramSchema_FreshDB`.
- The FTS5 trigger test is the most valuable — it catches SQLite version compatibility issues.

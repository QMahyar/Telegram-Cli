# Plan 010: Add behavioral tests for CLI commands

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/*_test.go`
> If the in-scope files changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MEDIUM
- **Depends on**: Plan 001 (nil pointer fix)
- **Category**: tests
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

Current test coverage for batch, broadcast, inbox, digest, since, stats, daemon run, and schema check commands only validates that `--help` exits cleanly. These tests catch broken help text but miss broken command logic, flag parsing errors, and missing required flags. The user requested adding behavioral tests alongside smoke tests.

## Current state

Existing test files (smoke-only):
- `internal/cli/batch_test.go` — `TestBatchCommand_Help`
- `internal/cli/broadcast_test.go` — `TestBroadcastCommand_Help`
- `internal/cli/inbox_test.go` — `TestInboxCommand_Help`
- `internal/cli/digest_test.go` — `TestDigestCommand_Help`
- `internal/cli/since_test.go` — `TestSinceCommand_Help`
- `internal/cli/stats_test.go` — `TestStatsCommand_Help`
- `internal/cli/daemon_run_test.go` — `TestDaemonRunCommand_Help`
- `internal/cli/schema_check_test.go` — `TestSchemaCheckCommand_Help`
- `internal/cli/accounts_health_test.go` — `TestAccountsHealthCommand_Help`
- `internal/cli/jobs_test.go` — `TestJobsCommand_Help`

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- All `*_test.go` files listed above (add behavioral tests to existing files)

**Out of scope**:
- Creating new test files.
- Testing actual Telegram API calls (requires credentials).
- Testing database operations (covered by store tests).

## Git workflow

- Branch: `advisor/010-add-command-behavioral-tests`
- Commit: `test(cli): add behavioral tests for command flag parsing and error paths`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add behavioral tests to batch_test.go

Append to `internal/cli/batch_test.go`:

```go
func TestBatchCommand_MissingAccount(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"batch"})
    err := cmd.Execute()
    // Should fail with missing account argument
    if err == nil {
        t.Error("expected error for missing account")
    }
}

func TestBatchCommand_InvalidFlag(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"batch", "--nonexistent-flag"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error for invalid flag")
    }
}
```

**Verify**: `go test ./internal/cli -run TestBatch -v` → passes

### Step 2: Add behavioral tests to broadcast_test.go

Append to `internal/cli/broadcast_test.go`:

```go
func TestBroadcastCommand_MissingAccount(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"broadcast"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error for missing account")
    }
}

func TestBroadcastCommand_InvalidFlag(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"broadcast", "--nonexistent-flag"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error for invalid flag")
    }
}
```

**Verify**: `go test ./internal/cli -run TestBroadcast -v` → passes

### Step 3: Add behavioral tests to inbox_test.go

Append to `internal/cli/inbox_test.go`:

```go
func TestInboxCommand_MissingAccount(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"inbox"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error for missing account")
    }
}

func TestInboxCommand_InvalidFlag(t *testing.T) {
    cmd := newRootCmd()
    cmd.SetArgs([]string{"inbox", "--nonexistent-flag"})
    err := cmd.Execute()
    if err == nil {
        t.Error("expected error for invalid flag")
    }
}
```

**Verify**: `go test ./internal/cli -run TestInbox -v` → passes

### Step 4: Add behavioral tests to remaining test files

Append similar tests to `digest_test.go`, `since_test.go`, `stats_test.go`, `daemon_run_test.go`, and `schema_check_test.go`.

**Verify**: `go test ./internal/cli -v` → all pass

### Step 5: Run full test suite

**Verify**: `go test ./...` → all pass

### Step 6: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- Each command gets 2 new tests: missing required argument + invalid flag.
- Existing `--help` smoke tests remain untouched.
- Total: ~16 new test functions across 8 files.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `go test ./internal/cli -v` shows tests with `MissingAccount` and `InvalidFlag` in the output
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- `newRootCmd()` is not accessible from test files (different package).
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- When new commands are added, follow the pattern: add a `_Help` smoke test + `MissingAccount` + `InvalidFlag` behavioral tests.
- These tests don't require Telegram credentials or a database — they test flag parsing and validation only.

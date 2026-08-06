# Plan 001: Fix nil pointer dereference in broadcast INSERT and constant RandomID in media upload

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/novel_commands.go internal/mtproto/media.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

Two bugs in the Telegram MTProto surface can cause data loss or silent failures:

1. **Nil pointer panic**: The `broadcast` command discards the error from `ExecContext` and calls `res.LastInsertId()` on a potentially nil `res`. If the INSERT fails (disk full, table missing, constraint violation), the CLI panics instead of returning an error.

2. **Message deduplication**: `UploadAndSendMedia` uses a constant `RandomID: 0`. Telegram uses `random_id` to deduplicate messages — sending two different media files to the same peer with `RandomID: 0` causes the second to be silently dropped. The sibling function `SendMessageWithOptions` at `ops.go:122` correctly uses `rand.Int63()`.

## Current state

- `internal/cli/novel_commands.go:104-108` — broadcast INSERT ignores error:
  ```go
  res, _ := s.DB().ExecContext(ctx,
      `INSERT INTO tg_jobs (kind, status, accounts_csv, targets_csv, text, media_path, at) VALUES ('broadcast', ?, ?, ?, ?, ?, ?)`,
      status, alias, targets, text, mediaPath, scheduledAt,
  )
  jobID, _ := res.LastInsertId()
  ```

- `internal/mtproto/media.go:104` — constant RandomID:
  ```go
  rnd := int64(0)
  resp, err := api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
      Peer:     peer,
      Media:    inputMedia,
      Message:  caption,
      RandomID: rnd,
  })
  ```

- `internal/mtproto/ops.go:122` — correct pattern (uses `rand.Int63()`):
  ```go
  rnd := rand.Int63()
  ```

- Conventions: errors are returned as `fmt.Errorf("context: %w", err)`. The `rand` package is already imported in `ops.go`.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/cli/novel_commands.go` (broadcast INSERT error handling)
- `internal/mtproto/media.go` (RandomID fix)

**Out of scope**:
- Other `LastInsertId()` call sites in `playbooks.go` (separate finding, lower priority)
- The `batch forward` INSERT at `novel_commands.go:275-283` (already checks `err`, just ignores `LastInsertId` error — lower risk)

## Git workflow

- Branch: `advisor/001-fix-nil-pointer-randomid`
- Commit per logical unit: one for the nil-pointer fix, one for the RandomID fix
- Commit message style: conventional commits (e.g. `fix(cli): check INSERT error before LastInsertId in broadcast`)
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Fix nil pointer dereference in broadcast INSERT

In `internal/cli/novel_commands.go`, replace lines 104-108:

**Before:**
```go
res, _ := s.DB().ExecContext(ctx,
    `INSERT INTO tg_jobs (kind, status, accounts_csv, targets_csv, text, media_path, at) VALUES ('broadcast', ?, ?, ?, ?, ?, ?)`,
    status, alias, targets, text, mediaPath, scheduledAt,
)
jobID, _ := res.LastInsertId()
```

**After:**
```go
res, err := s.DB().ExecContext(ctx,
    `INSERT INTO tg_jobs (kind, status, accounts_csv, targets_csv, text, media_path, at) VALUES ('broadcast', ?, ?, ?, ?, ?, ?)`,
    status, alias, targets, text, mediaPath, scheduledAt,
)
if err != nil {
    return fmt.Errorf("recording broadcast job: %w", err)
}
jobID, err := res.LastInsertId()
if err != nil {
    return fmt.Errorf("getting broadcast job ID: %w", err)
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 2: Fix constant RandomID in UploadAndSendMedia

In `internal/mtproto/media.go`, add `"math/rand"` to the import block if not already present, then change line 104:

**Before:**
```go
rnd := int64(0)
```

**After:**
```go
rnd := rand.Int63()
```

**Verify**: `go build ./internal/mtproto` → exit 0

### Step 3: Run full test suite

**Verify**: `go test ./...` → all pass (cached or fresh)

### Step 4: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- No new tests needed — these are one-line bug fixes in existing code paths.
- The existing test suite should continue passing.
- Manual verification: `go build ./...` confirms the code compiles.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -n "rnd := int64(0)" internal/mtproto/media.go` returns no matches
- [ ] `grep -n "res, _ := s.DB().ExecContext" internal/cli/novel_commands.go` returns no matches (at the broadcast INSERT site)
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts (the codebase has drifted since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file.
- You discover the assumption "rand.Int63() is the correct random ID pattern" is false (e.g. the codebase has migrated to a different RNG).

## Maintenance notes

- If the `batch forward` INSERT at `novel_commands.go:275` is ever made non-optional, the same error-handling pattern should be applied there too.
- The `rand` package usage is consistent with `ops.go` — no need for `crypto/rand` since Telegram's `random_id` is for deduplication, not security.

# Plan 008: Fix duplicate error check in digest command

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/novel_commands.go`
> If the in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

The `digest` command checks the same `err` variable twice in immediate succession. The second check is unreachable dead code. While harmless, it indicates a copy-paste error and makes the code harder to read.

## Current state

- `internal/cli/novel_commands.go:979-984`:
  ```go
  rows, err := s.DB().QueryContext(ctx, ...)
  if err != nil {       // line 979
      return err
  }
  if err != nil {       // line 982 — identical check, dead code
      return err
  }
  defer rows.Close()
  ```

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/cli/novel_commands.go` (remove duplicate error check)

**Out of scope**:
- Other error handling patterns in the file.
- The `LastInsertId()` error suppression at line 283 (separate finding).

## Git workflow

- Branch: `advisor/008-fix-duplicate-error-check`
- Commit: `fix(cli): remove duplicate error check in digest command`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Remove duplicate error check

In `internal/cli/novel_commands.go`, remove lines 982-984 (the second `if err != nil` block):

**Before:**
```go
if err != nil {
    return err
}
if err != nil {
    return err
}
defer rows.Close()
```

**After:**
```go
if err != nil {
    return err
}
defer rows.Close()
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 2: Run tests

**Verify**: `go test ./...` → all pass

### Step 3: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- No new tests needed — this is removing dead code.
- The existing test suite validates that the digest command still works.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -c "if err != nil" internal/cli/novel_commands.go` shows one fewer occurrence than before
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- The remaining `if err != nil` checks in the file are legitimate. Only the duplicate was removed.

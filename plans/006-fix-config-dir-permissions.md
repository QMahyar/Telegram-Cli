# Plan 006: Fix config directory permissions from 0o755 to 0o700

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/config/config.go`
> If the in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

The `Save` function in `config.go` creates the config directory with `0o755` permissions (group/other-readable), while the config file itself is hardened to `0o600`. On shared systems, other local users can list the directory contents (filenames like `config.toml` are visible), even though they can't read the file. The cache and session directories already use `0o700` — the config directory should match.

## Current state

- `internal/config/config.go:98`:
  ```go
  if err := os.MkdirAll(dir, 0o755); err != nil {
  ```

- Other directories in the codebase use `0o700`:
  - `internal/cache/cache.go:52`: `os.MkdirAll(s.Dir, 0o700)`
  - `internal/cliutil/paths.go:387`: `os.MkdirAll(filepath.Dir(res.Dir), 0o700)`
  - `internal/store/store.go:138`: `os.MkdirAll(filepath.Dir(dbPath), 0o700)`

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/config/config.go` (change `0o755` to `0o700`)

**Out of scope**:
- Changes to other directory creation calls.
- Permission hardening on existing installations (this only affects new directory creation).

## Git workflow

- Branch: `advisor/006-fix-config-dir-perms`
- Commit: `fix(config): use owner-only permissions for config directory`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Change directory permissions

In `internal/config/config.go`, line 98:

**Before:**
```go
if err := os.MkdirAll(dir, 0o755); err != nil {
```

**After:**
```go
if err := os.MkdirAll(dir, 0o700); err != nil {
```

**Verify**: `go build ./internal/config` → exit 0

### Step 2: Run tests

**Verify**: `go test ./...` → all pass

### Step 3: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- No new tests needed — this is a one-character permission change.
- The existing test suite validates that config operations still work.
- The test from Plan 005 (`TestSave_AtomicWrite`) validates that config saving works after this change.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -n "0o755" internal/config/config.go` returns no matches
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- This change only affects new directory creation. Existing config directories with `0o755` will retain their permissions. Users can manually `chmod 700 ~/.telegram-cli/config` if needed.
- The `doctor` command could be extended to check and warn about overly-permissive directory permissions in a follow-up.

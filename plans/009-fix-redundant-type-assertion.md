# Plan 009: Fix redundant type assertion in watchMediaType

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/watch.go`
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

The `watchMediaType` function has a redundant inner type assertion. After matching `*tg.MessageMediaDocument` in the `case` clause, it immediately re-asserts the same type with `if _, ok := m.Media.(*tg.MessageMediaDocument); ok` — which is always true. The `if` wrapper is dead code.

## Current state

- `internal/cli/watch.go:240-250`:
  ```go
  func watchMediaType(m *tg.Message) string {
      switch m.Media.(type) {
      case *tg.MessageMediaPhoto:
          return "photo"
      case *tg.MessageMediaDocument:
          if _, ok := m.Media.(*tg.MessageMediaDocument); ok {
              return "document"
          }
      }
      return ""
  }
  ```

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/cli/watch.go` (simplify watchMediaType)

**Out of scope**:
- Other functions in watch.go.
- Changes to the media type detection logic.

## Git workflow

- Branch: `advisor/009-fix-redundant-type-assertion`
- Commit: `fix(cli): remove redundant type assertion in watchMediaType`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Simplify watchMediaType

In `internal/cli/watch.go`, replace the function:

**Before:**
```go
func watchMediaType(m *tg.Message) string {
    switch m.Media.(type) {
    case *tg.MessageMediaPhoto:
        return "photo"
    case *tg.MessageMediaDocument:
        if _, ok := m.Media.(*tg.MessageMediaDocument); ok {
            return "document"
        }
    }
    return ""
}
```

**After:**
```go
func watchMediaType(m *tg.Message) string {
    switch m.Media.(type) {
    case *tg.MessageMediaPhoto:
        return "photo"
    case *tg.MessageMediaDocument:
        return "document"
    }
    return ""
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 2: Run tests

**Verify**: `go test ./...` → all pass

### Step 3: Run vet

**Verify**: `go vet ./...` → exit 0

## Test plan

- No new tests needed — this is simplifying existing logic.
- The existing test suite validates that the watch command still works.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -n "if _, ok := m.Media" internal/cli/watch.go` returns no matches
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- The function now has a clean switch statement. Future media types can be added as additional `case` clauses.

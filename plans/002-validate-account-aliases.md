# Plan 002: Validate account aliases against path traversal

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/accounts.go internal/mtproto/mtproto.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

Account aliases flow from CLI arguments directly into `filepath.Join` for session directory operations (`accounts add`, `accounts remove`, `accounts rename`, `accounts import`). An alias like `../../.ssh` or `../../config/telegram-cli/config.toml` would cause `os.RemoveAll` to delete directories outside the intended sessions tree, or `os.Rename` to move session files to attacker-chosen paths. This is a path-traversal vulnerability via CLI argument.

## Current state

- `internal/mtproto/mtproto.go:67` — SessionDir builds path from unsanitized alias:
  ```go
  func (m *Manager) SessionDir(alias string) string {
      return filepath.Join(m.Home, "sessions", alias)
  }
  ```

- `internal/cli/accounts.go:346` — os.RemoveAll with unsanitized alias:
  ```go
  os.RemoveAll(filepath.Join(home, "sessions", alias))
  ```

- `internal/cli/accounts.go:303-308` — os.Rename with unsanitized newAlias:
  ```go
  oldDir := filepath.Join(home, "sessions", oldAlias)
  newDir := filepath.Join(home, "sessions", newAlias)
  os.Rename(oldDir, newDir)
  ```

- No existing alias validation function exists anywhere in the codebase.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/cli/accounts.go` (add alias validation in accounts add, remove, rename, import)
- `internal/cli/telegram_helpers.go` (add shared `validateAlias` function)

**Out of scope**:
- `internal/mtproto/mtproto.go` — the `SessionDir` function is a low-level utility; validation belongs at the CLI boundary where user input arrives, not in the MTProto layer.
- Any changes to the session storage format.

## Git workflow

- Branch: `advisor/002-validate-aliases`
- Commit: `fix(cli): reject account aliases containing path separators or dots`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add validateAlias function

In `internal/cli/telegram_helpers.go`, add a new function:

```go
// validateAlias rejects account aliases that could cause path traversal
// or filesystem confusion. Aliases must be non-empty, contain only
// alphanumeric characters, hyphens, underscores, and dots, and be at
// most 64 characters long. Leading/trailing dots and control characters
// are rejected.
func validateAlias(alias string) error {
    alias = strings.TrimSpace(alias)
    if alias == "" {
        return usageErr(fmt.Errorf("account alias cannot be empty"))
    }
    if len(alias) > 64 {
        return usageErr(fmt.Errorf("account alias %q exceeds 64 characters", alias))
    }
    if strings.HasPrefix(alias, ".") || strings.HasSuffix(alias, ".") {
        return usageErr(fmt.Errorf("account alias %q must not start or end with a dot", alias))
    }
    for _, r := range alias {
        if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
            continue
        }
        return usageErr(fmt.Errorf("account alias %q contains invalid character %q; only [a-zA-Z0-9._-] are allowed", alias, r))
    }
    return nil
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 2: Wire validateAlias into accounts add

In `internal/cli/accounts.go`, find the `newAccountsAddCmd` function. After the alias argument is parsed (after `cobra.ExactArgs(1)` resolves), add:

```go
if err := validateAlias(args[0]); err != nil {
    return err
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 3: Wire validateAlias into accounts remove

In `internal/cli/accounts.go`, find the `newAccountsRemoveCmd` function. After the alias argument is parsed, add:

```go
if err := validateAlias(args[0]); err != nil {
    return err
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 4: Wire validateAlias into accounts rename

In `internal/cli/accounts.go`, find the `newAccountsRenameCmd` function. After both old and new alias arguments are parsed, add:

```go
if err := validateAlias(args[0]); err != nil {
    return err
}
if err := validateAlias(args[1]); err != nil {
    return err
}
```

**Verify**: `go build ./internal/cli` → exit 0

### Step 5: Wire validateAlias into accounts import

In `internal/cli/accounts.go`, find the accounts import command handler. After the alias argument is parsed, add:

```go
if err := validateAlias(alias); err != nil {
    return err
}
```

Note: the import command may use a named variable for the alias rather than `args[0]`. Check the actual code and adapt.

**Verify**: `go build ./internal/cli` → exit 0

### Step 6: Add tests for validateAlias

In `internal/cli/telegram_helpers_test.go` (or create a new file if it doesn't exist), add tests:

```go
func TestValidateAlias(t *testing.T) {
    tests := []struct {
        name    string
        alias   string
        wantErr bool
    }{
        {"valid simple", "work", false},
        {"valid with dots", "my.account", false},
        {"valid with hyphens", "my-account", false},
        {"valid with underscores", "my_account", false},
        {"valid numeric", "123", false},
        {"empty", "", true},
        {"path traversal dotdot", "../../.ssh", true},
        {"absolute path", "/etc/passwd", true},
        {"leading dot", ".hidden", true},
        {"trailing dot", "hidden.", true},
        {"space", "my account", true},
        {"slash", "my/account", true},
        {"backslash", `my\account`, true},
        {"too long", strings.Repeat("a", 65), true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateAlias(tt.alias)
            if (err != nil) != tt.wantErr {
                t.Errorf("validateAlias(%q) error = %v, wantErr %v", tt.alias, err, tt.wantErr)
            }
        })
    }
}
```

**Verify**: `go test ./internal/cli -run TestValidateAlias -v` → all pass

### Step 7: Run full test suite and vet

**Verify**: `go test ./...` → all pass
**Verify**: `go vet ./...` → exit 0

## Test plan

- New tests in `internal/cli/telegram_helpers_test.go` covering valid aliases, path traversal attempts, invalid characters, and length limits.
- Existing test suite continues passing.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `go test ./internal/cli -run TestValidateAlias` passes all cases
- [ ] `grep -n "validateAlias" internal/cli/accounts.go` shows calls in add, remove, rename, import commands
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file.
- The accounts import command uses a different alias variable name than expected.

## Maintenance notes

- Future commands that accept account aliases should call `validateAlias` before using the value.
- The regex pattern `^[a-zA-Z0-9._-]{1,64}$` is intentionally strict — Telegram usernames have their own constraints, but the CLI alias is a local identifier and should be safe for filesystem operations.

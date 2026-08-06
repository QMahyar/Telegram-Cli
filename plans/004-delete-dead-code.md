# Plan 004: Delete dead pagination framework and makeAPIHandler

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- internal/cli/helpers.go internal/mcp/tools.go .golangci.yml`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

~420 lines of dead code from a generator that the repo absorbed still ship in the binary. The `.golangci.yml` excludes `helpers.go` and `tools.go` from lint entirely, meaning real bugs in the remaining live code in those files are never caught by the linter. The AGENTS.md contract says "when the generator is retired, delete these entries and remove the dead symbols."

## Current state

- `.golangci.yml:30-31` — lint exclusions:
  ```yaml
  exclude-files:
    - helpers\.go$
    - tools\.go$
  ```

- `internal/cli/helpers.go:436-642` — dead pagination framework: `paginatedGet`, `paginatedGetWithResponsePath`, `responsePathPaginatedClient`, `extractPaginatedItems`, `extractPaginatedItemsFromObject`, `extractPaginatedItemsMatchingPath`, `extractPaginatedObjectArray`, `paginatedCollectionEnvelopeField`, `nextFullPageOffsetCursor`, `nextClientSidePaginationCursor`, `emitTruncationWarning`, `emitMissingPaginationSignalWarning`, `emitPaginatedGetMaxPagesWarning`, `emitMissingPaginationCursorWarning`, `paginationCursorToken`, `cloneRawObject`, `deleteRawPath`, `rawAtPath`, `applyResponsePath`, `responsePayloadAtPath`, `responsePayloadParentAtPath`, `paginatedGetMaxPages` constant.

- `internal/mcp/tools.go:122-341` — dead `makeAPIHandler`, `mcpParamBinding`, `mcpPageConfig` types.

- `internal/cli/export.go:96-97` — only reference to `paginatedGetMaxPages` (the constant). Check if it's actually used or just declared.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `internal/cli/helpers.go` (delete dead pagination functions)
- `internal/mcp/tools.go` (delete dead makeAPIHandler and types)
- `.golangci.yml` (remove exclude-files entries)
- `internal/cli/export.go` (check and remove paginatedGetMaxPages reference if dead)

**Out of scope**:
- Live output formatting functions in helpers.go (filterFields, compactFields, printCSV, etc.)
- Live MCP tool registration in tools.go (RegisterTools, handleSQL, handleContext)
- Any changes to the pagination logic that IS still used

## Git workflow

- Branch: `advisor/004-delete-dead-code`
- Commit: `refactor: remove dead pagination framework and makeAPIHandler generator leftovers`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Verify paginatedGetMaxPages usage

Check if `paginatedGetMaxPages` is used anywhere beyond its declaration:

```bash
grep -rn "paginatedGetMaxPages" internal/
```

If the only hit is the declaration in helpers.go and a reference in export.go, check if export.go actually uses it in a code path. If export.go just references it in a dead branch, remove the reference.

**Verify**: `grep -rn "paginatedGetMaxPages" internal/` → shows declaration + at most one reference

### Step 2: Delete dead pagination functions from helpers.go

Delete the following functions and types from `internal/cli/helpers.go`:
- `paginatedGet` (the main function, ~200 lines)
- `paginatedGetWithResponsePath`
- `responsePathPaginatedClient` type and its `IsDryRun`/`GetWithHeaders` methods
- `extractPaginatedItems`
- `extractPaginatedItemsFromObject`
- `extractPaginatedItemsMatchingPath`
- `extractPaginatedObjectArray`
- `paginatedCollectionEnvelopeField`
- `nextFullPageOffsetCursor`
- `nextClientSidePaginationCursor`
- `emitTruncationWarning`
- `emitMissingPaginationSignalWarning`
- `emitPaginatedGetMaxPagesWarning`
- `emitMissingPaginationCursorWarning`
- `paginationCursorToken`
- `cloneRawObject`
- `deleteRawPath`
- `rawAtPath`
- `applyResponsePath`
- `responsePayloadAtPath`
- `responsePayloadParentAtPath`
- `paginatedGetMaxPages` constant
- `canonicalPaginationCollectionKeys` map
- `envelopeMetadataArrayKeys` map (check if still used by live code first!)
- `envelopeMetadataKeys` map (check if still used by live code first!)

**Important**: Before deleting `canonicalPaginationCollectionKeys`, `envelopeMetadataArrayKeys`, and `envelopeMetadataKeys`, grep for their usage in non-dead code. If any live function references them, keep them.

**Verify**: `go build ./internal/cli` → exit 0

### Step 3: Delete dead types from mcp/tools.go

Delete from `internal/mcp/tools.go`:
- `mcpParamBinding` type
- `mcpPageConfig` type
- `makeAPIHandler` function
- `formatMCPParamValue` function (check if still used first!)
- `RegisterNovelFeatureTools` function (if it's a no-op stub)

**Important**: Before deleting `formatMCPParamValue`, grep for its usage. If any live MCP handler calls it, keep it.

**Verify**: `go build ./internal/mcp` → exit 0

### Step 4: Remove .golangci.yml exclusions

In `.golangci.yml`, remove the `exclude-files` section entirely:

**Before:**
```yaml
issues:
  exclude-files:
    - helpers\.go$
    - tools\.go$
```

**After:**
```yaml
issues: {}
```

Or remove the `issues:` key entirely if it becomes empty.

**Verify**: `go vet ./...` → exit 0

### Step 5: Run full test suite

**Verify**: `go test ./...` → all pass

### Step 6: Verify lint now covers helpers.go and tools.go

Run golangci-lint if available, or at minimum `go vet ./...` to confirm the files are no longer excluded.

**Verify**: `go vet ./...` → exit 0 (no new warnings from helpers.go or tools.go)

## Test plan

- No new tests needed — this is dead code deletion.
- The existing test suite validates that live code still works.
- The linter now covers the previously-excluded files.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -n "paginatedGet" internal/cli/helpers.go` returns no matches (function deleted)
- [ ] `grep -n "makeAPIHandler" internal/mcp/tools.go` returns no matches (function deleted)
- [ ] `grep -n "exclude-files" .golangci.yml` returns no matches
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- Deleting a function causes a compile error because live code references it (STOP and report which function).
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- After this plan lands, `helpers.go` and `tools.go` should be lint-clean. Any existing lint issues in the remaining live code will surface — fix them in a follow-up.
- The generator is effectively retired from this repo. If a new generator is introduced, the exclusion pattern should be re-evaluated.

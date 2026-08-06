# Plan 003: Add authentication to MCP HTTP transport

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1e706c0..HEAD -- cmd/telegram-mcp/main.go internal/mcp/`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `1e706c0`, 2026-08-06

## Why this matters

The MCP server's HTTP transport (`--transport http`) serves all registered tools — including the command-mirror tools that shell out to the companion CLI binary — without any authentication. Any host that can reach the MCP server port can invoke arbitrary CLI commands (unauthenticated remote code execution) and query the mirror database (unauthenticated data exfiltration). The stdio transport is safe by design (the host owns the process), but the HTTP transport needs an auth gate for any non-localhost deployment.

## Current state

- `cmd/telegram-mcp/main.go:62-68` — HTTP server starts without auth:
  ```go
  case "http":
      httpSrv := server.NewStreamableHTTPServer(s)
      fmt.Fprintf(os.Stderr, "telegram-mcp serving MCP over streamable HTTP at %s\n", *addr)
      if err := httpSrv.Start(*addr); err != nil {
  ```

- `internal/mcp/tools.go:39-68` — `RegisterTools` registers all tools without auth middleware.

- The `mcp-go` library (`github.com/mark3labs/mcp-go v0.47.0`) provides `server.NewStreamableHTTPServer` which accepts middleware options.

## Commands you will need

| Purpose   | Command                  | Expected on success |
|-----------|--------------------------|---------------------|
| Build     | `go build ./...`         | exit 0              |
| Test      | `go test ./...`          | all pass            |
| Lint      | `go vet ./...`           | exit 0              |

## Scope

**In scope**:
- `cmd/telegram-mcp/main.go` (add auth middleware and `--api-key` flag)
- `README.md` (document the new auth requirement for HTTP transport)

**Out of scope**:
- Changes to the MCP tool registration or tool handlers.
- mTLS or OAuth implementation (out of scope for this plan; shared-secret is the minimal viable auth).
- Changes to the stdio transport path.

## Git workflow

- Branch: `advisor/003-mcp-http-auth`
- Commit: `fix(mcp): require API key auth for HTTP transport`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add --api-key flag to telegram-mcp

In `cmd/telegram-mcp/main.go`, add a new flag:

```go
apiKey := flag.String("api-key", "", "API key for HTTP transport authentication (required when transport=http; env: TELEGRAM_MCP_API_KEY)")
```

After `flag.Parse()`, add validation:

```go
if strings.ToLower(*transport) == "http" {
    key := *apiKey
    if key == "" {
        key = os.Getenv("TELEGRAM_MCP_API_KEY")
    }
    if key == "" {
        fmt.Fprintf(os.Stderr, "error: --api-key or TELEGRAM_MCP_API_KEY is required for HTTP transport\n")
        os.Exit(2)
    }
    // Store for middleware use
    *apiKey = key
}
```

**Verify**: `go build ./cmd/telegram-mcp` → exit 0

### Step 2: Add bearer token middleware

Add a simple bearer token check function to `cmd/telegram-mcp/main.go`:

```go
func requireAPIKey(next http.Handler, apiKey string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        auth := r.Header.Get("Authorization")
        if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != apiKey {
            http.Error(w, "unauthorized: valid API key required", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Verify**: `go build ./cmd/telegram-mcp` → exit 0

### Step 3: Wire middleware into HTTP transport

Replace the HTTP case in the switch with:

```go
case "http":
    key := *apiKey
    if key == "" {
        key = os.Getenv("TELEGRAM_MCP_API_KEY")
    }
    httpSrv := server.NewStreamableHTTPServer(s)
    wrapped := requireAPIKey(httpSrv, key)
    httpAddr := *addr
    fmt.Fprintf(os.Stderr, "telegram-mcp serving MCP over streamable HTTP at %s (API key required)\n", httpAddr)
    listener, err := net.Listen("tcp", httpAddr)
    if err != nil {
        fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
        os.Exit(1)
    }
    if err := http.Serve(listener, wrapped); err != nil {
        fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
        os.Exit(1)
    }
```

Add `"net"` and `"net/http"` to the import block.

Note: The `mcp-go` library's `StreamableHTTPServer` implements `http.Handler`, so it can be wrapped directly. If the library uses a different interface, adapt accordingly — check `httpSrv`'s type signature.

**Verify**: `go build ./cmd/telegram-mcp` → exit 0

### Step 4: Update README.md

In `README.md`, update the HTTP transport section to document the auth requirement:

```markdown
### Configure

Add to your agent's MCP config (file location varies by agent):

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-mcp"
    }
  }
}
```

For remote or container-hosted agents, use streamable HTTP with an API key:

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-mcp",
      "args": ["--transport", "http", "--addr", ":7777", "--api-key", "YOUR_SECRET"],
      "env": {
        "TELEGRAM_MCP_API_KEY": "YOUR_SECRET"
      }
    }
  }
}
```

Set `TELEGRAM_MCP_TRANSPORT` to override the default transport without a flag.
Set `TELEGRAM_MCP_API_KEY` as an alternative to the `--api-key` flag.
```

**Verify**: `go build ./...` → exit 0

### Step 5: Run full test suite and vet

**Verify**: `go test ./...` → all pass
**Verify**: `go vet ./...` → exit 0

## Test plan

- No new automated tests in this plan — the auth middleware is a simple bearer token check. Manual testing:
  1. `telegram-mcp --transport http --addr :7777 --api-key test123` → server starts
  2. `curl http://localhost:7777/mcp` → 401 unauthorized
  3. `curl -H "Authorization: Bearer test123" http://localhost:7777/mcp` → 200 OK (or MCP protocol response)
- Existing test suite continues passing.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `grep -n "requireAPIKey" cmd/telegram-mcp/main.go` shows the middleware is wired
- [ ] `grep -n "api-key" cmd/telegram-mcp/main.go` shows the flag is registered
- [ ] `grep -n "TELEGRAM_MCP_API_KEY" README.md` shows documentation
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts.
- A step's verification fails twice after a reasonable fix attempt.
- The `mcp-go` library's `StreamableHTTPServer` does not implement `http.Handler` (check the library's API surface).
- The fix appears to require touching an out-of-scope file.

## Maintenance notes

- This plan implements shared-secret auth (bearer token). For production deployments, consider mTLS or OAuth 2.1 in a follow-up plan.
- The `--api-key` flag is required for HTTP transport — the server refuses to start without it, preventing accidental unauthenticated exposure.
- The stdio transport is unaffected (no auth needed; the host owns the process).

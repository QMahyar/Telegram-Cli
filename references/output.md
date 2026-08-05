# Output Format & Runtime Contract

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

### Envelope shape

Success on stdout:
```json
{"ok": true, "data": <payload>, "metadata": {"source": "telegram"}}
```

Error on stderr:
```json
{"ok": false, "error": {"type": "...", "exit_code": N, "hint": "...", "details": "..."}}
```

Parse `.data` for results. Use `--select` to keep only needed fields.

### Output flags

| Flag | Effect |
|------|--------|
| `--json` | JSON on stdout |
| `--agent` | JSON + compact + no prompts + uniform envelope |
| `--compact` | High-gravity fields only (implied by `--agent`) |
| `--select a,b.c` | Keep subset; dotted paths descend into nesting |
| `--csv` | Arrays as CSV |
| `--plain` | Tab-separated rows |
| `--quiet` | Suppress stdout; communicate via exit code |
| `--dry-run` | Preview request without sending |

`--select` wins over `--compact` when both set. No-match `--select` is fail-open (full output + warning on stderr).

### Mutating commands

`send`, `forward`, `delete`, `read`, `react`, `edit`, `media`, `accounts add/use/rename/remove/import` print human prose on stderr and, when stdout is piped or a machine flag is set, emit a machine-readable payload on stdout (e.g. `send` returns `{"msg_id": 123, "chat": "..."}`).

### Data source

`--data-source auto|live|local`:
- `auto` (default): prefer live, fallback to local SQLite
- `live`: Telegram servers only
- `local`: local SQLite mirror only

When stdout is a terminal with no machine-format flag, a human-readable table is printed instead.

## Output Delivery

`--deliver <sink>` routes output in addition to (or instead of) stdout:

| Sink | Effect |
|------|--------|
| `stdout` | Default |
| `file:<path>` | Atomic write (tmp + rename) |
| `webhook:<url>` | POST to URL |

## Named Profiles

Saved flag sets reused across invocations:

```bash
telegram-cli profile save briefing --json
telegram-cli --profile briefing mirror
telegram-cli profile list --json
```

Explicit flags > profile values > defaults. `agent-context` lists available profiles.

## Paths & State

| Kind | Contents |
|------|----------|
| `config` | `config.toml`, profiles |
| `data` | `credentials.toml`, `data.db`, sessions |
| `state` | Persisted queries, jobs, `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Resolution order: per-kind env var → `--home` → `TELEGRAM_HOME` → XDG → platform defaults.

For MCP, pass relocation through host config:
```json
{ "mcpServers": { "telegram": { "command": "telegram-mcp", "env": { "TELEGRAM_HOME": "/srv/telegram" } } } }
```

Relocation is one-way. Move files manually before unsetting.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error |
| 3 | Resource not found |
| 5 | API error |
| 6 | Confirmation required (re-run with `--yes`) |
| 7 | Rate limited |
| 10 | Config error |

## Agent Feedback

```bash
telegram-cli feedback "what surprised you"
telegram-cli feedback list --json --limit 10
```

Local-only by default. Write what *surprised* you, not a bug report.

# Telegram CLI

**Every Telegram account you own in one terminal: unified sync and search, flood-aware cross-account broadcasts, and a schema-driven raw gateway no other Telegram CLI offers.**

telegram-cli speaks MTProto as a real user across all your accounts at once. Sync every account into one local database, search it offline, then coordinate broadcasts, downloads, and triage across the whole fleet with protocol-aware flood protection. When Telegram ships new capabilities, the TL-layer registry and raw invoke gateway have you covered before any command is added.

Created by [@QMahyar](https://github.com/QMahyar).

## Install

Build both binaries from source (requires Go 1.26.5 or newer):

```bash
make build-all
```

or, without make:

```bash
go build -o bin/telegram-cli ./cmd/telegram-cli
go build -o bin/telegram-mcp ./cmd/telegram-mcp
```

To install into your Go bin directory so both are on `PATH`:

```bash
go install ./cmd/telegram-cli
go install ./cmd/telegram-mcp
```

### Pre-built binary

Download a pre-built binary for your platform from [GitHub Releases](https://github.com/QMahyar/Telegram-Cli/releases). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

### Agent skill

The `telegram-cli` agent skill lives in this repository's `SKILL.md`. Point your agent at it directly, or install it with the [`skills`](https://github.com/vercel-labs/skills) CLI (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other supported agents).

## Use with Claude Desktop

This CLI ships an MCP server (`telegram-mcp`) that works with Claude Desktop either via a packaged [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle or manual JSON config.

To install:

1. Download the `.mcpb` bundle for your platform from [GitHub Releases](https://github.com/QMahyar/Telegram-Cli/releases).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go build -o bin/telegram-mcp ./cmd/telegram-mcp
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-mcp"
    }
  }
}
```

</details>

## Authentication

Create app credentials once at https://my.telegram.org/apps, then export TELEGRAM_API_ID and TELEGRAM_API_HASH. Add each account with 'telegram-cli accounts add --phone +1234567890 --alias work' — the CLI prompts for the login code sent to that phone and, if enabled, the 2FA password. Non-interactive flows exist for agents: pass `--code` (and `--password` if 2FA is on) to skip the prompt, or `--qr` to log in by scanning a code with the Telegram app. `accounts import --session <telethon-string> --alias <name>` imports an existing Telethon/Pyrogram string session. Sessions are stored as per-account files in the config directory with restrictive permissions; never commit or share them. Accounts logged in via unofficial clients are monitored by Telegram under its API Terms of Service — this CLI paces itself and refuses spam-shaped defaults, but abusive use can still get accounts banned.

## Quick Start

```bash
# Works offline with no credentials — proves the binary and shows the TL method registry.
telegram-cli capabilities --json --limit 5


# Registers your first account by phone; the CLI prompts for the login code sent to it.
telegram-cli accounts add --phone +1234567890 --alias work


# Dry-run shows the MTProto calls that would run; drop --dry-run to list real dialogs.
telegram-cli chats --json --limit 10 --dry-run


# Previews an incremental sync of dialogs and messages into the local mirror.
telegram-cli sync --account work --dry-run


# Offline full-text search across everything synced from every account.
telegram-cli search "release notes" --json


# Cross-account fan-out safely previewed; drop --dry-run to send.
telegram-cli broadcast "Weekly update" --chats @mychannel --account work --dry-run --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-account orchestration

- **`broadcast`** — Post one message to dozens of chats spread across all your Telegram accounts in a single command, paced so no account trips flood control.

  _When an agent must deliver the same announcement to many chats across several accounts safely, this is the only one-shot path that handles pacing, retries, and failure reporting._

  ```bash
  telegram-cli broadcast "Release v2.1 is out" --chats @mychannel,@updates --account work --dry-run --json
  ```
- **`batch`** — Fan out forward, media download, mark-read, or raw MTProto method calls across accounts and chats as one resumable, audited job — optionally at a scheduled time.

  _When bulk-downloading or bulk-forwarding across several accounts, the job survives interruptions and reports exactly what succeeded per account._

  ```bash
  telegram-cli batch forward @releases 123 124 --to @updates --account work --dry-run --json
  ```
- **`jobs`** — Queue any broadcast or batch operation for a future time; jobs persist across restarts and fire via the scheduler loop or one-shot OS tasks.

  _When an agent must time posts or batch runs without keeping a terminal open, this is the safe, inspectable queue — 'jobs list' shows pending work, 'jobs cancel' aborts it._

  ```bash
  telegram-cli broadcast "Weekly report" --chats @team --account work --at 2026-08-04T09:00 --dry-run --json
  ```

### Fleet awareness

- **`accounts health`** — See every account's auth state, active flood cooldowns, unread totals, and session freshness in one table before you run anything risky.

  _Before any batch operation, an agent should verify which accounts are healthy and which are cooling down; this returns that in one structured call._

  ```bash
  telegram-cli accounts health --probe --json
  ```
- **`inbox`** — One unread view across every Telegram account you own, ranked by urgency, instead of opening each account separately.

  _For triage across a fleet of accounts, one call replaces N session logins and manual comparison._

  ```bash
  telegram-cli inbox --accounts all --agent
  ```
- **`daemon run`** — Run a bounded multi-account daemon: hold live sessions, collect updates into the mirror, fire due scheduled jobs, and exit with a structured report of everything observed.

  _When an agent needs live Telegram activity for a bounded window — collect for 10 minutes, then report — this returns counts, notable events, and fired jobs in one structured envelope._

  ```bash
  telegram-cli daemon run --duration 10m --accounts all --collect messages,edits,deletes --report --json
  ```
- **`since`** — Everything new across all your accounts since a point in time, grouped by account and chat.

  _For shift handoffs or morning catch-up, one call replaces scrolling every account._

  ```bash
  telegram-cli since 1d --accounts all --json
  ```

### Local mirror intelligence

- **`stats`** — Top senders, messages per day, and per-chat volume computed over your whole synced Telegram history, across accounts.

  _For archive analysis and community health checks, this answers questions the Telegram API itself cannot aggregate._

  ```bash
  telegram-cli stats --days 30 --account work --json
  ```
- **`digest`** — A mechanical weekly digest of your Telegram activity: volume per account and chat, busiest hours, top terms.

  _For weekly reviews, gives agents a compact structured summary without any LLM dependency._

  ```bash
  telegram-cli digest --days 7 --json
  ```

### Schema-driven extensibility

- **`schema check`** — Instantly see which Telegram TL layer this CLI speaks, how many methods it exposes, and whether Telegram has shipped a newer layer.

  _Before relying on a new Telegram feature, an agent can verify the installed CLI actually supports the required layer._

  ```bash
  telegram-cli schema check --json
  ```

## Recipes


### Pre-flight before any batch run

```bash
telegram-cli accounts health --probe --json
```

Confirms every account is authenticated and out of cooldown before fan-out.

### Broadcast to all accounts safely

```bash
telegram-cli broadcast "Maintenance window tonight 02:00 UTC" --accounts all --chats @ops,@status --dry-run --json
```

Previews the full fan-out plan; rerun without --dry-run to send.

### Agent triage with minimal context

```bash
telegram-cli inbox --agent --select account,chat,unread
```

Returns only the fields an agent needs, keeping deep dialog payloads out of context.

### Search the archive

```bash
telegram-cli search "kubernetes outage" --json --limit 20
```

FTS over every synced account; add --account or --chat to narrow.

### Catch up after time away

```bash
telegram-cli since 1d --accounts all --json
```

Fleet-wide delta of the last 24 hours grouped by account and chat.

## Usage

Run `telegram-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `TELEGRAM_CONFIG_DIR`, `TELEGRAM_DATA_DIR`, `TELEGRAM_STATE_DIR`, or `TELEGRAM_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `TELEGRAM_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export TELEGRAM_HOME=/srv/telegram
telegram-cli doctor
```

Under `TELEGRAM_HOME=/srv/telegram`, the four dirs resolve to `/srv/telegram/config`, `/srv/telegram/data`, `/srv/telegram/state`, and `/srv/telegram/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-mcp",
      "env": {
        "TELEGRAM_HOME": "/srv/telegram"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `TELEGRAM_DATA_DIR` overrides an explicit `--home` for that kind. Use `TELEGRAM_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `TELEGRAM_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `telegram-cli doctor --fail-on warn` to check path warnings in automation.

## Commands

### mirror

Local SQLite mirror of synced Telegram data

- **`telegram-cli mirror`** - Show local mirror stats: per-account message counts, chats, db size, sync age


### config

Inspect and edit `config.toml` (reads are read-only; `set`/`unset` rewrite the file atomically and never persist `TELEGRAM_BASE_URL`)

- **`telegram-cli config`** - Show resolved config: path, home dir, base_url, headers, and redacted auth state
- **`telegram-cli config path`** - Print the resolved config file path
- **`telegram-cli config get <key>`** - Print one value (`base_url`, `auth_header`, `headers.<name>`; empty when unset)
- **`telegram-cli config set <key> <value>`** - Set a value; `base_url` must be an absolute http(s) URL
- **`telegram-cli config unset <key>`** - Remove a value

`auth_header` is a credential: `show`/`get` always report it as `<redacted>`, and `set` writes the file with owner-only permissions.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`telegram-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`telegram-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`telegram-cli learnings list`** - Inspect taught rows
- **`telegram-cli learnings forget <query>`** - Undo a teach
- **`telegram-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`telegram-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`telegram-cli teach-pattern`** - Install a query/resource template up front
- **`telegram-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `TELEGRAM_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `telegram-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
telegram-cli mirror

# JSON for scripting and agents
telegram-cli mirror --json

# Filter to specific fields
telegram-cli mirror --json --select id,name,status

# Dry run — show the request without sending
telegram-cli mirror --dry-run

# Agent mode — JSON + compact + no prompts in one flag
telegram-cli mirror --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Flag-driven** - every input is a flag or positional; the login code/2FA during `accounts add` is the only inherent prompt, and it can be bypassed with `--code`/`--password` or `--qr` for agent-driven flows
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Uniform envelope under `--agent`** - success wraps in `{ok:true,data,metadata}` on stdout; errors emit `{ok:false,error:{type,exit_code,hint}}` on stderr
- **Write-safe** - mutating commands (`send`, `forward`, `delete`, `react`, `edit`, `broadcast`, `batch`) preview with `--dry-run`; without it they execute immediately. Read commands are non-mutating.
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `6` confirmation required, `7` rate limited, `10` config error.

## Health Check

```bash
telegram-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `telegram-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/telegram-cli/config.toml`; `--home`, `TELEGRAM_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Edit the file by hand or through the CLI:

```bash
telegram-cli config set base_url https://api.example.com
telegram-cli config set headers.X-Tenant my-tenant
telegram-cli config unset headers.X-Tenant
telegram-cli config get base_url
```

`config set` never persists the `TELEGRAM_BASE_URL` environment override, and `auth_header` values are never echoed back by `show`/`get`.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **FLOOD_WAIT_N error on sends** — Wait N seconds; the CLI records the cooldown in 'accounts health' and paces batches automatically.
- **SESSION_REVOKED or AUTH_KEY_UNREGISTERED** — The session was logged out remotely. Run 'accounts remove <alias>' then 'accounts add <alias>' to re-login.
- **PHONE_NUMBER_BANNED on login** — Telegram banned this number for API abuse. Write recover@telegram.org; do not retry from this CLI.
- **missing api_id/api_hash** — Create them at https://my.telegram.org/apps and export TELEGRAM_API_ID / TELEGRAM_API_HASH.
- **PEER_FLOOD during a broadcast** — That account entered a soft restriction. The job pauses the account for its cooldown; do not force-retry — repeated sends escalate to a ban.
- **search returns nothing you expect** — Run 'sync' first — search reads the local mirror. Use 'search --live' to query Telegram servers directly.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**gotd/td**](https://github.com/gotd/td) — Go (2303 stars)
- [**tg-mtproto-cli**](https://github.com/cyberash-dev/tg-mtproto-cli) — TypeScript (27 stars)
- [**tgcli (virat-mankali)**](https://github.com/virat-mankali/telegram-cli) — Go (7 stars)
- [**clitg (leynier)**](https://github.com/leynier/telegram-cli) — Python (6 stars)
- [**tgcli (dapi)**](https://github.com/dapi/tgcli) — JavaScript (5 stars)
- [**tgctl (b1rd33)**](https://github.com/b1rd33/tg-cli) — Python (2 stars)
- [**telegram-cli (vika2603)**](https://github.com/vika2603/telegram-cli) — Go (1 stars)
- [**telegcli**](https://github.com/KrishnaGupta653/telegcli) — Python (1 stars)

# Telegram CLI

**Every Telegram account you own in one terminal: unified sync and search, flood-aware cross-account broadcasts, and a schema-driven raw gateway no other Telegram CLI offers.**

telegram-pp-cli speaks MTProto as a real user across all your accounts at once. Sync every account into one local database, search it offline, then coordinate broadcasts, downloads, and triage across the whole fleet with protocol-aware flood protection. When Telegram ships new capabilities, the TL-layer registry and raw invoke gateway have you covered before any command is added.

Created by [@QMahyar](https://github.com/QMahyar).

## Install

The recommended path installs both the `telegram-pp-cli` binary and the `pp-telegram` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install telegram
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install telegram --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install telegram --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install telegram --agent claude-code
npx -y @mvanhorn/printing-press-library install telegram --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.5 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/telegram/cmd/telegram-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/telegram-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install telegram --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-telegram --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-telegram --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install telegram --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/telegram-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/telegram/cmd/telegram-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-pp-mcp"
    }
  }
}
```

</details>

## Authentication

Create app credentials once at https://my.telegram.org/apps, then export TELEGRAM_API_ID and TELEGRAM_API_HASH. Add each account with 'telegram-pp-cli accounts add <alias>' — a QR code renders in the terminal (scan with Telegram → Settings → Devices → Link Desktop Device), or use --phone for code login with 2FA fallback. Sessions are stored as per-account files in the config directory with restrictive permissions; never commit or share them. Accounts logged in via unofficial clients are monitored by Telegram under its API Terms of Service — this CLI paces itself and refuses spam-shaped defaults, but abusive use can still get accounts banned.

## Quick Start

```bash
# Works offline with no credentials — proves the binary and shows the TL method registry.
telegram-pp-cli capabilities list --json --limit 5


# Registers your first account by scanning a QR code; creates the session file.
telegram-pp-cli accounts add work --qr


# Dry-run shows the MTProto calls that would run; drop --dry-run to list real dialogs.
telegram-pp-cli chats list --json --limit 10 --dry-run


# Previews an incremental sync of dialogs and messages into the local mirror.
telegram-pp-cli sync --account work --dry-run


# Offline full-text search across everything synced from every account.
telegram-pp-cli search "release notes" --json


# Cross-account fan-out safely previewed; drop --dry-run to send.
telegram-pp-cli broadcast "Weekly update" --chats @mychannel --account work --dry-run --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-account orchestration

- **`broadcast`** — Post one message to dozens of chats spread across all your Telegram accounts in a single command, paced so no account trips flood control.

  _When an agent must deliver the same announcement to many chats across several accounts safely, this is the only one-shot path that handles pacing, retries, and failure reporting._

  ```bash
  telegram-pp-cli broadcast "Release v2.1 is out" --chats @mychannel,@updates --account work --dry-run --json
  ```
- **`batch`** — Fan out forward, media download, mark-read, or raw MTProto method calls across accounts and chats as one resumable, audited job — optionally at a scheduled time.

  _When bulk-downloading or bulk-forwarding across several accounts, the job survives interruptions and reports exactly what succeeded per account._

  ```bash
  telegram-pp-cli batch download --chat @releases --limit 20 --account work --dry-run --json
  ```
- **`jobs`** — Queue any broadcast or batch operation for a future time; jobs persist across restarts and fire via the scheduler loop or one-shot OS tasks.

  _When an agent must time posts or batch runs without keeping a terminal open, this is the safe, inspectable queue — 'jobs list' shows pending work, 'jobs cancel' aborts it._

  ```bash
  telegram-pp-cli broadcast "Weekly report" --chats @team --account work --at 2026-08-04T09:00 --dry-run --json
  ```

### Fleet awareness

- **`accounts health`** — See every account's auth state, active flood cooldowns, unread totals, and session freshness in one table before you run anything risky.

  _Before any batch operation, an agent should verify which accounts are healthy and which are cooling down; this returns that in one structured call._

  ```bash
  telegram-pp-cli accounts health --probe --json
  ```
- **`inbox`** — One unread view across every Telegram account you own, ranked by urgency, instead of opening each account separately.

  _For triage across a fleet of accounts, one call replaces N session logins and manual comparison._

  ```bash
  telegram-pp-cli inbox --accounts all --agent
  ```
- **`daemon run`** — Run a bounded multi-account daemon: hold live sessions, collect updates into the mirror, fire due scheduled jobs, and exit with a structured report of everything observed.

  _When an agent needs live Telegram activity for a bounded window — collect for 10 minutes, then report — this returns counts, notable events, and fired jobs in one structured envelope._

  ```bash
  telegram-pp-cli daemon run --duration 10m --accounts all --collect messages,edits,deletes --report --json
  ```
- **`since`** — Everything new across all your accounts since a point in time, grouped by account and chat.

  _For shift handoffs or morning catch-up, one call replaces scrolling every account._

  ```bash
  telegram-pp-cli since 1d --accounts all --json
  ```

### Local mirror intelligence

- **`stats`** — Top senders, messages per day, and per-chat volume computed over your whole synced Telegram history, across accounts.

  _For archive analysis and community health checks, this answers questions the Telegram API itself cannot aggregate._

  ```bash
  telegram-pp-cli stats --days 30 --account work --json
  ```
- **`digest`** — A mechanical weekly digest of your Telegram activity: volume per account and chat, busiest hours, top terms.

  _For weekly reviews, gives agents a compact structured summary without any LLM dependency._

  ```bash
  telegram-pp-cli digest --days 7 --json
  ```

### Schema-driven extensibility

- **`schema check`** — Instantly see which Telegram TL layer this CLI speaks, how many methods it exposes, and whether Telegram has shipped a newer layer.

  _Before relying on a new Telegram feature, an agent can verify the installed CLI actually supports the required layer._

  ```bash
  telegram-pp-cli schema check --json
  ```

## Recipes


### Pre-flight before any batch run

```bash
telegram-pp-cli accounts health --probe --json
```

Confirms every account is authenticated and out of cooldown before fan-out.

### Broadcast to all accounts safely

```bash
telegram-pp-cli broadcast "Maintenance window tonight 02:00 UTC" --accounts all --chats @ops,@status --dry-run --json
```

Previews the full fan-out plan; rerun without --dry-run to send.

### Agent triage with minimal context

```bash
telegram-pp-cli inbox --agent --select account,chat,unread
```

Returns only the fields an agent needs, keeping deep dialog payloads out of context.

### Search the archive

```bash
telegram-pp-cli search "kubernetes outage" --json --limit 20
```

FTS over every synced account; add --account or --chat to narrow.

### Catch up after time away

```bash
telegram-pp-cli since 1d --accounts all --json
```

Fleet-wide delta of the last 24 hours grouped by account and chat.

## Usage

Run `telegram-pp-cli --help` for the full command reference and flag list.

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
telegram-pp-cli doctor
```

Under `TELEGRAM_HOME=/srv/telegram`, the four dirs resolve to `/srv/telegram/config`, `/srv/telegram/data`, `/srv/telegram/state`, and `/srv/telegram/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "telegram": {
      "command": "telegram-pp-mcp",
      "env": {
        "TELEGRAM_HOME": "/srv/telegram"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `TELEGRAM_DATA_DIR` overrides an explicit `--home` for that kind. Use `TELEGRAM_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `TELEGRAM_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `telegram-pp-cli doctor --fail-on warn` to check path warnings in automation.

## Commands

### mirror

Local SQLite mirror of synced Telegram data

- **`telegram-pp-cli mirror`** - Show local mirror stats: per-account message counts, chats, db size, sync age


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`telegram-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`telegram-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`telegram-pp-cli learnings list`** - Inspect taught rows
- **`telegram-pp-cli learnings forget <query>`** - Undo a teach
- **`telegram-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`telegram-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`telegram-pp-cli teach-pattern`** - Install a query/resource template up front
- **`telegram-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `TELEGRAM_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `telegram-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
telegram-pp-cli mirror

# JSON for scripting and agents
telegram-pp-cli mirror --json

# Filter to specific fields
telegram-pp-cli mirror --json --select id,name,status

# Dry run — show the request without sending
telegram-pp-cli mirror --dry-run

# Agent mode — JSON + compact + no prompts in one flag
telegram-pp-cli mirror --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
telegram-pp-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `telegram-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/telegram-pp-cli/config.toml`; `--home`, `TELEGRAM_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

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

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)

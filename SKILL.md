---
name: telegram-cli
description: "Multi-account Telegram CLI: sync, search, broadcast, and batch across all your accounts from one terminal. Use when the user wants to read telegram chats, search messages, send to channels, check unreads, manage multiple telegram accounts, schedule posts, or run telegram operations from an agent. Triggers: telegram, check my telegram, send message, search messages, telegram unread, telegram broadcast, telegram channel, telegram chat, post to telegram."
author: "qmahyar"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  author: qmahyar
  version: "0.1.0"
  mcp-server: telegram-mcp
---

# Telegram CLI

## Prerequisites

Verify the CLI is installed before any command:

```bash
telegram-cli --version
```

If missing, install:

```bash
go build -o bin/telegram-cli ./cmd/telegram-cli
# or
go install ./cmd/telegram-cli
```

Do not proceed until `--version` succeeds.

## Task Routing

Map user intent to a command pattern. For exact flags and subcommands, run `telegram-cli <command> --help` or `telegram-cli which "<description>"`.

| User wants to... | Command pattern | Key flags |
|---|---|---|
| Check unread messages | `inbox --accounts all --agent` | `--select account,chat,unread` |
| See account health | `accounts health --probe --json` | Run before any batch |
| Send a message | `send "text" --chat @target --account <alias> --json` | |
| Post to many chats | `broadcast "text" --chats @a,@b --account <alias> --dry-run --json` | Drop `--dry-run` to send |
| Search message history | `search "query" --json --limit 20` | Requires prior `sync` |
| Sync messages locally | `sync --account <alias> --dry-run --json` | Drop `--dry-run` to sync |
| Catch up after absence | `since 1d --accounts all --json` | Relative or absolute time |
| Schedule a post | `broadcast "text" --chats @a --at <ISO-time> --dry-run --json` | |
| Run timed batch jobs | `batch forward ... --at <ISO-time> --dry-run --json` | |
| View pending jobs | `jobs list --json` | |
| Analyze activity | `stats --days 30 --json` or `digest --days 7 --json` | |
| Run bounded daemon | `daemon run --duration 10m --accounts all --collect messages --report --json` | |
| Check TL schema layer | `schema check --json` | |
| Diagnose issues | `doctor` | |
| Find right command | `which "<what you want to do>"` | Natural-language resolver |

## When to Use

Use this CLI when a task involves real Telegram user accounts: reading chats, searching history, posting to channels, triaging unreads, or coordinating across multiple accounts. It is the right tool whenever more than one account must act in one workflow.

## Anti-triggers

Do NOT use for:
- Mass unsolicited DMs or spam — violates Telegram ToS, burns accounts
- Secret (E2E encrypted) chats — out of scope
- Voice/video calls or payments — out of scope in v1
- Bot-only integrations — use Bot API directly
- Resident message-relay daemon — commands dial per invocation in v1

## Agent Mode

Add `--agent` to any command. Expands to `--json --compact --no-input --no-color --yes`.

- **Pipeable:** JSON on stdout, errors on stderr
- **Filterable:** `--select field1,field2.nested` keeps only needed fields
- **Previewable:** `--dry-run` shows the request without sending
- **Write-safe:** mutating commands require `--dry-run` to preview; without it they execute

Full envelope shape, output flags, data-source semantics, and exit codes: see [references/output.md](references/output.md).

## Automatic Learning

The CLI caches query→resource mappings so repeat queries skip discovery. The protocol:

1. **`recall` first** — before any list/search/drill on a new question:
   ```bash
   telegram-cli recall "<question>" --agent
   ```
2. **Handle candidates** — if `candidates` appear, follow each candidate's `next_action` (trial command → `learnings confirm <id>`). Reject wrong ones with `learnings reject <id>`.
3. **Handle playbook** — if `playbook` appears, replay its `steps` with `slots_resolved` substitutions. Read `notes` first.
4. **`teach &` always** — after resolving a query the store couldn't answer, background-teach:
   ```bash
   telegram-cli teach --query "<question>" --resource-type <type> --resource <id> &
   ```
5. **`playbook amend &`** when you observe a correction the notes should know.

**Empty-store short-circuit:** if `recall` finds nothing AND `learnings list` and `learnings candidates` are both empty, skip recall for the rest of the session.

Full decision tree, warning semantics, playbook format, and PII rules: see [references/learning.md](references/learning.md).

Disable per-invocation with `--no-learn`, or globally with `TELEGRAM_NO_LEARN=true`.

## MCP Server

Build and register with your agent's MCP config:

```bash
go build -o bin/telegram-mcp ./cmd/telegram-mcp
```

```json
{ "mcpServers": { "telegram": { "command": "telegram-mcp" } } }
```

Transports: `stdio` (default, local) or `http --addr :7777` (remote/hosted). Set `TELEGRAM_MCP_TRANSPORT` to override. Config file location varies by agent — check your agent's docs.

## Direct Use

1. Check installed: `telegram-cli --version`
2. Match task to command via the routing table above, or ask the CLI: `telegram-cli which "<task>"`
3. Execute with `--agent`:
   ```bash
   telegram-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill in: `telegram-cli <command> --help`

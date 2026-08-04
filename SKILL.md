---
name: telegram-cli
description: "Every Telegram account you own in one terminal: unified sync and search, flood-aware cross-account broadcasts, and a schema-driven raw gateway no other Telegram CLI offers. Trigger phrases: `check my telegram unread`, `schedule this telegram post for tomorrow`, `post this to all my telegram channels`, `search my telegram messages`, `list my telegram chats`, `which of my telegram accounts are healthy`, `run the telegram daemon for 10 minutes and report`, `use telegram`, `run telegram cli`."
author: "qmahyar"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - telegram-cli
    install:
      - kind: go
        bins: [telegram-cli]
        module: telegram-cli/cmd/telegram-cli
---

# Telegram — CLI

## Prerequisites: Install the CLI

This skill drives the `telegram-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Build from the repository source (requires Go 1.26.5 or newer):
   ```bash
   go build -o bin/telegram-cli ./cmd/telegram-cli
   ```
2. Verify: `telegram-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

To install into `$GOPATH/bin` (default `$HOME/go/bin`) instead, add that directory to `$PATH`:

```bash
go install ./cmd/telegram-cli
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

telegram-cli speaks MTProto as a real user across all your accounts at once. Sync every account into one local database, search it offline, then coordinate broadcasts, downloads, and triage across the whole fleet with protocol-aware flood protection. When Telegram ships new capabilities, the TL-layer registry and raw invoke gateway have you covered before any command is added.

## When to Use This CLI

Use this CLI when a task involves real Telegram user accounts from a terminal or agent: reading and archiving chats, offline search over message history, coordinating posts or downloads across several accounts, triaging unread inboxes, or invoking arbitrary Telegram API methods. It is the right tool whenever more than one account must act in one workflow.

## Anti-triggers

Do not use this CLI for:
- Do not use for mass unsolicited DM campaigns or spam — it violates Telegram's API Terms of Service and will burn accounts
- Do not use for secret (end-to-end encrypted) chats — MTProto secret chats are out of scope
- Do not use for voice/video calls or payments/Stars flows in v1
- Do not use for bot-only integrations — a Bot API wrapper is the simpler choice there
- Do not use it as a resident message-relay daemon in v1 — commands dial per invocation

## Unique Capabilities

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
  telegram-cli batch download --chat @releases --limit 20 --account work --dry-run --json
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

## Command Reference

**mirror** — Local SQLite mirror of synced Telegram data

- `telegram-cli mirror` — Show local mirror stats: per-account message counts, chats, db size, sync age


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
telegram-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

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

## Auth Setup

Create app credentials once at https://my.telegram.org/apps, then export TELEGRAM_API_ID and TELEGRAM_API_HASH. Add each account with 'telegram-cli accounts add --phone +1234567890 --alias work' — the CLI prompts for the login code sent to that phone and, if enabled, the 2FA password. Non-interactive flows exist for agents: pass `--code` (and `--password` if 2FA is on) to skip the prompt, or `--qr` to log in by scanning a code with the Telegram app (Settings → Devices → Link Desktop Device). `accounts import --session <telethon-string> --alias <name>` imports an existing Telethon/Pyrogram string session. Sessions are stored as per-account files in the config directory with restrictive permissions; never commit or share them. Accounts logged in via unofficial clients are monitored by Telegram under its API Terms of Service — this CLI paces itself and refuses spam-shaped defaults, but abusive use can still get accounts banned.

Run `telegram-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`. It also switches to machine-built output: success envelopes `{ok:true, data, metadata}` on stdout, and errors emit `{ok:false, error:{type, exit_code, hint, details}}` on stderr.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  telegram-cli mirror --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Flag-driven** — every input is a flag or positional; the login code/2FA during `accounts add` is the only inherent prompt, and it can be bypassed with `--code`/`--password` or `--qr` for agent-driven flows
- **Write-safe** — mutating commands (`send`, `forward`, `delete`, `react`, `edit`, `broadcast`, `batch`) preview with `--dry-run`; without it they execute immediately. Read commands are non-mutating.

### Output shape

Read commands emit JSON on stdout — a JSON array for lists, a JSON object for a single resource. In `--agent` mode the payload is nested under a uniform envelope: `{"ok": true, "data": <payload>, "metadata": {"source": "telegram"}}` — parse `.data` for the results and `.ok` to confirm success. Use `--select` to keep only the fields you need (comma-separated; dotted paths descend into nested structures and arrays traverse element-wise). A no-match `--select` is fail-open: full output is returned with a `warning: --select "x" matched no fields; valid fields: ...` line on stderr. The root output flags behave identically on Telegram commands and scaffolded commands: `--compact` keeps only high-gravity fields (and `--agent` implies it), `--csv` renders arrays as CSV, `--plain` as tab-separated rows, `--quiet` suppresses stdout and communicates via the exit code, and `--select` wins over `--compact` when both are set.

```bash
telegram-cli chats --agent --select peer_id,title,unread_count
```

Mutating commands (`send`, `forward`, `delete`, `read`, `react`, `edit`, `media`, `accounts add/use/rename/remove/import`) print human prose on stderr and, when stdout is a pipe or a machine flag (`--json`, `--agent`, `--csv`, ...) is set, also emit a machine-readable payload on stdout (e.g. `send` returns `{"msg_id": 123, "chat": "..."}`) so an agent can capture the identifiers it needs to follow up.

The `--data-source` flag (`auto` | `live` | `local`) controls whether reads hit Telegram's servers (`live`), the local SQLite mirror (`local`), or prefer live with a local fallback (`auto`, the default). When stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set, a human-readable table is printed instead; piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `TELEGRAM_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `TELEGRAM_CONFIG_DIR`, `TELEGRAM_DATA_DIR`, `TELEGRAM_STATE_DIR`, `TELEGRAM_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `TELEGRAM_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `telegram-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `TELEGRAM_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `TELEGRAM_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
telegram-cli recall "<user's question>" --agent
```

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "telegram-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `telegram-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `telegram-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities — direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
telegram-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
telegram-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
telegram-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know — a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback — fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
telegram-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of this CLI. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`telegram-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `TELEGRAM_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
telegram-cli feedback "the --since flag is inclusive but docs say exclusive"
telegram-cli feedback --stdin < notes.txt
telegram-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `TELEGRAM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `TELEGRAM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
telegram-cli profile save briefing --json
telegram-cli --profile briefing mirror
telegram-cli profile list --json
telegram-cli profile show briefing
telegram-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 5 | API error (upstream issue) |
| 6 | Confirmation required (irreversible/visible write — re-run with `--yes`) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `telegram-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go build -o bin/telegram-mcp ./cmd/telegram-mcp
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add telegram-mcp -- telegram-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which telegram-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   telegram-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `telegram-cli <command> --help`.

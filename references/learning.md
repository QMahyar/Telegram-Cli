# Learning Protocol — Full Reference

This document is the complete self-learning protocol. SKILL.md summarizes it; this file has every detail.

## Overview

The CLI journals every invocation locally. It auto-derives `flag_alias` candidates from failed-flag + corrected-retry pairs, and synthesizes a `playbook_candidate` when a family is taught without one. Your role is judgment only: `recall` first, act on candidates, `teach` the answer, `playbook amend` when you observe a correction.

## Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question:

```bash
telegram-cli recall "<user's question>" --agent
```

Response envelope:

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
  "mismatches": [],
  "warnings": [],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "telegram-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot}>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "...", "canonical": "..." } },
    "notes": "<workarounds + gotchas>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

**Empty-store short-circuit:** if the store has no learnings, playbooks, or candidates yet (recall finds nothing AND `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session. Resume once something has been taught.

## Step 2: Decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    → candidates are try-then-confirm, never facts
    → follow each candidate's next_action verbatim:
      1. run the trial command
      2. run `learnings confirm <id>` ONLY after trial verified the behavior
    → reject wrong candidates with `learnings reject <id>`
    → NEVER re-teach something recall surfaced as a candidate
    → candidates ride alongside playbooks/hits, not instead of them

elif Playbook present:
    → READ Playbook.notes verbatim FIRST
    → replay Playbook.steps in order, substituting slots_resolved
    → if a slot is unresolved, fall back to discovery for that step only
    → expected_tool_calls is a budget; exceeding it → `playbook amend`

elif Notes present (no Playbook):
    → read Notes verbatim before any discovery step

elif Found AND Results[0].entity_match == "exact" AND Results[0].confidence >= 2:
    → skip discovery; fetch live data for Results[*].resource_id

elif Found AND Results[0].entity_match == "partial":
    → candidate hint, NOT a hit; validate before trusting

elif Mismatches[] present (--debug-mismatches):
    → cold start; stored learning is for a different entity

else:
    → cold start; run discovery; teach afterward (Step 4)
```

## Step 3: Warnings

| Warning | Meaning | Action |
|---------|---------|--------|
| `low_confidence` | confidence < 2 | Hint only, don't skip discovery |
| `resource_not_in_store` | learning points to missing resource | Direct-fetch and re-evaluate |
| `cross_alias_match` | matched via entity_lookups alias | Trust the resource_id |
| `similar_shape_different_entity:<canonical>` | same structure, different entity | Cold start; warning carries conflicting canonical |
| `ambiguous_alias` | one entity → multiple canonicals | Surface ambiguity before committing |
| `candidates_present` | envelope has candidates section | Handle via Step 2 before anything else |
| `no_learnings_for_query_family` | no rows above Jaccard floor | Pure cold start |

## Step 4: `teach &` — always, unconditionally

After resolving a query the store could not answer, background-teach:

```bash
telegram-cli teach --query "<question>" --resource-type <type> --resource <id>
# append shell `&` to background it
```

- Silent on success; errors land in `teach.log`
- Teach the **most specific** resource (leaf id, not parent)
- Cross-alias resolution is automatic via `entity_lookups`
- **PII rule:** strip names, emails, phones, account ids from taught queries

## Step 5: Playbooks

A teach on a family without a playbook auto-synthesizes a `playbook_candidate`. To attach explicit playbook flags:

```bash
# Integrated form (resource + playbook in one call)
telegram-cli teach \
  --query "<question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md &
```

Playbook JSON shape: `{ "steps": [...], "entity_slots": [...], "expected_tool_calls": N }`

Notes files: markdown with gotchas verbatim.

File-free callers (MCP): use `--playbook-json` and `--playbook-notes` inline.

Playbooks are keyed on structural query family (entities stripped) so one recipe covers all same-shaped queries.

## Step 6: `playbook amend &`

When you observe a concrete correction:

```bash
telegram-cli playbook amend \
  --query "<exact recall query>" \
  --add-note "<correction>" &
```

**Worth amending:** workarounds, undocumented endpoint shapes, schema drift, pagination tricks.

**NOT worth amending:** entity-specific answers, per-row data, paraphrases of existing notes.

PII discipline: no user filesystems, API keys, emails, GitHub handles in amend notes.

## Measuring the loop

```bash
telegram-cli learnings stats
```

Reports recall hit rate, teach-to-reuse, playbook resolution rate, candidate confirm/reject counts. Everything local.

## Disabling

- `--no-learn` per invocation
- `TELEGRAM_NO_LEARN=true` globally

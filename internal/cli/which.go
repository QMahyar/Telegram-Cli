// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// whichEntry is one row of the curated capability index. The index is seeded
// at generation time from the verified NovelFeature list that drives the
// SKILL.md feature section, so the command a `which` query returns is
// guaranteed to exist and to match what the skill advertises.
type whichEntry struct {
	Command      string `json:"command"`
	Description  string `json:"description"`
	Group        string `json:"group,omitempty"`
	WhyItMatters string `json:"why_it_matters,omitempty"`
}

// whichIndex is the curated capability index: one entry per friendly command
// (plus subcommand variants for the families agents actually ask about).
// `which` resolves a natural-language capability query to the best match;
// the index is static and hand-curated so every returned command exists.
var whichIndex = []whichEntry{
	// Cross-account orchestration
	{Command: "broadcast", Description: "Post one message to dozens of chats spread across all your Telegram accounts in a single command, paced so no account trips flood control.", Group: "Cross-account orchestration", WhyItMatters: "When an agent must deliver the same announcement to many chats across several accounts safely, this is the only one-shot path that handles pacing, retries, and failure reporting."},
	{Command: "batch forward", Description: "Fan out forwarding of message ids to many target chats as one resumable, audited job with pacing.", Group: "Cross-account orchestration", WhyItMatters: "Bulk-forwarding across several accounts without tripping flood control; the job survives interruptions."},
	{Command: "jobs", Description: "Queue broadcast or batch operations for a future time; jobs persist across restarts.", Group: "Cross-account orchestration", WhyItMatters: "Timing posts or batch runs without keeping a terminal open."},
	{Command: "daemon run", Description: "Run a bounded multi-account daemon: hold live sessions, collect updates into the mirror, fire due scheduled jobs, and exit with a structured report.", Group: "Fleet awareness", WhyItMatters: "Live Telegram activity for a bounded window, then a structured report."},
	{Command: "accounts health", Description: "See every account's auth state, active flood cooldowns, unread totals, and session freshness in one table; --probe dials each account live.", Group: "Fleet awareness", WhyItMatters: "Before any batch operation, verify which accounts are healthy and which are cooling down."},
	{Command: "accounts list", Description: "List every configured account with its user id, username, phone, and verification status.", Group: "Fleet awareness", WhyItMatters: "See the fleet: which aliases exist, which are still unverified after add."},
	{Command: "accounts add", Description: "Add a new Telegram account with phone + code login or QR scan, then backfill its identity.", Group: "Fleet awareness", WhyItMatters: "Onboarding a new account into the fleet."},
	{Command: "accounts use", Description: "Set an account as the default for subsequent commands (mark it last-used).", Group: "Fleet awareness", WhyItMatters: "Convenience default for single-account workflows; multi-account agents must pass --account."},
	{Command: "accounts remove", Description: "Remove an account: logs out, deletes the session directory and the registry row.", Group: "Fleet awareness", WhyItMatters: "Decommissioning an account cleanly."},
	{Command: "accounts status", Description: "Show the auth status (authorized / user identity) of one account.", Group: "Fleet awareness", WhyItMatters: "Quick single-account auth check."},
	{Command: "inbox", Description: "One unread view across every Telegram account you own, ranked by urgency, instead of opening each account separately.", Group: "Fleet awareness", WhyItMatters: "For triage across a fleet of accounts, one call replaces N session logins."},
	{Command: "since", Description: "Everything new across all your accounts since a point in time, grouped by account and chat.", Group: "Fleet awareness", WhyItMatters: "For shift handoffs or morning catch-up, one call replaces scrolling every account."},

	// Read: chats, messages, search, contacts
	{Command: "chats", Description: "List and inspect Telegram chats (dialogs): titles, unread counts, pinned state.", Group: "Read Telegram data", WhyItMatters: "What conversations exist for an account?"},
	{Command: "chats leave", Description: "Leave a group or channel (messages.deleteChatUser / channels.leaveChannel).", Group: "Chat lifecycle", WhyItMatters: "Exit a conversation without destroying it."},
	{Command: "chats delete", Description: "Delete a group or channel you own — irreversible, requires --yes.", Group: "Chat lifecycle", WhyItMatters: "Destroy a conversation permanently."},
	{Command: "search", Description: "Search messages across all chats; filter by chat, date range (--since/--until), and type.", Group: "Read Telegram data", WhyItMatters: "Find a message anywhere across the fleet."},
	{Command: "messages", Description: "Show message history for a chat with paging and date filters (--offset, --since, --until).", Group: "Read Telegram data", WhyItMatters: "Read a conversation thread."},
	{Command: "contacts", Description: "List or search your Telegram contacts.", Group: "Read Telegram data", WhyItMatters: "Who is in the address book?"},
	{Command: "contacts add", Description: "Add a user to your contacts (contacts.addContact) with optional first/last name.", Group: "Contact management", WhyItMatters: "Build the address book programmatically."},
	{Command: "contacts remove", Description: "Remove users from your contacts (contacts.deleteContacts), gated by --yes.", Group: "Contact management", WhyItMatters: "Clean the address book."},
	{Command: "contacts info", Description: "Show a full user profile: about, common chats, blocked status, premium.", Group: "Read Telegram data", WhyItMatters: "Full profile without opening the app."},
	{Command: "blocked", Description: "List users you have blocked (contacts.getBlocked).", Group: "Contact management", WhyItMatters: "Review the block list."},
	{Command: "block", Description: "Block users (contacts.block).", Group: "Contact management", WhyItMatters: "Stop spam and harassment."},
	{Command: "unblock", Description: "Unblock users (contacts.unblock).", Group: "Contact management", WhyItMatters: "Restore access after blocking."},
	{Command: "topics", Description: "List forum topics for a chat.", Group: "Read Telegram data", WhyItMatters: "Browse forum-style groups."},
	{Command: "templates", Description: "Add, show, or remove reusable message templates for broadcasts.", Group: "Read Telegram data", WhyItMatters: "Reusable text for repeated posts."},

	// Write: send / forward / delete / read / react / edit / media
	{Command: "send", Description: "Send a text message (or media with --media) to a chat; supports --reply-to and scheduled --at.", Group: "Send messages", WhyItMatters: "The core outbound capability."},
	{Command: "forward", Description: "Forward messages from one chat to another.", Group: "Send messages", WhyItMatters: "Move content between chats."},
	{Command: "delete", Description: "Delete messages from a chat (optionally --revoke for all participants), gated by --yes.", Group: "Send messages", WhyItMatters: "Remove content you sent."},
	{Command: "read", Description: "Mark all messages in a chat as read.", Group: "Send messages", WhyItMatters: "Zero the unread counter."},
	{Command: "react", Description: "Send an emoji reaction to a message.", Group: "Send messages", WhyItMatters: "Lightweight engagement."},
	{Command: "edit", Description: "Edit a message you already sent.", Group: "Send messages", WhyItMatters: "Fix a typo post-send."},
	{Command: "media", Description: "Download media attached to a message.", Group: "Send messages", WhyItMatters: "Pull attachments to disk."},

	// Sync / mirror / analytics
	{Command: "sync", Description: "Sync dialogs and peers into the local mirror; pass a chat to also pull its message history.", Group: "Local mirror intelligence", WhyItMatters: "Populate the offline mirror that sql/stats/digest read."},
	{Command: "export", Description: "Export synced messages as JSONL.", Group: "Local mirror intelligence", WhyItMatters: "Archive or migrate data."},
	{Command: "sql", Description: "Run read-only SQL against the local mirror database (tg_messages, tg_dialogs, tg_peers).", Group: "Local mirror intelligence", WhyItMatters: "Ad-hoc analysis over synced history."},
	{Command: "stats", Description: "Message volume statistics computed over your synced history.", Group: "Local mirror intelligence", WhyItMatters: "Community health and archive analysis."},
	{Command: "digest", Description: "A mechanical digest of your Telegram activity: volume per account and chat, busiest hours, top terms.", Group: "Local mirror intelligence", WhyItMatters: "Weekly review without an LLM."},
	{Command: "watch", Description: "Watch a chat for new messages for a bounded window and report them.", Group: "Fleet awareness", WhyItMatters: "Live monitoring without a daemon."},

	// Gateway / schema
	{Command: "raw", Description: "Invoke raw MTProto methods with JSON params (the escape hatch for anything not covered).", Group: "Schema-driven extensibility", WhyItMatters: "Anything the friendly surface lacks."},
	{Command: "schema check", Description: "Instantly see which Telegram TL layer this CLI speaks and whether Telegram shipped a newer one.", Group: "Schema-driven extensibility", WhyItMatters: "Verify layer support before relying on a new feature."},
	{Command: "api", Description: "Show the friendly endpoint mirrors available for scripted access.", Group: "Schema-driven extensibility", WhyItMatters: "Discover the mirror surface."},
	{Command: "capabilities", Description: "List every top-level command with its purpose.", Group: "Discovery", WhyItMatters: "Broad capability discovery."},
	{Command: "doctor", Description: "Check CLI health: config, Telegram app credentials, sessions, API reachability.", Group: "Discovery", WhyItMatters: "Diagnose why commands fail."},
	{Command: "which", Description: "Resolve a natural-language capability query to the best matching command.", Group: "Discovery", WhyItMatters: "Ask what command implements an intent."},
	{Command: "agent-context", Description: "Dump a schema-versioned inventory of every command, flag, and annotation for agents.", Group: "Discovery", WhyItMatters: "Introspect the CLI without parsing help text."},
	{Command: "config", Description: "Show or set CLI configuration values.", Group: "Discovery", WhyItMatters: "Manage credentials and options."},
	{Command: "audit", Description: "Read the local run receipt / audit trail.", Group: "Discovery", WhyItMatters: "See what ran and when."},
	{Command: "feedback", Description: "Read or send feedback about this CLI.", Group: "Discovery", WhyItMatters: "Report issues."},

	// Learning loop
	{Command: "recall", Description: "Search the self-learned capability store for previously taught answers.", Group: "Self-learning loop", WhyItMatters: "Reuse what was taught before."},
	{Command: "teach", Description: "Teach the CLI a query → resource mapping so recall finds it later.", Group: "Self-learning loop", WhyItMatters: "Grow the local knowledge base."},
	{Command: "learnings", Description: "Inspect taught rows, candidates, and loop metrics.", Group: "Self-learning loop", WhyItMatters: "Audit what the loop has learned."},
}

// whichMatch pairs an index entry with its ranking score for a query.
// Higher score means stronger match. The ranker is naive (exact token
// then substring then group tag) because 20-40 entries do not need
// semantic retrieval - a ranker upgrade is a future change that would
// not break this contract.
type whichMatch struct {
	Entry whichEntry `json:"entry"`
	Score int        `json:"score"`
}

// rankWhich returns up to `limit` best matches for `query` against the
// index, sorted by descending score. Score breakdown:
//
//	+3  exact token match on the command's leaf or full path
//	+2  substring match on the command (any part)
//	+2  substring match on the description
//	+2  per-token match on the description
//	+1  group tag contains the query as a word
//
// Ties break on declaration order in the index. An empty query returns
// every entry at score 0 in declaration order - this is the "list all"
// behavior the skill documents for broad agent discovery.
func rankWhich(index []whichEntry, query string, limit int) []whichMatch {
	if limit <= 0 {
		limit = 3
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		out := make([]whichMatch, 0, len(index))
		for _, e := range index {
			out = append(out, whichMatch{Entry: e, Score: 0})
		}
		return out
	}
	// Sub-tokenize the query the same way command paths are split, so a
	// pasted hyphenated capability (repos-list-for-authenticated) matches.
	qTokens := whichSubTokens(q)

	scored := make([]whichMatch, 0, len(index))
	for i, e := range index {
		score := whichScoreEntry(e, q, qTokens)
		scored = append(scored, whichMatch{Entry: e, Score: score})
		_ = i
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		// Specificity tie-break: at equal score prefer the command with the
		// fewest capability sub-tokens - the canonical operation over variants
		// carrying extra words the request never used.
		return len(whichSubTokens(strings.ToLower(scored[i].Entry.Command))) <
			len(whichSubTokens(strings.ToLower(scored[j].Entry.Command)))
	})
	// Drop zero-score matches when the query was non-empty; agents
	// branching on exit code rely on "no match" meaning no confidence.
	filtered := scored[:0]
	for _, m := range scored {
		if m.Score > 0 {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

func whichScoreEntry(e whichEntry, query string, qTokens []string) int {
	score := 0
	cmd := strings.ToLower(e.Command)
	// Sub-token split (spaces, hyphens, underscores, slashes): a capability
	// word buried in a hyphenated leaf (repos-list-for-authenticated) must be
	// matchable by the words a human asks with, or every command in a group
	// ties on the group token alone and index order decides the answer.
	cmdTokens := whichSubTokens(cmd)
	desc := strings.ToLower(e.Description)
	descTokens := strings.Fields(desc)
	group := strings.ToLower(e.Group)

	// Exact token match on the command path (any token).
	for _, qt := range qTokens {
		for _, ct := range cmdTokens {
			if whichTokenMatch(qt, ct) {
				score += 3
				break
			}
		}
	}
	// Substring match on the full command (covers hyphenated leaves).
	if strings.Contains(cmd, query) {
		score += 2
	}
	// Substring match on the description.
	if strings.Contains(desc, query) {
		score += 2
	}
	// Per-token description match, CAPPED: natural-language requests often say
	// "top coins by market cap" and the endpoint doc uses the same words - but
	// uncapped description credit lets long token-soup descriptions outrank the
	// precise command path, so the credit saturates at 3.
	descCredit := 0
	for _, qt := range qTokens {
		for _, dt := range descTokens {
			if whichTokenMatch(qt, dt) {
				descCredit++
				break
			}
		}
		if descCredit == 3 {
			break
		}
	}
	score += descCredit
	// Group tag match.
	if group != "" {
		for _, qt := range qTokens {
			if strings.Contains(group, qt) {
				score += 1
				break
			}
		}
	}
	// Possessive aliasing: "my/mine/me/current" in a request is API-speak for
	// the authenticated caller; commands scoped to the authenticated user must
	// outrank generic listings for possessive asks.
	possessive := false
	for _, qt := range qTokens {
		switch qt {
		case "my", "mine", "me", "current":
			possessive = true
		}
	}
	if possessive {
		for _, ct := range cmdTokens {
			if ct == "authenticated" || ct == "me" {
				score += 3
				break
			}
		}
	}
	// Read-intent default: penalize write-verb commands when the request never
	// asked for a write, so neutral asks can never rank a destructive command
	// first on a tie.
	queryWrite := false
	for _, qt := range qTokens {
		if whichWriteVerbs[qt] {
			queryWrite = true
			break
		}
	}
	if score > 0 {
		if !queryWrite {
			for _, ct := range cmdTokens {
				if whichWriteVerbs[ct] {
					score -= 2
					break
				}
			}
		}
		// Write-intent mirror: when the request explicitly asked for a write
		// ("send a message", "block a user"), read-only commands carrying the
		// same noun must not win a tie against the write command — the asker's
		// verb is the intent, not the noun.
		if queryWrite {
			readOnly := true
			for _, ct := range cmdTokens {
				if whichWriteVerbs[ct] {
					readOnly = false
					break
				}
			}
			if readOnly {
				score -= 2
			}
		}
	}
	// Specificity: a command leaf carrying capability sub-tokens the request never
	// used is a variant, not the canonical answer ("activity-list-repos-
	// starred-by-authenticated" for a repositories ask). Parent resource tokens
	// are excluded so a valid nested command is not erased by its path.
	if score > 0 && len(qTokens) > 1 {
		unmatched := 0
		commandParts := strings.Fields(cmd)
		leafTokens := whichSubTokens(commandParts[len(commandParts)-1])
		for _, ct := range leafTokens {
			hit := false
			for _, qt := range qTokens {
				if whichTokenMatch(qt, ct) {
					hit = true
					break
				}
			}
			if !hit {
				unmatched++
			}
		}
		if unmatched > 3 {
			unmatched = 3
		}
		score -= unmatched
	}
	return score
}

func whichTokenMatch(a, b string) bool {
	a = strings.Trim(strings.ToLower(a), ".,:;!?()[]{}\"'")
	b = strings.Trim(strings.ToLower(b), ".,:;!?()[]{}\"'")
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if whichSingular(a) == whichSingular(b) {
		return true
	}
	return whichTokenAliases[a] != "" && whichTokenAliases[a] == whichTokenAliases[b]
}

func whichSubTokens(cmd string) []string {
	return strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '/'
	})
}

// The closed API-verb set for write-shaped commands. A request that never
// asked for a write must not tie-break into a destructive command.
var whichWriteVerbs = map[string]bool{
	"delete": true, "remove": true, "update": true, "create": true, "set": true,
	"add": true, "replace": true, "rename": true, "transfer": true, "merge": true,
	"lock": true, "unlock": true, "star": true, "unstar": true, "follow": true,
	"unfollow": true, "block": true, "unblock": true, "mute": true, "archive": true,
	"unarchive": true, "cancel": true, "send": true, "upload": true, "subscribe": true,
	"unsubscribe": true, "dismiss": true, "approve": true, "decline": true,
	"post": true, "put": true, "write": true, "edit": true, "modify": true,
	"publish": true, "share": true, "comment": true, "grant": true, "revoke": true,
}

var whichTokenAliases = map[string]string{
	"repo": "repository", "repos": "repository", "repository": "repository", "repositories": "repository",
	// Telegram capability aliases: one canonical form per intent family so
	// natural-language requests resolve to the same command regardless of
	// the noun the asker used.
	"contact": "contacts", "contacts": "contacts", "people": "contacts", "person": "contacts",
	"chat": "chats", "chats": "chats", "dialog": "chats", "dialogs": "chats", "conversation": "chats",
	"user": "user", "users": "user", "username": "user", "profile": "user",
	"message": "messages", "messages": "messages", "history": "messages", "thread": "messages",
	"unread": "inbox", "inbox": "inbox",
	"block": "block", "ban": "block",
	"blocked": "blocked",
	"unblock": "unblock",
	"reply":   "send", "schedule": "send", "post": "send", "message-send": "send",
	"sync": "sync", "mirror": "sync",
	"search": "search", "find": "search", "lookup": "search", "filter": "search", "date": "search",
	"stats": "stats", "statistics": "stats", "analytics": "stats",
	"digest": "digest", "summary": "digest",
	"export": "export", "backup": "export",
	"delete": "delete", "remove": "delete",
	"react": "react", "reaction": "react",
	"edit": "edit", "modify": "edit",
	"pin": "pin", "pinned": "pin",
	"leave": "leave", "exit": "leave",
	"media": "media", "download": "media", "attachment": "media",
}

func whichSingular(s string) string {
	if len(s) > 3 && strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	if len(s) > 3 && strings.HasSuffix(s, "es") {
		return strings.TrimSuffix(s, "es")
	}
	if len(s) > 2 && strings.HasSuffix(s, "s") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

func newWhichCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "which [query]",
		Short: "Find the command that implements a capability",
		Annotations: map[string]string{
			"mcp:read-only":        "true",
			"cli:typed-exit-codes": "0,2",
		},
		Long: `which resolves a natural-language capability query (for example, "search messages" or "stale tickets") to the best matching command from this CLI's curated feature index.

Exit codes:
  0  at least one match found
  2  no confident match - the query did not score against any indexed capability; fall back to '--help' or 'search' if this CLI has one`,
		Example: `  telegram-cli which "stale tickets"
  telegram-cli which "bottleneck"
  telegram-cli which --limit 1 "send message"
  telegram-cli which                                # list the full capability index`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(whichIndex) == 0 {
				return usageErr(fmt.Errorf("this CLI has no curated capability index; run '--help' to see every command"))
			}
			query := strings.Join(args, " ")
			matches := rankWhich(whichIndex, query, limit)

			// Empty query returns the whole index at score 0 (listing mode).
			if strings.TrimSpace(query) == "" {
				return renderWhich(cmd, flags, rankWhichAll(whichIndex))
			}

			if len(matches) == 0 {
				// Under --json, return an empty matches envelope at exit 0
				// so agents can branch on `matches.length == 0` instead of
				// parsing a usage error message. Non-JSON keeps the typed
				// exit-2 path so terminal users see the help hint.
				if flags.asJSON {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
						"matches": []whichMatch{},
					}, flags)
				}
				return usageErr(fmt.Errorf("no match for %q; try '%s --help' for the full command list", query, cmd.Root().Name()))
			}
			return renderWhich(cmd, flags, matches)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 3, "Maximum number of matches to return")
	return cmd
}

// rankWhichAll is a narrow helper used by the "empty query lists the
// index" path. It returns every entry in declaration order at score 0
// so the render path treats them uniformly.
func rankWhichAll(index []whichEntry) []whichMatch {
	out := make([]whichMatch, 0, len(index))
	for _, e := range index {
		out = append(out, whichMatch{Entry: e, Score: 0})
	}
	return out
}

func renderWhich(cmd *cobra.Command, flags *rootFlags, matches []whichMatch) error {
	w := cmd.OutOrStdout()
	// Output shape follows the same rule as every other generated
	// command: JSON when the caller asked for it OR when stdout is not
	// a terminal; table when a human is looking.
	asJSON := flags.asJSON
	if !asJSON && !isTerminal(w) {
		asJSON = true
	}
	if asJSON {
		// JSON envelope: {matches: [...]}. The wrap is critical:
		// printJSONFiltered's --compact path uses compactListFields
		// (allowlist) for top-level arrays, which would strip
		// entry/score keys; routing through compactObjectFields
		// (blocklist) via an object envelope preserves them.
		if matches == nil {
			matches = []whichMatch{}
		}
		return printJSONFiltered(w, map[string]any{"matches": matches}, flags)
	}
	fmt.Fprintf(w, "%-24s  %-8s  %s\n", "COMMAND", "SCORE", "DESCRIPTION")
	for _, m := range matches {
		fmt.Fprintf(w, "%-24s  %-8d  %s\n", m.Entry.Command, m.Score, m.Entry.Description)
	}
	return nil
}

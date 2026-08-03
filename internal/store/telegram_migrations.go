// Telegram-specific store schema. Runs its own CREATE TABLE IF NOT EXISTS
// migrations from a lazy init; the emitted migration slice in store.go stays
// untouched (regen-durable pattern).
package store

import (
	"context"
	"fmt"
)

// EnsureTelegramSchema creates the tg_* tables the multi-account CLI needs.
// Idempotent; safe to call on every command that touches the mirror.
func EnsureTelegramSchema(ctx context.Context, s *Store) error {
	stmts := []string{
		// Account registry: one row per logged-in Telegram account.
		`CREATE TABLE IF NOT EXISTS tg_accounts (
			alias        TEXT PRIMARY KEY,
			user_id      INTEGER NOT NULL DEFAULT 0,
			username     TEXT NOT NULL DEFAULT '',
			first_name   TEXT NOT NULL DEFAULT '',
			phone        TEXT NOT NULL DEFAULT '',
			dc_id        INTEGER NOT NULL DEFAULT 0,
			session_dir  TEXT NOT NULL,
			created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			last_used_at TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL DEFAULT 'active'
		)`,
		// Per-account peer cache: access_hash values are session-scoped.
		`CREATE TABLE IF NOT EXISTS tg_peers (
			account     TEXT NOT NULL,
			peer_type   TEXT NOT NULL,           -- user | chat | channel
			peer_id     INTEGER NOT NULL,        -- positive; channels stored without -100 prefix
			access_hash INTEGER NOT NULL DEFAULT 0,
			title       TEXT NOT NULL DEFAULT '',
			username    TEXT NOT NULL DEFAULT '',
			updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			PRIMARY KEY (account, peer_type, peer_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tg_peers_username ON tg_peers (account, username)`,
		// Synced dialogs (chat list) per account.
		`CREATE TABLE IF NOT EXISTS tg_dialogs (
			account       TEXT NOT NULL,
			peer_type     TEXT NOT NULL,
			peer_id       INTEGER NOT NULL,
			title         TEXT NOT NULL DEFAULT '',
			username      TEXT NOT NULL DEFAULT '',
			unread_count  INTEGER NOT NULL DEFAULT 0,
			last_msg_id   INTEGER NOT NULL DEFAULT 0,
			last_msg_date TEXT NOT NULL DEFAULT '',
			last_msg_text TEXT NOT NULL DEFAULT '',
			pinned        INTEGER NOT NULL DEFAULT 0,
			synced_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			PRIMARY KEY (account, peer_type, peer_id)
		)`,
		// Synced messages — the unified mirror across all accounts.
		`CREATE TABLE IF NOT EXISTS tg_messages (
			account    TEXT NOT NULL,
			peer_type  TEXT NOT NULL,
			peer_id    INTEGER NOT NULL,
			msg_id     INTEGER NOT NULL,
			date       INTEGER NOT NULL DEFAULT 0,   -- unix seconds
			sender_id  INTEGER NOT NULL DEFAULT 0,
			sender     TEXT NOT NULL DEFAULT '',
			text       TEXT NOT NULL DEFAULT '',
			media_type TEXT NOT NULL DEFAULT '',     -- '' | photo | document | video | audio | sticker | ...
			media_name TEXT NOT NULL DEFAULT '',
			outgoing   INTEGER NOT NULL DEFAULT 0,
			synced_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			PRIMARY KEY (account, peer_type, peer_id, msg_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tg_messages_date ON tg_messages (account, date)`,
		// Full-text search over the unified mirror.
		`CREATE VIRTUAL TABLE IF NOT EXISTS tg_messages_fts USING fts5(
			text, sender, title,
			content='tg_messages', content_rowid='rowid',
			tokenize='unicode61'
		)`,
		`CREATE TRIGGER IF NOT EXISTS tg_messages_ai AFTER INSERT ON tg_messages BEGIN
			INSERT INTO tg_messages_fts(rowid, text, sender, title)
			VALUES (new.rowid, new.text, new.sender, '');
		END`,
		`CREATE TRIGGER IF NOT EXISTS tg_messages_ad AFTER DELETE ON tg_messages BEGIN
			INSERT INTO tg_messages_fts(tg_messages_fts, rowid, text, sender, title)
			VALUES ('delete', old.rowid, old.text, old.sender, '');
		END`,
		// Batch jobs: broadcast / forward / download / read / raw fan-outs,
		// plus scheduled executions.
		`CREATE TABLE IF NOT EXISTS tg_jobs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			kind         TEXT NOT NULL,            -- broadcast | forward | download | read | raw
			status       TEXT NOT NULL DEFAULT 'pending', -- pending | scheduled | running | done | failed | cancelled
			params_json  TEXT NOT NULL DEFAULT '{}',
			accounts_csv TEXT NOT NULL DEFAULT '',
			targets_csv  TEXT NOT NULL DEFAULT '',
			text         TEXT NOT NULL DEFAULT '',
			media_path   TEXT NOT NULL DEFAULT '',
			at           TEXT NOT NULL DEFAULT '', -- RFC3339 scheduled time; '' = immediate
			created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			started_at   TEXT NOT NULL DEFAULT '',
			finished_at  TEXT NOT NULL DEFAULT '',
			error        TEXT NOT NULL DEFAULT ''
		)`,
		// Per-target results of a job (audit + resume).
		`CREATE TABLE IF NOT EXISTS tg_job_results (
			job_id     INTEGER NOT NULL,
			account    TEXT NOT NULL,
			target     TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'pending', -- pending | ok | skipped | failed
			detail     TEXT NOT NULL DEFAULT '',
			message_id INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			PRIMARY KEY (job_id, account, target)
		)`,
		// Per-account flood cooldown ledger (FLOOD_WAIT / PEER_FLOOD memory).
		`CREATE TABLE IF NOT EXISTS tg_cooldowns (
			account    TEXT NOT NULL,
			scope      TEXT NOT NULL DEFAULT 'global', -- global | send | <peer key>
			kind       TEXT NOT NULL DEFAULT 'FLOOD_WAIT',
			until_unix INTEGER NOT NULL DEFAULT 0,
			seconds    INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			PRIMARY KEY (account, scope, kind)
		)`,
		// Append-only audit trail for mutating operations.
		`CREATE TABLE IF NOT EXISTS tg_audit (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			account    TEXT NOT NULL DEFAULT '',
			command    TEXT NOT NULL DEFAULT '',
			target     TEXT NOT NULL DEFAULT '',
			params     TEXT NOT NULL DEFAULT '',
			result     TEXT NOT NULL DEFAULT '',
			detail     TEXT NOT NULL DEFAULT ''
		)`,
		// Broadcast templates.
		`CREATE TABLE IF NOT EXISTS tg_templates (
			name       TEXT PRIMARY KEY,
			text       TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB().ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("telegram schema migration: %w", err)
		}
	}
	return nil
}

// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"telegram-cli/internal/store"
)

// seedMirrorFixture writes a minimal account/peer/message set into a store
// and returns its DB path. Reused by local-search tests.
func seedMirrorFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "telegram.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.DB().Close()
	ctx := context.Background()
	if err := store.EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	stmts := []string{
		`INSERT OR REPLACE INTO tg_accounts (alias, status, phone, user_id, username, first_name, dc_id, session_dir, last_used_at)
		 VALUES ('work','active','+0000',100,'workuser','Work',2,'work', strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, access_hash, title, username, updated_at)
		 VALUES ('work','channel',-100111,999,'Release Notes','releases', strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
		`INSERT OR REPLACE INTO tg_messages (account, peer_type, peer_id, msg_id, date, sender_id, sender, text, media_type, outgoing)
		 VALUES ('work','channel',-100111,1,1735689600,100,'Work User','deploy v2.3 to prod', '', 0)`,
		`INSERT OR REPLACE INTO tg_messages (account, peer_type, peer_id, msg_id, date, sender_id, sender, text, media_type, outgoing)
		 VALUES ('work','channel',-100111,2,1735693200,100,'Work User','photo dump from the party', 'photo', 0)`,
	}
	for _, q := range stmts {
		if _, err := s.DB().ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	return dbPath
}

// TestMirrorFTSSearch_PhraseMatch pins the local mirror search: FTS5 MATCH
// against tg_messages_fts joined back to tg_messages, with a media-type filter
// and a since window.
func TestMirrorFTSSearch_PhraseMatch(t *testing.T) {
	dbPath := seedMirrorFixture(t)
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.DB().Close()
	ctx := context.Background()

	items, err := mirrorFTSSearch(ctx, s.DB(), "work", "deploy", "", "", "", time.Time{}, time.Time{}, "", 0, 30)
	if err != nil {
		t.Fatalf("mirrorFTSSearch: %v", err)
	}
	if len(items) != 1 || items[0].MsgID != 1 {
		t.Fatalf("expected msg 1 for 'deploy', got %+v", items)
	}
	if items[0].Text != "deploy v2.3 to prod" {
		t.Fatalf("unexpected text %q", items[0].Text)
	}
}

// TestMirrorFTSSearch_MediaTypeFilter pins the --type filter on the local path.
func TestMirrorFTSSearch_MediaTypeFilter(t *testing.T) {
	dbPath := seedMirrorFixture(t)
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.DB().Close()
	ctx := context.Background()

	items, err := mirrorFTSSearch(ctx, s.DB(), "work", "party", "", "", "", time.Time{}, time.Time{}, "photo", 0, 30)
	if err != nil {
		t.Fatalf("mirrorFTSSearch: %v", err)
	}
	if len(items) != 1 || items[0].MsgID != 2 {
		t.Fatalf("expected photo msg 2, got %+v", items)
	}
}

// TestMirrorMessages_EmptyMirrorIsNotAnError pins that an unpopulated mirror
// returns an empty slice (not an error) so the notice can fire.
func TestMirrorMessages_EmptyMirrorIsNotAnError(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "telegram.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.DB().Close()
	ctx := context.Background()
	if err := store.EnsureTelegramSchema(ctx, s); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	items, err := mirrorMessages(ctx, s.DB(), "work", "", "", "", time.Time{}, time.Time{}, "", 0, 30)
	if err != nil {
		t.Fatalf("mirrorMessages on empty mirror: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

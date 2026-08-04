package mtproto

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

// PeerResolver resolves text references (username, ID, "me") to tg.InputPeerClass.
// It uses a local SQLite cache populated by `sync` and populated during
// dialog/message sync. Live provides an optional API-backed fallback for
// @username misses (wired by commands that hold a live session).
type PeerResolver struct {
	db *sql.DB
	// Live resolves an @username via the live session (e.g. contacts.resolveUsername).
	// It must return the InputPeer and the username that was requested, so the
	// resolver can persist the resolved access hash into the local cache.
	Live func(ctx context.Context, username string) (tg.InputPeerClass, error)
}

// NewPeerResolver wraps a *sql.DB.
func NewPeerResolver(db *sql.DB) *PeerResolver {
	return &PeerResolver{db: db}
}

// Resolve turns a text reference into an InputPeerClass.
// Supported formats: "me"/"self", @username, numeric ID (positive = user,
// negative or -100xxx = channel), raw peer ID with prefix "id:".
func (pr *PeerResolver) Resolve(ctx context.Context, account, ref string) (tg.InputPeerClass, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty peer reference")
	}

	// "me" / "self"
	if ref == "me" || ref == "self" {
		return &tg.InputPeerSelf{}, nil
	}

	// @username
	if strings.HasPrefix(ref, "@") {
		username := strings.TrimPrefix(ref, "@")
		return pr.resolveUsername(ctx, account, username)
	}

	// id: prefix
	if strings.HasPrefix(ref, "id:") {
		id, err := strconv.ParseInt(strings.TrimPrefix(ref, "id:"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric ID: %w", err)
		}
		return pr.resolveByID(ctx, account, id)
	}

	// Plain numeric
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return pr.resolveByID(ctx, account, id)
	}

	// Strip t.me/ prefix
	if strings.HasPrefix(ref, "https://t.me/") {
		username := strings.TrimPrefix(ref, "https://t.me/")
		username = strings.TrimSuffix(username, "/")
		return pr.resolveUsername(ctx, account, username)
	}

	return nil, fmt.Errorf("unrecognized peer reference: %q (use @username, numeric ID, or me)", ref)
}

// resolveUsername tries the local cache first, then the API.
func (pr *PeerResolver) resolveUsername(ctx context.Context, account, username string) (tg.InputPeerClass, error) {
	// Try cache
	var (
		peerType string
		peerID   int64
		hash     int64
	)
	err := pr.db.QueryRowContext(ctx,
		`SELECT peer_type, peer_id, access_hash FROM tg_peers WHERE account = ? AND username = ? COLLATE NOCASE`,
		account, username,
	).Scan(&peerType, &peerID, &hash)
	if err == nil && hash != 0 {
		return makeInputPeer(peerType, peerID, hash), nil
	}

	// Cache miss or missing access_hash — fall back to the live session when
	// one is attached, then persist the resolved peer so future lookups hit.
	if pr.Live != nil {
		peer, err := pr.Live(ctx, username)
		if err != nil {
			return nil, err
		}
		if err := pr.persistResolved(ctx, account, username, peer); err != nil {
			// Persistence is best-effort; the resolved peer still works.
			_ = err
		}
		return peer, nil
	}

	return nil, fmt.Errorf("@%s not found in local cache — run sync first or use numeric ID", username)
}

// persistResolved stores a live-resolved peer (with its access hash) into the
// local cache so subsequent lookups for the same username succeed offline.
func (pr *PeerResolver) persistResolved(ctx context.Context, account, username string, peer tg.InputPeerClass) error {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		_, err := pr.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, access_hash, title, username, updated_at)
			 VALUES (?, 'user', ?, ?, '', ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			account, p.UserID, p.AccessHash, username,
		)
		return err
	case *tg.InputPeerChannel:
		_, err := pr.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, access_hash, title, username, updated_at)
			 VALUES (?, 'channel', ?, ?, '', ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			account, p.ChannelID, p.AccessHash, username,
		)
		return err
	case *tg.InputPeerChat:
		_, err := pr.db.ExecContext(ctx,
			`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, access_hash, title, username, updated_at)
			 VALUES (?, 'chat', ?, 0, '', ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
			account, p.ChatID, username,
		)
		return err
	default:
		return fmt.Errorf("unsupported resolved peer type %T", peer)
	}
}

// resolveByID maps a raw numeric ID to InputPeerClass using cached access_hash.
// Telegram user IDs are positive; chat IDs are negative; channel IDs are
// -100xxx (but stored without the -100 prefix in our DB).
func (pr *PeerResolver) resolveByID(ctx context.Context, account string, rawID int64) (tg.InputPeerClass, error) {
	// Determine likely peer type from sign
	peerType, dbID := classifyID(rawID)

	var accessHash int64
	err := pr.db.QueryRowContext(ctx,
		`SELECT access_hash FROM tg_peers WHERE account = ? AND peer_type = ? AND peer_id = ?`,
		account, peerType, dbID,
	).Scan(&accessHash)
	if err == nil && accessHash != 0 {
		return makeInputPeer(peerType, dbID, accessHash), nil
	}

	// If it looks like a user and we have no access_hash, return a self-peer
	// (the API still works for some operations without access_hash on legacy IDs).
	if peerType == "user" {
		return &tg.InputPeerUser{UserID: rawID}, nil
	}

	return nil, fmt.Errorf("peer %d not found in local cache — run sync first", rawID)
}

// classifyID infers peer_type and stores the clean peer_id.
func classifyID(id int64) (peerType string, peerID int64) {
	switch {
	case id == 1 || id == 1466512585: // Telegram service accounts
		return "user", id
	case id > 0:
		return "user", id
	case id < -1000000000000:
		// -100xxx — channel. The raw MTProto peer id is -(1000000000000 + N)
		// where N is the plain channel id, so stripping the -100 prefix yields
		// the positive id: -id - 1000000000000. (id + (-1000000000000) would
		// push the id further negative and is non-idempotent.)
		return "channel", -id - 1000000000000
	case id < 0:
		// Legacy chat (negative, not -100xxx)
		return "chat", -id
	default:
		return "user", id
	}
}

// makeInputPeer constructs the correct InputPeerClass.
func makeInputPeer(peerType string, id, accessHash int64) tg.InputPeerClass {
	switch peerType {
	case "user":
		return &tg.InputPeerUser{UserID: id, AccessHash: accessHash}
	case "chat":
		return &tg.InputPeerChat{ChatID: id}
	case "channel":
		return &tg.InputPeerChannel{ChannelID: id, AccessHash: accessHash}
	default:
		return &tg.InputPeerUser{UserID: id, AccessHash: accessHash}
	}
}

// InputPeerFromRef is a convenience wrapper for commands that need to resolve
// a single peer reference. Returns the InputPeerClass.
func InputPeerFromRef(db *sql.DB, ctx context.Context, account, ref string) (tg.InputPeerClass, error) {
	return NewPeerResolver(db).Resolve(ctx, account, ref)
}

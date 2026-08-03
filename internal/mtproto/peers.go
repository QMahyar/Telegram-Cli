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
// dialog/message sync. Fallback to ContactsResolveUsername when the cache misses.
type PeerResolver struct {
	db *sql.DB
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

	// Cache miss or missing access_hash — try to resolve via cache from dialogs
	// (the sync command populates the cache with access_hash values).
	return nil, fmt.Errorf("@%s not found in local cache — run sync first or use numeric ID", username)
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
		// -100xxx — channel. Strip the -100 prefix.
		return "channel", id + (-1000000000000)
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

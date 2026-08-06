package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"
	"telegram-cli/internal/store"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newSearchCmd(flags *rootFlags) *cobra.Command {
	var chatRef, fromRef, sinceStr, untilStr, filterType string
	var offsetID int64
	var local bool
	cmd := &cobra.Command{
		Use:         "search <query>",
		Short:       "Search messages across all chats (or within --chat), with date/type filters",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Args:        cobra.ExactArgs(1),
		Example: `  telegram-cli search "hello" --chat @mychannel
  telegram-cli search "deploy" --since 1d --until 2026-08-01
  telegram-cli search "photo dump" --type photo --chat @archive
  telegram-cli search "roadmap" --from @colleague --limit 20
  telegram-cli search "roadmap" --local            (query the synced mirror, offline)
  telegram-cli search "roadmap" --data-source local (same as --local)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			s, err := openStore(ctx, home)
			if err != nil {
				return err
			}
			defer s.DB().Close()
			f := parseTelegramFlags(cmd)
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			// --data-source local / --local: query the mirror offline.
			if local || strings.EqualFold(flags.dataSource, "local") {
				return searchLocal(ctx, cmd, s, alias, query, f, searchLocalOptions{
					Chat:    chatRef,
					From:    fromRef,
					Since:   sinceStr,
					Until:   untilStr,
					Type:    filterType,
					Offset:  offsetID,
					ForceDS: true,
				})
			}
			// Parse date filters before dialing; a bad spec is a usage error.
			sinceT, err := parseSinceSpec(sinceStr)
			if err != nil {
				return usageErr(err)
			}
			untilT, err := parseSinceSpec(untilStr)
			if err != nil {
				return usageErr(err)
			}
			filter, err := mtproto.MessageFilterForType(filterType)
			if err != nil {
				return usageErr(err)
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var messages []mtproto.MessageItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				opts := mtproto.SearchOptions{
					Limit:    f.Limit,
					OffsetID: int(offsetID),
					Filter:   filter,
				}
				if chatRef != "" {
					peer, err := resolver.Resolve(ctx, alias, chatRef)
					if err != nil {
						return err
					}
					opts.Peer = peer
				}
				if fromRef != "" {
					fromPeer, err := resolver.Resolve(ctx, alias, fromRef)
					if err != nil {
						return err
					}
					opts.FromID = fromPeer
				}
				if sinceStr != "" {
					opts.MinDate = sinceT.Unix()
				}
				if untilStr != "" {
					opts.MaxDate = untilT.Unix()
				}
				messages, err = mtproto.SearchMessagesWithOptions(ctx, api, query, opts)
				return err
			})
			if err != nil {
				return err
			}
			return outResult(stdout(), f, messages)
		},
	}
	cmd.Flags().StringVar(&chatRef, "chat", "", "scope search to one chat (@username, id, or me)")
	cmd.Flags().StringVar(&fromRef, "from", "", "only messages sent by this user")
	cmd.Flags().StringVar(&sinceStr, "since", "", "only messages newer than this (RFC3339, 2026-01-01, or 1d/12h/30m)")
	cmd.Flags().StringVar(&untilStr, "until", "", "only messages older than this (RFC3339, 2026-01-01, or 1d/12h/30m)")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by type: photo, video, document, url, gif, voice, music, sticker, poll, geo, pinned")
	cmd.Flags().Int64Var(&offsetID, "offset", 0, "page to messages older than this message id")
	cmd.Flags().BoolVar(&local, "local", false, "query the synced mirror (tg_messages_fts) instead of Telegram servers")
	addTelegramFlags(cmd)
	return cmd
}

// searchLocalOptions carries the filters for a mirror-backed search.
type searchLocalOptions struct {
	Chat    string
	From    string
	Since   string
	Until   string
	Type    string
	Offset  int64
	ForceDS bool // true when the caller explicitly asked for local
}

// localMediaType maps the live --type names to mirror media_type values.
func localMediaType(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "all":
		return "", true
	case "photo", "photos":
		return "photo", true
	case "video", "videos":
		return "video", true
	case "document", "file", "files":
		return "document", true
	case "music", "audio":
		return "audio", true
	case "sticker", "stickers":
		return "sticker", true
	case "voice", "voice-message":
		return "voice", true
	default:
		return "", false
	}
}

// searchLocal runs a query against the mirror's FTS index (tg_messages_fts).
// It resolves chat/from refs from the cached tg_peers table only — no dial.
func searchLocal(ctx context.Context, cmd *cobra.Command, s *store.Store, alias, query string, f telegramCmdFlags, opts searchLocalOptions) error {
	_ = opts.ForceDS
	var peerType, peerID string
	var senderID string
	// Offline peer resolution from the cached mirror (no Live fallback).
	if opts.Chat != "" {
		peer, err := offlineResolve(s.DB(), alias, opts.Chat)
		if err != nil {
			return notFoundErr(fmt.Errorf("resolve chat %q from mirror: %w (run `telegram-cli sync` to cache peers)", opts.Chat, err))
		}
		pt, pid, err := peerKey(s.DB(), alias, peer)
		if err != nil {
			return err
		}
		peerType = pt
		peerID = fmt.Sprintf("%d", pid)
	}
	if opts.From != "" {
		peer, err := offlineResolve(s.DB(), alias, opts.From)
		if err != nil {
			return notFoundErr(fmt.Errorf("resolve user %q from mirror: %w (run `telegram-cli sync` to cache peers)", opts.From, err))
		}
		if p, ok := peer.(*tg.InputPeerUser); ok {
			senderID = fmt.Sprintf("%d", p.UserID)
		} else {
			return usageErr(fmt.Errorf("--from %q is not a user peer", opts.From))
		}
	}
	var sinceT, untilT time.Time
	var err error
	if opts.Since != "" {
		if sinceT, err = parseSinceSpec(opts.Since); err != nil {
			return usageErr(err)
		}
	}
	if opts.Until != "" {
		if untilT, err = parseSinceSpec(opts.Until); err != nil {
			return usageErr(err)
		}
	}
	mediaType, ok := localMediaType(opts.Type)
	if !ok {
		return usageErr(fmt.Errorf("--type %q is not supported on the local mirror (use photo, video, document, music, sticker, or voice)", opts.Type))
	}
	var messages []mtproto.MessageItem
	limit := f.Limit
	if limit <= 0 {
		limit = 30
	}
	if strings.TrimSpace(query) == "" {
		// Empty query: recent messages with filters.
		messages, err = mirrorMessages(ctx, s.DB(), alias, peerType, peerID, senderID, sinceT, untilT, mediaType, opts.Offset, limit)
	} else {
		messages, err = mirrorFTSSearch(ctx, s.DB(), alias, query, peerType, peerID, senderID, sinceT, untilT, mediaType, opts.Offset, limit)
	}
	if err != nil {
		return err
	}
	warnMirrorEmpty(ctx, cmd, s.DB(), &f)
	return outResult(stdout(), f, messages)
}

// offlineResolve resolves a peer ref from the cached tg_peers mirror only.
func offlineResolve(db *sql.DB, alias, ref string) (tg.InputPeerClass, error) {
	r := mtproto.NewPeerResolver(db) // no Live → cache-only lookups
	return r.Resolve(context.Background(), alias, ref)
}

// mirrorMessages reads recent messages from the mirror with filters.
func mirrorMessages(ctx context.Context, db *sql.DB, alias, peerType, peerID, senderID string, sinceT, untilT time.Time, mediaType string, offsetID int64, limit int) ([]mtproto.MessageItem, error) {
	where := []string{"account = ?"}
	args := []any{alias}
	if peerType != "" {
		where = append(where, "peer_type = ?")
		args = append(args, peerType)
	}
	if peerID != "" {
		where = append(where, "peer_id = ?")
		args = append(args, peerID)
	}
	if senderID != "" {
		where = append(where, "sender_id = ?")
		args = append(args, senderID)
	}
	if !sinceT.IsZero() {
		where = append(where, "date >= ?")
		args = append(args, sinceT.Unix())
	}
	if !untilT.IsZero() {
		where = append(where, "date <= ?")
		args = append(args, untilT.Unix())
	}
	if mediaType != "" {
		where = append(where, "media_type = ?")
		args = append(args, mediaType)
	}
	if offsetID > 0 {
		where = append(where, "msg_id < ?")
		args = append(args, offsetID)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT msg_id, sender_id, sender, text, media_type, outgoing, date FROM tg_messages
		 WHERE `+strings.Join(where, " AND ")+` ORDER BY date DESC LIMIT ?`,
		append(args, limit)...,
	)
	if err != nil {
		return nil, fmt.Errorf("mirror read: %w", err)
	}
	defer rows.Close()
	var items []mtproto.MessageItem
	for rows.Next() {
		var m mtproto.MessageItem
		if err := rows.Scan(&m.MsgID, &m.SenderID, &m.Sender, &m.Text, &m.Media, &m.Outgoing, &m.Date); err != nil {
			return nil, fmt.Errorf("scanning mirror row: %w", err)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// mirrorFTSSearch runs an FTS5 MATCH over tg_messages_fts, joined back to
// tg_messages for the full row.
func mirrorFTSSearch(ctx context.Context, db *sql.DB, alias, query, peerType, peerID, senderID string, sinceT, untilT time.Time, mediaType string, offsetID int64, limit int) ([]mtproto.MessageItem, error) {
	// FTS5 MATCH phrase: escape embedded quotes by doubling, wrap in quotes so
	// multi-word queries match as a phrase rather than an implicit OR.
	escaped := strings.ReplaceAll(query, `"`, `""`)
	match := `"` + escaped + `"`
	where := []string{"m.account = ?", "tg_messages_fts MATCH ?"}
	args := []any{alias, match}
	if peerType != "" {
		where = append(where, "m.peer_type = ?")
		args = append(args, peerType)
	}
	if peerID != "" {
		where = append(where, "m.peer_id = ?")
		args = append(args, peerID)
	}
	if senderID != "" {
		where = append(where, "m.sender_id = ?")
		args = append(args, senderID)
	}
	if !sinceT.IsZero() {
		where = append(where, "m.date >= ?")
		args = append(args, sinceT.Unix())
	}
	if !untilT.IsZero() {
		where = append(where, "m.date <= ?")
		args = append(args, untilT.Unix())
	}
	if mediaType != "" {
		where = append(where, "m.media_type = ?")
		args = append(args, mediaType)
	}
	if offsetID > 0 {
		where = append(where, "m.msg_id < ?")
		args = append(args, offsetID)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT m.msg_id, m.sender_id, m.sender, m.text, m.media_type, m.outgoing, m.date
		 FROM tg_messages_fts f
		 JOIN tg_messages m ON m.rowid = f.rowid
		 WHERE `+strings.Join(where, " AND ")+` ORDER BY m.date DESC LIMIT ?`,
		append(args, limit)...,
	)
	if err != nil {
		// Malformed FTS query (e.g. stray quotes) — surface as usage error.
		return nil, fmt.Errorf("mirror search: %w", err)
	}
	defer rows.Close()
	var items []mtproto.MessageItem
	for rows.Next() {
		var m mtproto.MessageItem
		if err := rows.Scan(&m.MsgID, &m.SenderID, &m.Sender, &m.Text, &m.Media, &m.Outgoing, &m.Date); err != nil {
			return nil, fmt.Errorf("scanning mirror row: %w", err)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

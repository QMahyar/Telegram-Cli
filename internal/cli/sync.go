package cli

import (
	"context"
	"database/sql"
	"fmt"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"
	"telegram-cli/internal/store"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

// peerKey reduces a resolved InputPeerClass to the mirror's (peer_type, peer_id)
// pair. The self peer is stored as the account's own user ID.
func peerKey(db interface {
	QueryRow(query string, args ...any) *sql.Row
}, alias string, peer tg.InputPeerClass) (string, int64, error) {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		return "user", p.UserID, nil
	case *tg.InputPeerChat:
		return "chat", p.ChatID, nil
	case *tg.InputPeerChannel:
		return "channel", p.ChannelID, nil
	case *tg.InputPeerSelf:
		var userID int64
		err := db.QueryRow(`SELECT user_id FROM tg_accounts WHERE alias = ?`, alias).Scan(&userID)
		if err != nil {
			return "", 0, fmt.Errorf("resolve self user id: %w", err)
		}
		return "user", userID, nil
	default:
		return "", 0, fmt.Errorf("unsupported peer type %T", peer)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// peerFromDialog rebuilds an InputPeerClass for a dialog row using the access
// hash saved in tg_peers (required for channel history fetches).
func peerFromDialog(db interface {
	QueryRow(query string, args ...any) *sql.Row
}, alias string, d mtproto.DialogItem) (tg.InputPeerClass, error) {
	var accessHash int64
	err := db.QueryRow(`SELECT access_hash FROM tg_peers WHERE account=? AND peer_type=? AND peer_id=?`,
		alias, d.PeerType, d.PeerID).Scan(&accessHash)
	if err != nil {
		accessHash = 0
	}
	switch d.PeerType {
	case "user":
		return &tg.InputPeerUser{UserID: d.PeerID, AccessHash: accessHash}, nil
	case "chat":
		return &tg.InputPeerChat{ChatID: d.PeerID}, nil
	case "channel":
		return &tg.InputPeerChannel{ChannelID: d.PeerID, AccessHash: accessHash}, nil
	default:
		return nil, fmt.Errorf("unsupported peer type %q", d.PeerType)
	}
}

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var withMessages bool
	var perChat int
	cmd := &cobra.Command{
		Use:   "sync [chat]",
		Short: "Sync dialogs (or messages from a specific chat) to the local mirror",
		Example: `  telegram-cli sync
  telegram-cli sync --messages
  telegram-cli sync --messages --per-chat 50
  telegram-cli sync @mychannel --limit 200`,
		Annotations: map[string]string{
			"cli:api-resource": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "sync dialogs")
			}
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var s *store.Store
			if dbPath != "" {
				s, err = openStoreAtPath(ctx, dbPath)
			} else {
				s, err = openStore(ctx, home)
			}
			if err != nil {
				return err
			}
			defer s.DB().Close()
			f := parseTelegramFlags(cmd)
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				// Chat message sync: sync <chat> pulls that chat's history into the mirror.
				if len(args) > 0 {
					resolver := liveResolver(s.DB(), api)
					peer, err := resolver.Resolve(ctx, alias, args[0])
					if err != nil {
						return err
					}
					peerType, peerID, err := peerKey(s.DB(), alias, peer)
					if err != nil {
						return err
					}
					msgs, err := mtproto.GetHistory(ctx, api, peer, f.Limit)
					if err != nil {
						return err
					}
					for _, m := range msgs {
						if _, err := s.DB().ExecContext(ctx,
							`INSERT OR REPLACE INTO tg_messages (account, peer_type, peer_id, msg_id, date, sender_id, sender, text, media_type, outgoing)
							 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
							alias, peerType, peerID, m.MsgID, m.Date, m.SenderID, m.Sender, m.Text, m.Media, boolToInt(m.Outgoing),
						); err != nil {
							return fmt.Errorf("persisting message %d from %s: %w", m.MsgID, args[0], err)
						}
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d messages from %s.\n", len(msgs), args[0])
					return nil
				}
				dialogs, err := mtproto.GetDialogs(ctx, api, f.Limit)
				if err != nil {
					return err
				}
				// Save dialogs and peers to the mirror
				total := 0
				for _, d := range dialogs {
					if _, err := s.DB().ExecContext(ctx,
						`INSERT OR REPLACE INTO tg_dialogs (account, peer_type, peer_id, title, username, unread_count, last_msg_id, pinned)
						 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						alias, d.PeerType, d.PeerID, d.Title, d.Username, d.Unread, d.LastMsgID, d.Pinned,
					); err != nil {
						return fmt.Errorf("persisting dialog %s: %w", d.Title, err)
					}
					if _, err := s.DB().ExecContext(ctx,
						`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, access_hash, title, username, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
						alias, d.PeerType, d.PeerID, d.AccessHash, d.Title, d.Username,
					); err != nil {
						return fmt.Errorf("persisting peer %s: %w", d.Title, err)
					}
					// With --messages, pull recent history per dialog so the mirror
					// has real tg_messages rows for stats/digest/since/inbox.
					if withMessages {
						if _, err := s.DB().ExecContext(ctx, `DELETE FROM tg_messages WHERE account=? AND peer_type=? AND peer_id=?`,
							alias, d.PeerType, d.PeerID); err != nil {
							return fmt.Errorf("clearing mirror messages for %s: %w", d.Title, err)
						}
						peer, err := peerFromDialog(s.DB(), alias, d)
						if err != nil {
							return fmt.Errorf("resolve dialog %s: %w", d.Title, err)
						}
						msgs, err := mtproto.GetHistory(ctx, api, peer, perChat)
						if err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: history for %s: %v\n", d.Title, err)
							continue
						}
						for _, m := range msgs {
							if _, err := s.DB().ExecContext(ctx,
								`INSERT OR REPLACE INTO tg_messages (account, peer_type, peer_id, msg_id, date, sender_id, sender, text, media_type, outgoing)
							 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
								alias, d.PeerType, d.PeerID, m.MsgID, m.Date, m.SenderID, m.Sender, m.Text, m.Media, boolToInt(m.Outgoing),
							); err != nil {
								return fmt.Errorf("persisting mirror message %d from %s: %w", m.MsgID, d.Title, err)
							}
						}
						total += len(msgs)
					}
				}
				if withMessages {
					fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d dialogs and %d messages for %s.\n", len(dialogs), total, alias)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d dialogs for %s.\n", len(dialogs), alias)
				}
				return nil
			})
			return err
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database (default: auto-detected)")
	cmd.Flags().BoolVar(&withMessages, "messages", false, "also pull recent messages per dialog into the mirror")
	cmd.Flags().IntVar(&perChat, "per-chat", 50, "messages to pull per dialog with --messages")
	addTelegramFlags(cmd, telegramCmdFlags{Limit: 100})
	return cmd
}

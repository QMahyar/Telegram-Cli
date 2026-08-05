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

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "sync [chat]",
		Short: "Sync dialogs (or messages from a specific chat) to the local mirror",
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
			alias, err := resolveAccount(ctx, s, "")
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
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
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d dialogs for %s.\n", len(dialogs), alias)
				return nil
			})
			return err
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "path to SQLite database (default: auto-detected)")
	addTelegramFlags(cmd, telegramCmdFlags{Limit: 100})
	return cmd
}

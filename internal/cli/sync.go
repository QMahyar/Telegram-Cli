package cli

import (
	"context"
	"fmt"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"
	"telegram-cli/internal/store"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "sync [chat]",
		Short: "Sync dialogs (or messages from a specific chat) to the local mirror",
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
				dialogs, err := mtproto.GetDialogs(ctx, api, f.Limit)
				if err != nil {
					return err
				}
				// Save dialogs and peers to the mirror
				for _, d := range dialogs {
					s.DB().ExecContext(ctx,
						`INSERT OR REPLACE INTO tg_dialogs (account, peer_type, peer_id, title, username, unread_count, last_msg_id, pinned)
						 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
						alias, d.PeerType, d.PeerID, d.Title, d.Username, d.Unread, d.LastMsgID, d.Pinned,
					)
					s.DB().ExecContext(ctx,
						`INSERT OR REPLACE INTO tg_peers (account, peer_type, peer_id, title, username)
						 VALUES (?, ?, ?, ?, ?)`,
						alias, d.PeerType, d.PeerID, d.Title, d.Username,
					)
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

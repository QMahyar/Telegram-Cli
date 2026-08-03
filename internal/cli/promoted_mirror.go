// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written replacement: queries the local SQLite mirror instead of an HTTP endpoint.

package cli

import (
	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

func newMirrorPromotedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Show local mirror stats: per-account message counts, chats, db size, sync age",
		Example: `  tele mirror
  tele mirror --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			type mirrorStats struct {
				Accounts  int `json:"accounts"`
				Dialogs   int `json:"dialogs"`
				Messages  int `json:"messages"`
				Jobs      int `json:"jobs"`
				Peers     int `json:"peers"`
				Cooldowns int `json:"cooldowns"`
				AuditRows int `json:"audit_rows"`
			}
			var ms mirrorStats
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_accounts`).Scan(&ms.Accounts)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_dialogs`).Scan(&ms.Dialogs)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_messages`).Scan(&ms.Messages)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_jobs`).Scan(&ms.Jobs)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_peers`).Scan(&ms.Peers)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_cooldowns`).Scan(&ms.Cooldowns)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_audit`).Scan(&ms.AuditRows)

			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, ms)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

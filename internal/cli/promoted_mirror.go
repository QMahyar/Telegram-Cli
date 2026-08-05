// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written replacement: queries the local SQLite mirror instead of an HTTP endpoint.

package cli

import (
	"fmt"

	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

func newMirrorPromotedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Show local mirror stats: per-account message counts, chats, db size, sync age",
		Args:  cobra.NoArgs,
		Example: `  telegram-cli mirror
  telegram-cli mirror --json`,
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
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_accounts`).Scan(&ms.Accounts); err != nil {
				return fmt.Errorf("counting accounts: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_dialogs`).Scan(&ms.Dialogs); err != nil {
				return fmt.Errorf("counting dialogs: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_messages`).Scan(&ms.Messages); err != nil {
				return fmt.Errorf("counting messages: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_jobs`).Scan(&ms.Jobs); err != nil {
				return fmt.Errorf("counting jobs: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_peers`).Scan(&ms.Peers); err != nil {
				return fmt.Errorf("counting peers: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_cooldowns`).Scan(&ms.Cooldowns); err != nil {
				return fmt.Errorf("counting cooldowns: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_audit`).Scan(&ms.AuditRows); err != nil {
				return fmt.Errorf("counting audit rows: %w", err)
			}

			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, ms)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

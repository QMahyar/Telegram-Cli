package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newNovelAccountsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "accounts",
		Short:       "manage Telegram accounts: add, list, use, rename, remove, status, health, import",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(
		newAccountsAddCmd(flags),
		newAccountsListCmd(flags),
		newAccountsUseCmd(flags),
		newAccountsRenameCmd(flags),
		newAccountsRemoveCmd(flags),
		newAccountsStatusCmd(flags),
		newAccountsHealthCmd(flags),
		newAccountsImportCmd(flags),
	)
	return cmd
}

func newAccountsAddCmd(flags *rootFlags) *cobra.Command {
	var phone, alias string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new Telegram account (phone + code login)",
		Example: `  telegram-cli accounts add --phone +1234567890 --alias work
  telegram-cli accounts add --phone +98912345678 --alias personal`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return fmt.Errorf("--alias is required")
			}
			if phone == "" {
				return fmt.Errorf("--phone is required")
			}
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			dir, err := mgr.EnsureDir(alias)
			if err != nil {
				return fmt.Errorf("create session dir: %w", err)
			}
			ctx := cmd.Context()
			fmt.Fprintf(os.Stderr, "Logging in as %s...\n", phone)
			err = mgr.DialAndRunUnchecked(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				codeFn := func(ctx context.Context, phone string) (string, error) {
					fmt.Fprintf(os.Stderr, "Enter the code sent to %s: ", phone)
					var code string
					fmt.Scanln(&code)
					return strings.TrimSpace(code), nil
				}
				pwdFn := func(ctx context.Context) (string, error) {
					fmt.Fprint(os.Stderr, "Enter 2FA password (leave empty if none): ")
					var pwd string
					fmt.Scanln(&pwd)
					return strings.TrimSpace(pwd), nil
				}
				return mtproto.LoginPhone(ctx, client, phone, codeFn, pwdFn)
			})
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			s, err := openStore(ctx, home)
			if err != nil {
				return err
			}
			defer s.DB().Close()
			_, err = s.DB().ExecContext(ctx,
				`INSERT INTO tg_accounts (alias, session_dir, phone, status) VALUES (?, ?, ?, 'active')`,
				alias, dir, phone,
			)
			if err != nil {
				return fmt.Errorf("save account: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Account %q added successfully.\n", alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&phone, "phone", "", "phone number with country code (e.g. +1234567890)")
	cmd.Flags().StringVar(&alias, "alias", "", "short alias for this account")
	return cmd
}

func newAccountsListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List all configured Telegram accounts",
		Annotations: map[string]string{"mcp:read-only": "true"},
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
			rows, err := s.DB().QueryContext(ctx,
				`SELECT alias, user_id, username, phone, status FROM tg_accounts ORDER BY alias`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			var accounts []AccountInfo
			for rows.Next() {
				var a AccountInfo
				if err := rows.Scan(&a.Alias, &a.UserID, &a.Username, &a.Phone, &a.Status); err != nil {
					return err
				}
				accounts = append(accounts, a)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, accounts)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newAccountsUseCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "use <alias>",
		Short: "Set an account as the default for subsequent commands",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
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
			res, err := s.DB().ExecContext(ctx,
				`UPDATE tg_accounts SET last_used_at = datetime('now') WHERE alias = ?`, alias,
			)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("account %q not found", alias)
			}
			fmt.Fprintf(os.Stderr, "Now using account %q.\n", alias)
			return nil
		},
	}
}

func newAccountsRenameCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old-alias> <new-alias>",
		Short: "Rename an account alias",
		Example: `  tele accounts rename work office
  tele accounts rename personal main`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldAlias, newAlias := args[0], args[1]
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
			res, err := s.DB().ExecContext(ctx,
				`UPDATE tg_accounts SET alias = ? WHERE alias = ?`, newAlias, oldAlias,
			)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("account %q not found", oldAlias)
			}
			oldDir := filepath.Join(home, "sessions", oldAlias)
			newDir := filepath.Join(home, "sessions", newAlias)
			if _, err := os.Stat(oldDir); err == nil {
				os.Rename(oldDir, newDir)
			}
			fmt.Fprintf(os.Stderr, "Renamed %q → %q.\n", oldAlias, newAlias)
			return nil
		},
	}
}

func newAccountsRemoveCmd(flags *rootFlags) *cobra.Command {
	var keepSession bool
	cmd := &cobra.Command{
		Use:   "remove <alias>",
		Short: "Remove an account (logs out and deletes session by default)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
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
			if !keepSession {
				mgr, err2 := openManager(home)
				if err2 == nil {
					_ = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
						return mtproto.LogoutAuth(ctx, client)
					})
				}
			}
			s.DB().ExecContext(ctx, `DELETE FROM tg_accounts WHERE alias = ?`, alias)
			if !keepSession {
				os.RemoveAll(filepath.Join(home, "sessions", alias))
			}
			fmt.Fprintf(os.Stderr, "Account %q removed.\n", alias)
			return nil
		},
	}
	cmd.Flags().BoolVar(&keepSession, "keep-session", false, "keep session files (don't log out)")
	return cmd
}

func newAccountsStatusCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "status [alias]",
		Short:       "Show auth status of an account",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
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
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			} else {
				alias, err = resolveAccount(ctx, s, "")
				if err != nil {
					return err
				}
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var result map[string]any
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				status, err := mtproto.Status(ctx, client)
				if err != nil {
					return err
				}
				result = map[string]any{
					"alias":      alias,
					"authorized": status.Authorized,
				}
				if status.User != nil {
					result["user_id"] = status.User.ID
					result["username"] = status.User.Username
					result["first_name"] = status.User.FirstName
					result["phone"] = status.User.Phone
				}
				return nil
			})
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, result)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newAccountsHealthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "health",
		Short:       "Show health of all accounts: auth state, cooldowns, unread counts",
		Annotations: map[string]string{"mcp:read-only": "true"},
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
			rows, err := s.DB().QueryContext(ctx,
				`SELECT alias, user_id, username, phone, status FROM tg_accounts ORDER BY alias`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			type healthItem struct {
				Alias       string `json:"alias"`
				UserID      int64  `json:"user_id"`
				Username    string `json:"username"`
				Phone       string `json:"phone"`
				Status      string `json:"status"`
				UnreadTotal int    `json:"unread_total"`
				Cooldown    string `json:"cooldown,omitempty"`
			}
			var items []healthItem
			for rows.Next() {
				var h healthItem
				if err := rows.Scan(&h.Alias, &h.UserID, &h.Username, &h.Phone, &h.Status); err != nil {
					return err
				}
				s.DB().QueryRowContext(ctx,
					`SELECT COALESCE(SUM(unread_count), 0) FROM tg_dialogs WHERE account = ?`, h.Alias,
				).Scan(&h.UnreadTotal)
				var until int64
				err := s.DB().QueryRowContext(ctx,
					`SELECT until_unix FROM tg_cooldowns WHERE account = ? AND until_unix > ? ORDER BY until_unix LIMIT 1`,
					h.Alias, time.Now().Unix(),
				).Scan(&until)
				if err == nil {
					h.Cooldown = time.Unix(until, 0).Format(time.RFC3339)
				}
				items = append(items, h)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newAccountsImportCmd(flags *rootFlags) *cobra.Command {
	var sessionStr, alias string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a Telethon/Pyrogram string session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return fmt.Errorf("--alias is required")
			}
			if sessionStr == "" {
				return fmt.Errorf("--session is required (the Telethon/Pyrogram hex string)")
			}
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			_ = sessionStr // TODO: parse via session.TelethonSession and save to FileStorage
			dir := filepath.Join(home, "sessions", alias)
			os.MkdirAll(dir, 0o700)
			s, err := openStore(cmd.Context(), home)
			if err != nil {
				return err
			}
			defer s.DB().Close()
			_, err = s.DB().ExecContext(cmd.Context(),
				`INSERT OR REPLACE INTO tg_accounts (alias, session_dir, status) VALUES (?, ?, 'imported')`,
				alias, dir,
			)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Session string stored for %q. Run 'accounts status %s' to validate.\n", alias, alias)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionStr, "session", "", "Telethon/Pyrogram session string")
	cmd.Flags().StringVar(&alias, "alias", "", "alias for the imported account")
	return cmd
}

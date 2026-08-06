package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

// sanitizeAlias removes path separators and other dangerous characters from
// account aliases to prevent path traversal attacks.
func sanitizeAlias(alias string) string {
	alias = strings.ReplaceAll(alias, "/", "")
	alias = strings.ReplaceAll(alias, "\\", "")
	alias = strings.ReplaceAll(alias, "..", "")
	alias = strings.TrimSpace(alias)
	return alias
}

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
	var phone, alias, code, password string
	var useQR bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new Telegram account (phone + code login, or --qr)",
		Example: `  telegram-cli accounts add --phone +1234567890 --alias work
  telegram-cli accounts add --phone +98912345678 --alias personal
  telegram-cli accounts add --qr --alias work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return fmt.Errorf("--alias is required")
			}
			alias = sanitizeAlias(alias)
			if alias == "" {
				return fmt.Errorf("--alias is empty after sanitization")
			}
			if !useQR && phone == "" {
				return fmt.Errorf("--phone is required (or use --qr to log in by scanning a code)")
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
			var authedUser *tg.User
			var nearestDC int
			if useQR {
				if flags.noInput {
					return usageErr(fmt.Errorf("QR login requires a human to scan the code; --no-input cannot complete it"))
				}
				fmt.Fprintf(os.Stderr, "Scan the QR code with your Telegram app (Settings → Devices → Link Desktop Device)...\n")
				dispatcher := tg.NewUpdateDispatcher()
				err = mgr.DialAndRunUncheckedWithUpdates(ctx, alias, dispatcher, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					if err := mtproto.LoginQR(ctx, client, dispatcher, func(ctx context.Context, url string) error {
						fmt.Fprintf(os.Stderr, "Scan this QR code: %s\n", url)
						return nil
					}); err != nil {
						return err
					}
					// Backfill the account row with the authed user's identity so
					// `accounts list` never shows a stale user_id 0 / empty
					// username "active" row (P0-2).
					st, err := mtproto.Status(ctx, client)
					if err != nil {
						return fmt.Errorf("fetch authed user: %w", err)
					}
					authedUser = st.User
					dc, err := api.HelpGetNearestDC(ctx)
					if err == nil {
						nearestDC = dc.ThisDC
					}
					return nil
				})
				if err != nil {
					return fmt.Errorf("QR login failed: %w", err)
				}
				fmt.Fprintf(os.Stderr, "QR login succeeded for %q.\n", alias)
			} else {
				fmt.Fprintf(os.Stderr, "Logging in as %s...\n", phone)
				err = mgr.DialAndRunUnchecked(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					codeFn := func(ctx context.Context, phone string) (string, error) {
						if code != "" {
							return code, nil
						}
						if flags.noInput {
							return "", fmt.Errorf("login code required: pass --code to log in non-interactively, or remove --no-input")
						}
						fmt.Fprintf(os.Stderr, "Enter the code sent to %s: ", phone)
						var c string
						if _, err := fmt.Scanln(&c); err != nil {
							return "", fmt.Errorf("reading login code from stdin: %w (pass --code to log in non-interactively)", err)
						}
						return strings.TrimSpace(c), nil
					}
					pwdFn := func(ctx context.Context) (string, error) {
						if password != "" {
							return password, nil
						}
						if flags.noInput {
							return "", fmt.Errorf("2FA password required: pass --password to log in non-interactively, or remove --no-input")
						}
						fmt.Fprint(os.Stderr, "Enter 2FA password (leave empty if none): ")
						var pwd string
						if _, err := fmt.Scanln(&pwd); err != nil {
							return "", fmt.Errorf("reading 2FA password from stdin: %w (pass --password to log in non-interactively)", err)
						}
						return strings.TrimSpace(pwd), nil
					}
					if err := mtproto.LoginPhone(ctx, client, phone, codeFn, pwdFn); err != nil {
						return err
					}
					// Backfill identity (P0-2), same as the QR path.
					st, err := mtproto.Status(ctx, client)
					if err != nil {
						return fmt.Errorf("fetch authed user: %w", err)
					}
					authedUser = st.User
					dc, err := api.HelpGetNearestDC(ctx)
					if err == nil {
						nearestDC = dc.ThisDC
					}
					return nil
				})
				if err != nil {
					return fmt.Errorf("login failed: %w", err)
				}
			}
			s, err := openStore(ctx, home)
			if err != nil {
				return err
			}
			defer s.DB().Close()
			_, err = s.DB().ExecContext(ctx,
				`INSERT OR REPLACE INTO tg_accounts (alias, session_dir, phone, status) VALUES (?, ?, ?, 'active')`,
				alias, dir, phone,
			)
			if err != nil {
				return fmt.Errorf("save account: %w", err)
			}
			// Backfill the authed user's identity so `accounts list`/`health`
			// show real user_id/username instead of a stale "active" placeholder.
			if authedUser != nil {
				if _, err := s.DB().ExecContext(ctx,
					`UPDATE tg_accounts SET user_id = ?, username = ?, first_name = ?, dc_id = ? WHERE alias = ?`,
					authedUser.ID, authedUser.Username, authedUser.FirstName, nearestDC, alias,
				); err != nil {
					return fmt.Errorf("backfill account identity: %w", err)
				}
			}
			// Machine-readable success on stdout; human prose on stderr only.
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, map[string]any{
				"alias":  alias,
				"status": "active",
			})
		},
	}
	cmd.Flags().StringVar(&phone, "phone", "", "phone number with country code (e.g. +1234567890)")
	cmd.Flags().StringVar(&alias, "alias", "", "short alias for this account")
	cmd.Flags().StringVar(&code, "code", "", "login code sent to the phone (non-interactive; skip to be prompted)")
	cmd.Flags().BoolVar(&useQR, "qr", false, "log in by scanning a QR code with the Telegram app")
	cmd.Flags().StringVar(&password, "password", "", "2FA cloud password (non-interactive; skip to be prompted)")
	addTelegramFlags(cmd)
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
				// A row created at `accounts add` time (before login completed)
				// carries user_id 0 — never report it as a healthy "active"
				// account (P0-2).
				if a.UserID == 0 {
					a.Status = "unverified"
				}
				accounts = append(accounts, a)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading accounts: %w", err)
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
			alias := sanitizeAlias(args[0])
			if alias == "" {
				return fmt.Errorf("alias is empty after sanitization")
			}
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
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"alias": alias, "using": true})
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
			oldAlias, newAlias := sanitizeAlias(args[0]), sanitizeAlias(args[1])
			if oldAlias == "" || newAlias == "" {
				return fmt.Errorf("aliases are empty after sanitization")
			}
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
				if err := os.Rename(oldDir, newDir); err != nil {
					return fmt.Errorf("renaming session directory %q to %q: %w", oldDir, newDir, err)
				}
			}
			fmt.Fprintf(os.Stderr, "Renamed %q → %q.\n", oldAlias, newAlias)
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"from": oldAlias, "to": newAlias})
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
			alias := sanitizeAlias(args[0])
			if alias == "" {
				return fmt.Errorf("alias is empty after sanitization")
			}
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
			if _, err := s.DB().ExecContext(ctx, `DELETE FROM tg_accounts WHERE alias = ?`, alias); err != nil {
				return fmt.Errorf("removing account %q from local registry: %w", alias, err)
			}
			if !keepSession {
				os.RemoveAll(filepath.Join(home, "sessions", alias))
			}
			fmt.Fprintf(os.Stderr, "Account %q removed.\n", alias)
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"alias": alias, "removed": true})
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
			f := parseTelegramFlags(cmd)
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			} else {
				alias, err = resolveAccount(ctx, s, f.Account)
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
			return outResult(stdout(), f, result)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newAccountsHealthCmd(flags *rootFlags) *cobra.Command {
	var probe bool
	cmd := &cobra.Command{
		Use:         "health",
		Short:       "Show health of all accounts: auth state, cooldowns, unread counts",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  telegram-cli accounts health
  telegram-cli accounts health --probe      # dial every account live to verify auth
  telegram-cli accounts health --probe --json`,
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
				Alias           string `json:"alias"`
				UserID          int64  `json:"user_id"`
				Username        string `json:"username"`
				Phone           string `json:"phone"`
				Status          string `json:"status"`
				UnreadTotal     int    `json:"unread_total"`
				Cooldown        string `json:"cooldown,omitempty"`
				CooldownSeconds int    `json:"cooldown_seconds,omitempty"`
				CooldownKind    string `json:"cooldown_kind,omitempty"`
				SessionFresh    string `json:"session_fresh,omitempty"`
				SessionAge      string `json:"session_age,omitempty"`
				Probe           string `json:"probe,omitempty"`
				ProbeAuthorized bool   `json:"probe_authorized,omitempty"`
			}
			var items []healthItem
			for rows.Next() {
				var h healthItem
				if err := rows.Scan(&h.Alias, &h.UserID, &h.Username, &h.Phone, &h.Status); err != nil {
					return err
				}
				if h.UserID == 0 {
					h.Status = "unverified"
				}
				if err := s.DB().QueryRowContext(ctx,
					`SELECT COALESCE(SUM(unread_count), 0) FROM tg_dialogs WHERE account = ?`, h.Alias,
				).Scan(&h.UnreadTotal); err != nil {
					return fmt.Errorf("reading unread total for %q: %w", h.Alias, err)
				}
				var until int64
				var cooldownSeconds int
				var cooldownKind string
				err := s.DB().QueryRowContext(ctx,
					`SELECT until_unix, seconds, kind FROM tg_cooldowns WHERE account = ? AND until_unix > ? ORDER BY until_unix LIMIT 1`,
					h.Alias, time.Now().Unix(),
				).Scan(&until, &cooldownSeconds, &cooldownKind)
				if errors.Is(err, sql.ErrNoRows) {
					h.Cooldown = ""
				} else if err != nil {
					return fmt.Errorf("reading cooldown for %q: %w", h.Alias, err)
				} else {
					h.Cooldown = time.Unix(until, 0).Format(time.RFC3339)
					h.CooldownSeconds = cooldownSeconds
					h.CooldownKind = cooldownKind
				}
				// Session freshness: mtime of the session file, when present.
				if st, err := os.Stat(filepath.Join(home, "sessions", h.Alias, "session.json")); err == nil {
					h.SessionFresh = st.ModTime().Format(time.RFC3339)
					h.SessionAge = time.Since(st.ModTime()).Truncate(time.Second).String()
				}
				items = append(items, h)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading account health: %w", err)
			}
			// --probe: dial each account live and verify the session is still
			// authorized (auth.Status). A revoked/expired session reports
			// probe=error; a healthy one reports probe_authorized=true.
			if probe {
				mgr, err := openManager(home)
				if err != nil {
					return err
				}
				for i := range items {
					alias := items[i].Alias
					probeErr := mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
						st, err := mtproto.Status(ctx, client)
						if err != nil {
							return err
						}
						items[i].ProbeAuthorized = st.Authorized
						if !st.Authorized {
							return fmt.Errorf("session not authorized")
						}
						return nil
					})
					if probeErr != nil {
						items[i].Probe = fmt.Sprintf("error: %v", probeErr)
					} else {
						items[i].Probe = "ok"
					}
				}
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
	cmd.Flags().BoolVar(&probe, "probe", false, "dial each account live to verify the session is still authorized")
	addTelegramFlags(cmd)
	return cmd
}

// newAccountsImportCmd imports a Telethon/Pyrogram string session: it decodes
// the string with gotd's session.TelethonSession, writes the session in the
// same FileStorage format the live login flow uses, and registers the alias in
// the local store. A decoded session is required before anything is written so
// a malformed string can never leave a half-valid session file behind.
func newAccountsImportCmd(flags *rootFlags) *cobra.Command {
	var sessionStr, alias string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a Telethon/Pyrogram string session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return fmt.Errorf("--alias is required")
			}
			alias = sanitizeAlias(alias)
			if alias == "" {
				return fmt.Errorf("--alias is empty after sanitization")
			}
			if sessionStr == "" {
				return fmt.Errorf("--session is required (the Telethon/Pyrogram hex string)")
			}
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			// Decode first: the string is the only source of truth for the
			// imported session, and a bad decode must not leave a file behind.
			data, err := session.TelethonSession(sessionStr)
			if err != nil {
				return validationErr(fmt.Errorf("invalid session string: %w", err), "session")
			}
			dir := filepath.Join(home, "sessions", alias)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("create session dir: %w", err)
			}
			sessionPath := filepath.Join(dir, "session.json")
			loader := session.Loader{Storage: &session.FileStorage{Path: sessionPath}}
			if err := loader.Save(cmd.Context(), data); err != nil {
				return fmt.Errorf("save imported session: %w", err)
			}
			s, err := openStore(cmd.Context(), home)
			if err != nil {
				return err
			}
			defer s.DB().Close()
			_, err = s.DB().ExecContext(cmd.Context(),
				`INSERT OR REPLACE INTO tg_accounts (alias, session_dir, status) VALUES (?, ?, 'active')`,
				alias, dir,
			)
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, map[string]any{
				"alias":        alias,
				"session_dir":  dir,
				"dc_id":        data.DC,
				"addr":         data.Addr,
				"status":       "active",
				"instructions": "run 'accounts status " + alias + "' to validate the imported session",
			})
		},
	}
	cmd.Flags().StringVar(&sessionStr, "session", "", "Telethon/Pyrogram session string")
	cmd.Flags().StringVar(&alias, "alias", "", "alias for the imported account")
	addTelegramFlags(cmd)
	return cmd
}

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"
	"telegram-cli/internal/store"

	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

// telegramCmdFlags holds the common flags shared across telegram commands.
type telegramCmdFlags struct {
	Account string
	JSON    bool
	Human   bool
	Limit   int
	Select  string
	Agent   bool
}

func addTelegramFlags(cmd *cobra.Command, defaults ...telegramCmdFlags) {
	def := telegramCmdFlags{Limit: 30}
	if len(defaults) > 0 {
		def = defaults[0]
	}
	cmd.Flags().StringP("account", "a", "", "account alias (uses last-used if omitted)")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("human", false, "output as human-readable table (default)")
	cmd.Flags().IntP("limit", "n", def.Limit, "max items to return")
}

func parseTelegramFlags(cmd *cobra.Command) telegramCmdFlags {
	f := telegramCmdFlags{}
	f.Account, _ = cmd.Flags().GetString("account")
	f.JSON, _ = cmd.Flags().GetBool("json")
	f.Human, _ = cmd.Flags().GetBool("human")
	f.Limit, _ = cmd.Flags().GetInt("limit")
	// --select is a root persistent flag; honor it here so the core Telegram
	// read commands (chats, messages, contacts, ...) filter fields the same
	// way the scaffolded commands do via printJSONFiltered, instead of
	// silently ignoring --select.
	f.Select, _ = cmd.Flags().GetString("select")
	// --agent is a root persistent flag; honor it here so telegram commands
	// emit the {ok,data,metadata} success envelope (and structured errors)
	// for agent consumption.
	f.Agent, _ = cmd.Flags().GetBool("agent")
	return f
}

// liveResolver builds a PeerResolver whose @username lookups fall back to the
// live session (contacts.resolveUsername) on cache miss, and persist the
// resolved access hash for future offline lookups. Call it inside DialAndRun.
func liveResolver(db *sql.DB, api *tg.Client) *mtproto.PeerResolver {
	r := mtproto.NewPeerResolver(db)
	r.Live = func(ctx context.Context, username string) (tg.InputPeerClass, error) {
		return mtproto.ResolveUsernameLive(ctx, api, username)
	}
	return r
}

// openManager creates an mtproto.Manager using the resolved home directory.
func openManager(home string) (*mtproto.Manager, error) {
	return mtproto.NewManager(home)
}

// openStore opens the mirror SQLite database and ensures the Telegram schema.
func openStore(ctx context.Context, home string) (*store.Store, error) {
	dbPath := config.DefaultDBPath(home)
	return openStoreAtPath(ctx, dbPath)
}

// openStoreAtPath opens a SQLite database at the specified path and ensures the Telegram schema.
func openStoreAtPath(ctx context.Context, dbPath string) (*store.Store, error) {
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := store.EnsureTelegramSchema(ctx, s); err != nil {
		s.DB().Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return s, nil
}

// resolveAccount resolves the account alias from flag or finds the last-used account.
func resolveAccount(ctx context.Context, s *store.Store, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	var alias string
	err := s.DB().QueryRowContext(ctx,
		`SELECT alias FROM tg_accounts WHERE status = 'active' ORDER BY last_used_at DESC LIMIT 1`,
	).Scan(&alias)
	if err != nil {
		return "", fmt.Errorf("no account specified and no active accounts — use --account or run: accounts add")
	}
	return alias, nil
}

// markAccountUsed updates the last_used_at timestamp for the given alias.
func markAccountUsed(ctx context.Context, s *store.Store, alias string) {
	s.DB().ExecContext(ctx,
		`UPDATE tg_accounts SET last_used_at = datetime('now') WHERE alias = ?`, alias,
	)
}

// outResult writes the result to the given writer in the requested format.
// In JSON mode it honors the inherited --select flag (comma-separated field
// paths) so the core Telegram read commands keep the "Filterable" contract
// advertised in the README/SKILL instead of silently ignoring --select.
func outResult(w io.Writer, flags telegramCmdFlags, v any) error {
	if flags.JSON || !flags.Human {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if flags.Select != "" {
			raw = filterFields(raw, flags.Select)
		}
		// --agent mode nests the payload under {ok, data, metadata} so agents
		// parse one uniform shape (.data) across every command. --json output
		// stays raw for scripts and tests.
		if flags.Agent || argsAgentRequested(os.Args[1:]) {
			raw = wrapSuccess(raw, "telegram")
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
			return err
		}
		buf.WriteByte('\n')
		_, err = w.Write(buf.Bytes())
		return err
	}
	return outTable(w, v)
}

// outTable formats a slice of items as a human-readable table.
func outTable(w io.Writer, v any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	switch items := v.(type) {
	case []mtproto.DialogItem:
		fmt.Fprintf(tw, "TYPE\tID\tTITLE\tUNREAD\tPINNED\n")
		for _, d := range items {
			pinned := ""
			if d.Pinned {
				pinned = "✓"
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\n", d.PeerType, d.PeerID, d.Title, d.Unread, pinned)
		}
	case []mtproto.MessageItem:
		fmt.Fprintf(tw, "ID\tSENDER\tTEXT\tMEDIA\n")
		for _, m := range items {
			text := m.Text
			if len(text) > 50 {
				text = text[:47] + "..."
			}
			text = strings.ReplaceAll(text, "\n", " ")
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", m.MsgID, m.Sender, text, m.Media)
		}
	case []AccountInfo:
		fmt.Fprintf(tw, "ALIAS\tUSER ID\tUSERNAME\tPHONE\tSTATUS\n")
		for _, a := range items {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", a.Alias, a.UserID, a.Username, a.Phone, a.Status)
		}
	default:
		enc := json.NewEncoder(tw)
		enc.Encode(v)
	}
	return tw.Flush()
}

// AccountInfo is the output type for accounts list.
type AccountInfo struct {
	Alias    string `json:"alias"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Phone    string `json:"phone"`
	Status   string `json:"status"`
}

// stdout returns os.Stdout as an io.Writer.
func stdout() io.Writer { return os.Stdout }

// stderr returns os.Stderr as an io.Writer.
func stderr() io.Writer { return os.Stderr }

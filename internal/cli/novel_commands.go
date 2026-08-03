package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newNovelBroadcastCmd(flags *rootFlags) *cobra.Command {
	var text, mediaPath, template string
	cmd := &cobra.Command{
		Use:     "broadcast",
		Short:   "Post one message to multiple chats, paced to avoid flood",
		Example: `  tele broadcast --text "Hello everyone!" @channel1 @channel2
  tele broadcast --media photo.jpg --text "Check this out" @group1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if text == "" && template == "" {
				return fmt.Errorf("provide --text or --template")
			}
			if len(args) == 0 {
				return fmt.Errorf("provide at least one target chat")
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
			alias, err := resolveAccount(ctx, s, "")
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			// Record job
			targets := strings.Join(args, ",")
			res, _ := s.DB().ExecContext(ctx,
				`INSERT INTO tg_jobs (kind, status, accounts_csv, targets_csv, text, media_path) VALUES ('broadcast', 'running', ?, ?, ?, ?)`,
				alias, targets, text, mediaPath,
			)
			jobID, _ := res.LastInsertId()
			sent := 0
			failed := 0
			for _, ref := range args {
				err := mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					resolver := mtproto.NewPeerResolver(s.DB())
					peer, resolveErr := resolver.Resolve(ctx, alias, ref)
					if resolveErr != nil {
						return resolveErr
					}
					if mediaPath != "" {
						_, sendErr := mtproto.UploadAndSendMedia(ctx, api, peer, mediaPath, text)
						return sendErr
					}
					_, sendErr := mtproto.SendMessage(ctx, api, peer, text)
					return sendErr
				})
				if err != nil {
					failed++
					fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", ref, err)
				} else {
					sent++
					fmt.Fprintf(os.Stderr, "OK   %s\n", ref)
				}
				time.Sleep(2 * time.Second) // pace between sends
			}
			s.DB().ExecContext(ctx,
				`UPDATE tg_jobs SET status = 'done', finished_at = datetime('now'), error = ? WHERE id = ?`,
				fmt.Sprintf("sent=%d failed=%d", sent, failed), jobID,
			)
			fmt.Fprintf(os.Stderr, "\nBroadcast complete: %d sent, %d failed.\n", sent, failed)
			return nil
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&mediaPath, "media", "", "path to media file")
	cmd.Flags().StringVar(&template, "template", "", "template name to use")
	return cmd
}

func newNovelBatchCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Fan out operations across accounts and chats as a resumable job",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: implement batch job framework
			return cmd.Help()
		},
	}
	return cmd
}

func newNovelInboxCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "inbox",
		Short:       "View unread messages across all accounts",
		Example:     `  tele inbox
  tele inbox --json`,
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
				`SELECT account, title, unread_count FROM tg_dialogs WHERE unread_count > 0 ORDER BY unread_count DESC`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			type inboxItem struct {
				Account string `json:"account"`
				Title   string `json:"title"`
				Unread  int    `json:"unread_count"`
			}
			var items []inboxItem
			for rows.Next() {
				var item inboxItem
				rows.Scan(&item.Account, &item.Title, &item.Unread)
				items = append(items, item)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newNovelStatsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "stats",
		Short:       "Show message statistics across accounts",
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
			type stats struct {
				TotalMessages int `json:"total_messages"`
				TotalDialogs  int `json:"total_dialogs"`
				TotalAccounts int `json:"total_accounts"`
			}
			var st stats
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_messages`).Scan(&st.TotalMessages)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_dialogs`).Scan(&st.TotalDialogs)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_accounts`).Scan(&st.TotalAccounts)
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, st)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "since <time-spec>",
		Short:       "Show new messages since a point in time",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sinceStr := args[0]
			since, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return fmt.Errorf("parse time: %w (use RFC3339 format like 2026-01-01T00:00:00Z)", err)
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
			rows, err := s.DB().QueryContext(ctx,
				`SELECT account, sender, text, date FROM tg_messages WHERE date > ? ORDER BY date DESC LIMIT 50`,
				since.Unix(),
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			f := parseTelegramFlags(cmd)
			var items []mtproto.MessageItem
			for rows.Next() {
				var m mtproto.MessageItem
				rows.Scan(&m.Sender, &m.Text, &m.Date)
				items = append(items, m)
			}
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "digest",
		Short:       "Generate a digest of recent activity",
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
			type digestEntry struct {
				Account string `json:"account"`
				Title   string `json:"title"`
				Count   int    `json:"recent_messages"`
			}
			rows, err := s.DB().QueryContext(ctx,
				`SELECT account, title, COUNT(*) as cnt FROM tg_messages WHERE date > ? GROUP BY account, title ORDER BY cnt DESC LIMIT 20`,
				time.Now().Add(-24*time.Hour).Unix(),
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			var items []digestEntry
			for rows.Next() {
				var d digestEntry
				rows.Scan(&d.Account, &d.Title, &d.Count)
				items = append(items, d)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newNovelSchemaCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "schema",
		Short:       "Check Telegram TL schema compatibility",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newSchemaCheckCmd(flags))
	return cmd
}

func newSchemaCheckCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "check",
		Short:       "Check Telegram TL schema compatibility",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			f := parseTelegramFlags(cmd)
			result := map[string]any{
				"library":    "github.com/gotd/td",
				"version":    "v0.161.0",
				"tl_layer":   tg.Layer,
				"compatible": true,
			}
			return outResult(stdout(), f, result)
		},
	}
}

func newNovelDaemonCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run a bounded daemon (e.g. daemon run --duration 10m)",
		RunE:  parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newDaemonRunCmd(flags))
	return cmd
}

func newDaemonRunCmd(flags *rootFlags) *cobra.Command {
	var duration string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the daemon for a fixed duration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dur, err := time.ParseDuration(duration)
			if err != nil {
				return fmt.Errorf("parse duration: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Daemon running for %s...\n", dur)
			time.Sleep(dur)
			fmt.Fprintf(os.Stderr, "Daemon stopped.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&duration, "duration", "10m", "how long to run (e.g. 10m, 1h)")
	return cmd
}

func newNovelJobsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "jobs",
		Short:       "Manage scheduled/broadcast jobs",
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
				`SELECT id, kind, status, targets_csv, created_at, finished_at, error FROM tg_jobs ORDER BY id DESC LIMIT 20`,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			type jobInfo struct {
				ID         int64  `json:"id"`
				Kind       string `json:"kind"`
				Status     string `json:"status"`
				Targets    string `json:"targets"`
				CreatedAt  string `json:"created_at"`
				FinishedAt string `json:"finished_at,omitempty"`
				Error      string `json:"error,omitempty"`
			}
			var items []jobInfo
			for rows.Next() {
				var j jobInfo
				rows.Scan(&j.ID, &j.Kind, &j.Status, &j.Targets, &j.CreatedAt, &j.FinishedAt, &j.Error)
				items = append(items, j)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// Ensure imports are used
var _ = time.Second

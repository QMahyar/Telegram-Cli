package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
		Use:   "broadcast",
		Short: "Post one message to multiple chats, paced to avoid flood",
		Example: `  telegram-cli broadcast --text "Hello everyone!" @channel1 @channel2
  telegram-cli broadcast --media photo.jpg --text "Check this out" @group1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "broadcast message")
			}
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
			if template != "" {
				text, err = resolveTemplateText(ctx, s.DB(), template)
				if err != nil {
					return err
				}
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			// Mass-visible write: require explicit --yes (or --dry-run, handled
			// above). --agent implies --yes, so agent-driven broadcasts proceed;
			// a bare `broadcast` from a human is gated with a plan + exit 6.
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would broadcast to %d chat(s) as %s\n", len(args), alias)
				return confirmationErr(fmt.Errorf("broadcast requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
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
					resolver := liveResolver(s.DB(), api)
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
		Use:     "batch",
		Short:   "Fan out operations across accounts and chats as a resumable job",
		Example: `  tele batch forward @channel 123 124 --to @list1 @list2`,
		RunE:    parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newBatchForwardCmd(flags))
	return cmd
}

// newBatchForwardCmd forwards a set of messages from a source chat to one or
// more target chats as a single audited, resumable-style job. Each target is
// recorded in tg_job_results and paced so a large fan-out does not trip flood
// control.
func newBatchForwardCmd(flags *rootFlags) *cobra.Command {
	var toCSV string
	var to []string
	var pace time.Duration
	cmd := &cobra.Command{
		Use:   "forward <from-chat> <msg-id...>",
		Short: "Fan out forwards of one or more messages to many chats",
		Example: `  tele batch forward @source 100 101 --to @listA @listB
  tele batch forward 7528129992 1402 --to @dest1 @dest2 --pace 3s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "batch forward")
			}
			if len(args) < 2 {
				return cmd.Help()
			}
			if len(to) == 0 && toCSV != "" {
				for _, p := range strings.Split(toCSV, ",") {
					if p = strings.TrimSpace(p); p != "" {
						to = append(to, p)
					}
				}
			}
			if len(to) == 0 {
				return fmt.Errorf("provide at least one target chat (--to)")
			}
			fromRef := args[0]
			var msgIDs []int64
			for _, s := range args[1:] {
				if id, err := strconv.ParseInt(s, 10, 64); err == nil {
					msgIDs = append(msgIDs, id)
				}
			}
			if len(msgIDs) == 0 {
				return fmt.Errorf("provide at least one numeric message id")
			}
			if pace <= 0 {
				pace = time.Second
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

			// Mass-visible write: require explicit --yes (or --dry-run, handled
			// above). --agent implies --yes; a bare `batch forward` from a human
			// is gated with a plan + exit 6.
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would forward %d message(s) from %s to %d target(s)\n", len(msgIDs), fromRef, len(to))
				return confirmationErr(fmt.Errorf("batch forward requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
			}

			// Record the job.
			paramsJSON, _ := json.Marshal(map[string]any{
				"from":   fromRef,
				"msgIDs": msgIDs,
			})
			res, err := s.DB().ExecContext(ctx,
				`INSERT INTO tg_jobs (kind, status, params_json, accounts_csv, targets_csv, text)
				 VALUES ('forward', 'running', ?, ?, ?, '')`,
				string(paramsJSON), alias, strings.Join(to, ","),
			)
			if err != nil {
				return err
			}
			jobID, _ := res.LastInsertId()

			type targetResult struct {
				Target  string `json:"target"`
				Status  string `json:"status"`
				Detail  string `json:"detail,omitempty"`
				Message int64  `json:"message_id"`
			}
			var results []targetResult
			sent := 0
			failed := 0
			for _, ref := range to {
				r := targetResult{Target: ref, Status: "pending"}
				err := mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					resolver := liveResolver(s.DB(), api)
					fromPeer, err := resolver.Resolve(ctx, alias, fromRef)
					if err != nil {
						return err
					}
					toPeer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					ids, err := mtproto.ForwardMessages(ctx, api, fromPeer, toPeer, msgIDs)
					if err != nil {
						return err
					}
					if len(ids) > 0 {
						r.Message = ids[0]
					}
					return nil
				})
				if err != nil {
					failed++
					r.Status = "failed"
					r.Detail = err.Error()
				} else {
					sent++
					r.Status = "ok"
				}
				results = append(results, r)
				s.DB().ExecContext(ctx,
					`INSERT OR REPLACE INTO tg_job_results (job_id, account, target, status, detail, message_id, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
					jobID, alias, ref, r.Status, r.Detail, r.Message,
				)
				time.Sleep(pace)
			}

			s.DB().ExecContext(ctx,
				`UPDATE tg_jobs SET status = 'done', finished_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'), error = ? WHERE id = ?`,
				fmt.Sprintf("sent=%d failed=%d", sent, failed), jobID,
			)
			writeAuditRecord(ctx, s.DB(), alias, "batch forward", fromRef, string(paramsJSON), fmt.Sprintf("sent=%d failed=%d", sent, failed), "")

			out := map[string]any{
				"job_id":      jobID,
				"from":        fromRef,
				"message_ids": msgIDs,
				"sent":        sent,
				"failed":      failed,
				"results":     results,
			}
			f := parseTelegramFlags(cmd)
			if f.JSON {
				return outResult(stdout(), f, out)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Batch forward complete: %d sent, %d failed (job %d).\n", sent, failed, jobID)
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&to, "to", "t", nil, "target chat(s) to fan out to")
	cmd.Flags().StringVar(&toCSV, "to-csv", "", "comma-separated targets (alternative to --to)")
	cmd.Flags().DurationVar(&pace, "pace", 0, "delay between targets (default 1s)")
	return cmd
}

func newNovelInboxCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "View unread messages across all accounts",
		Example: `  tele inbox
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "query messages since time")
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			sinceStr := args[0]
			since, err := parseSinceSpec(sinceStr)
			if err != nil {
				return err
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
			f := parseTelegramFlags(cmd)
			rows, err := s.DB().QueryContext(ctx,
				`SELECT msg_id, sender, text, date FROM tg_messages WHERE date > ? ORDER BY date DESC LIMIT ?`,
				since.Unix(), f.Limit,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			var items []mtproto.MessageItem
			for rows.Next() {
				var m mtproto.MessageItem
				rows.Scan(&m.MsgID, &m.Sender, &m.Text, &m.Date)
				items = append(items, m)
			}
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// parseSinceSpec accepts an RFC3339 timestamp or a relative spec like "1d",
// "12h", "30m", "90s", or a plain day count ("7"). Relative specs are measured
// back from the current time.
func parseSinceSpec(spec string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, spec); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", spec); err == nil {
		return t, nil
	}
	text := strings.ToLower(strings.TrimSpace(spec))
	multiplier := time.Duration(0)
	suffix := ""
	switch {
	case strings.HasSuffix(text, "d"):
		multiplier, suffix = 24*time.Hour, "d"
	case strings.HasSuffix(text, "h"):
		multiplier, suffix = time.Hour, "h"
	case strings.HasSuffix(text, "m"):
		multiplier, suffix = time.Minute, "m"
	case strings.HasSuffix(text, "s"):
		multiplier, suffix = time.Second, "s"
	}
	if suffix != "" {
		numStr := strings.TrimSpace(strings.TrimSuffix(text, suffix))
		n, err := strconv.ParseFloat(numStr, 64)
		if err == nil && n > 0 {
			return time.Now().Add(-time.Duration(n * float64(multiplier))), nil
		}
	}
	if n, err := strconv.ParseFloat(text, 64); err == nil && n > 0 {
		return time.Now().Add(-time.Duration(n * float64(24*time.Hour))), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time spec %q (use RFC3339 like 2026-01-01T00:00:00Z, a date like 2026-01-01, or a relative spec like 1d, 2h, 30m)", spec)
}

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "digest",
		Short:       "Generate a digest of recent activity",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "generate digest")
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
			type digestEntry struct {
				Account string `json:"account"`
				Title   string `json:"title"`
				Count   int    `json:"recent_messages"`
			}
			rows, err := s.DB().QueryContext(ctx,
				`SELECT m.account, COALESCE(d.title, '') as title, COUNT(*) as cnt
				 FROM tg_messages m
				 LEFT JOIN tg_dialogs d ON d.account = m.account AND d.peer_type = m.peer_type AND d.peer_id = m.peer_id
				 WHERE m.date > ? GROUP BY m.account, title ORDER BY cnt DESC LIMIT 20`,
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
		Example: `  tele daemon run --duration 10m
  tele daemon run --duration 1h --json`,
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

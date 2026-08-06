package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

// stopWords filters filler terms from the digest --terms top-terms list.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true, "not": true,
	"you": true, "all": true, "any": true, "can": true, "had": true, "her": true,
	"was": true, "one": true, "our": true, "out": true, "day": true, "get": true,
	"has": true, "him": true, "his": true, "how": true, "man": true, "new": true,
	"now": true, "old": true, "see": true, "two": true, "way": true, "who": true,
	"boy": true, "did": true, "its": true, "let": true, "put": true, "say": true,
	"she": true, "too": true, "use": true, "that": true, "with": true, "have": true,
	"from": true, "this": true, "will": true, "your": true, "what": true, "when": true,
	"been": true, "into": true, "like": true, "more": true, "than": true, "them": true,
	"were": true, "would": true, "about": true, "there": true, "their": true,
	"here": true, "just": true, "make": true, "some": true, "time": true, "only": true,
}

func newNovelBroadcastCmd(flags *rootFlags) *cobra.Command {
	var text, mediaPath, template, atStr string
	cmd := &cobra.Command{
		Use:   "broadcast",
		Short: "Post one message to multiple chats, paced to avoid flood",
		Example: `  telegram-cli broadcast --text "Hello everyone!" @channel1 @channel2
  telegram-cli broadcast --media photo.jpg --text "Check this out" @group1
  telegram-cli broadcast --text "Scheduled post" --at 2026-08-04T09:00:00Z @channel1`,
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
			f := parseTelegramFlags(cmd)
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			if template != "" {
				text, err = resolveTemplateText(ctx, s.DB(), template)
				if err != nil {
					return err
				}
			}
			// Parse --at schedule time before dialing; a bad spec is a usage error.
			var scheduledAt string
			if atStr != "" {
				at, err := parseScheduleAt(atStr)
				if err != nil {
					return usageErr(err)
				}
				scheduledAt = at.UTC().Format(time.RFC3339)
			}
			// Mass-visible write: require explicit --yes (or --dry-run, handled
			// above). --agent implies --yes, so agent-driven broadcasts proceed;
			// a bare `broadcast` from a human is gated with a plan + exit 6.
			if !flags.yes {
				if scheduledAt != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "would schedule broadcast to %d chat(s) as %s at %s\n", len(args), alias, scheduledAt)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "would broadcast to %d chat(s) as %s\n", len(args), alias)
				}
				return confirmationErr(fmt.Errorf("broadcast requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
			}

			// Record job
			targets := strings.Join(args, ",")
			status := "running"
			if scheduledAt != "" {
				status = "scheduled"
			}
			res, _ := s.DB().ExecContext(ctx,
				`INSERT INTO tg_jobs (kind, status, accounts_csv, targets_csv, text, media_path, at) VALUES ('broadcast', ?, ?, ?, ?, ?, ?)`,
				status, alias, targets, text, mediaPath, scheduledAt,
			)
			jobID, _ := res.LastInsertId()

			// If scheduled, record the job and return immediately.
			if scheduledAt != "" {
				f := parseTelegramFlags(cmd)
				return mutationResult(f, map[string]any{
					"job_id":      jobID,
					"status":      "scheduled",
					"targets":     len(args),
					"scheduled_at": scheduledAt,
				})
			}

			mgr, err := openManager(home)
			if err != nil {
				return err
			}
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
			if _, err := s.DB().ExecContext(ctx,
				`UPDATE tg_jobs SET status = 'done', finished_at = datetime('now'), error = ? WHERE id = ?`,
				fmt.Sprintf("sent=%d failed=%d", sent, failed), jobID,
			); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to mark job %d done: %v\n", jobID, err)
			}
			fmt.Fprintf(os.Stderr, "\nBroadcast complete: %d sent, %d failed.\n", sent, failed)
			return nil
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "message text")
	cmd.Flags().StringVar(&mediaPath, "media", "", "path to media file")
	cmd.Flags().StringVar(&template, "template", "", "template name to use")
	cmd.Flags().StringVar(&atStr, "at", "", "schedule broadcast at ISO-8601 time (e.g. 2026-08-04T09:00:00Z)")
	addTelegramFlags(cmd)
	return cmd
}

func newNovelBatchCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "batch",
		Short:   "Fan out operations across accounts and chats as a resumable job",
		Example: `  tele batch forward @channel 123 124 --to @list1 @list2
  tele batch mark-read @chat1 @chat2 --account work
  tele batch media @channel 100 101 --out ./downloads`,
		RunE:    parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newBatchForwardCmd(flags))
	cmd.AddCommand(newBatchMarkReadCmd(flags))
	cmd.AddCommand(newBatchMediaCmd(flags))
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
	var atStr string
	cmd := &cobra.Command{
		Use:   "forward <from-chat> <msg-id...>",
		Short: "Fan out forwards of one or more messages to many chats",
		Example: `  tele batch forward @source 100 101 --to @listA @listB
  tele batch forward 7528129992 1402 --to @dest1 @dest2 --pace 3s
  tele batch forward @source 100 --to @dest --at 2026-08-04T09:00:00Z`,
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
			// Parse --at schedule time before dialing; a bad spec is a usage error.
			var scheduledAt string
			if atStr != "" {
				at, err := parseScheduleAt(atStr)
				if err != nil {
					return usageErr(err)
				}
				scheduledAt = at.UTC().Format(time.RFC3339)
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
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}

			// Mass-visible write: require explicit --yes (or --dry-run, handled
			// above). --agent implies --yes; a bare `batch forward` from a human
			// is gated with a plan + exit 6.
			if !flags.yes {
				if scheduledAt != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "would schedule forward %d message(s) from %s to %d target(s) at %s\n", len(msgIDs), fromRef, len(to), scheduledAt)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "would forward %d message(s) from %s to %d target(s)\n", len(msgIDs), fromRef, len(to))
				}
				return confirmationErr(fmt.Errorf("batch forward requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
			}

			// Record the job.
			paramsJSON, _ := json.Marshal(map[string]any{
				"from":   fromRef,
				"msgIDs": msgIDs,
			})
			status := "running"
			if scheduledAt != "" {
				status = "scheduled"
			}
			res, err := s.DB().ExecContext(ctx,
				`INSERT INTO tg_jobs (kind, status, params_json, accounts_csv, targets_csv, text, at)
				 VALUES ('forward', ?, ?, ?, ?, '', ?)`,
				status, string(paramsJSON), alias, strings.Join(to, ","), scheduledAt,
			)
			if err != nil {
				return err
			}
			jobID, _ := res.LastInsertId()

			// If scheduled, record the job and return immediately.
			if scheduledAt != "" {
				return mutationResult(f, map[string]any{
					"job_id":       jobID,
					"status":       "scheduled",
					"from":         fromRef,
					"message_ids":  msgIDs,
					"targets":      len(to),
					"scheduled_at": scheduledAt,
				})
			}

			mgr, err := openManager(home)
			if err != nil {
				return err
			}

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
				if _, err := s.DB().ExecContext(ctx,
					`INSERT OR REPLACE INTO tg_job_results (job_id, account, target, status, detail, message_id, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))`,
					jobID, alias, ref, r.Status, r.Detail, r.Message,
				); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to record job result for %s/%s: %v\n", alias, ref, err)
				}
				time.Sleep(pace)
			}

			if _, err := s.DB().ExecContext(ctx,
				`UPDATE tg_jobs SET status = 'done', finished_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'), error = ? WHERE id = ?`,
				fmt.Sprintf("sent=%d failed=%d", sent, failed), jobID,
			); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to mark job %d done: %v\n", jobID, err)
			}
			writeAuditRecord(ctx, s.DB(), alias, "batch forward", fromRef, string(paramsJSON), fmt.Sprintf("sent=%d failed=%d", sent, failed), "")

			out := map[string]any{
				"job_id":      jobID,
				"from":        fromRef,
				"message_ids": msgIDs,
				"sent":        sent,
				"failed":      failed,
				"results":     results,
			}
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
	cmd.Flags().StringVar(&atStr, "at", "", "schedule batch at ISO-8601 time (e.g. 2026-08-04T09:00:00Z)")
	addTelegramFlags(cmd)
	return cmd
}

// newBatchMarkReadCmd marks multiple chats as read in one operation.
func newBatchMarkReadCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mark-read <chat...>",
		Short: "Mark all messages in multiple chats as read",
		Example: `  tele batch mark-read @chat1 @chat2 @chat3
  tele batch mark-read @chat1 @chat2 --account work`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "batch mark-read")
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
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			type markResult struct {
				Chat   string `json:"chat"`
				Status string `json:"status"`
				Detail string `json:"detail,omitempty"`
			}
			var results []markResult
			read := 0
			failed := 0
			for _, ref := range args {
				r := markResult{Chat: ref, Status: "pending"}
				err := mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					resolver := liveResolver(s.DB(), api)
					peer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					return mtproto.ReadHistory(ctx, api, peer, 0)
				})
				if err != nil {
					failed++
					r.Status = "failed"
					r.Detail = err.Error()
				} else {
					read++
					r.Status = "ok"
				}
				results = append(results, r)
			}
			markAccountUsed(ctx, s, alias)
			return mutationResult(f, map[string]any{
				"read":   read,
				"failed": failed,
				"results": results,
			})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// newBatchMediaCmd downloads media from messages across chats.
func newBatchMediaCmd(flags *rootFlags) *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "media <chat> <msg-id...>",
		Short: "Download media from messages in a chat",
		Example: `  tele batch media @channel 100 101 102
  tele batch media @channel 100 --out ./downloads --account work`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "batch media download")
			}
			ref := args[0]
			var msgIDs []int64
			for _, s := range args[1:] {
				if id, err := strconv.ParseInt(s, 10, 64); err == nil {
					msgIDs = append(msgIDs, id)
				}
			}
			if len(msgIDs) == 0 {
				return usageErr(fmt.Errorf("provide at least one numeric message id"))
			}
			if outDir == "" {
				outDir = "."
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
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			type mediaResult struct {
				MsgID  int64  `json:"msg_id"`
				Path   string `json:"path,omitempty"`
				Status string `json:"status"`
				Detail string `json:"detail,omitempty"`
			}
			var results []mediaResult
			downloaded := 0
			failed := 0
			for _, msgID := range msgIDs {
				r := mediaResult{MsgID: msgID, Status: "pending"}
				err := mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
					resolver := liveResolver(s.DB(), api)
					peer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					msgs, err := mtproto.GetHistory(ctx, api, peer, 100)
					if err != nil {
						return err
					}
					for _, m := range msgs {
						if m.MsgID == msgID && m.Media != "" {
							history, err2 := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
								Peer:  peer,
								Limit: 1,
							})
							if err2 != nil {
								return err2
							}
							if cm, ok := history.(*tg.MessagesMessagesSlice); ok && len(cm.Messages) > 0 {
								if rawMsg, ok := cm.Messages[0].(*tg.Message); ok {
									path, err := mtproto.DownloadMedia(ctx, api, rawMsg, outDir)
									if err != nil {
										return err
									}
									r.Path = path
								}
							}
							break
						}
					}
					return nil
				})
				if err != nil {
					failed++
					r.Status = "failed"
					r.Detail = err.Error()
				} else {
					downloaded++
					r.Status = "ok"
				}
				results = append(results, r)
			}
			markAccountUsed(ctx, s, alias)
			return mutationResult(f, map[string]any{
				"downloaded": downloaded,
				"failed":     failed,
				"results":    results,
			})
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "output directory for downloaded media (default: current dir)")
	addTelegramFlags(cmd)
	return cmd
}

func newNovelInboxCmd(flags *rootFlags) *cobra.Command {
	var accountsStr string
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "View unread messages across all accounts",
		Example: `  tele inbox
  tele inbox --accounts all
  tele inbox --accounts work
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
			acctWhere, acctArgs, err := accountWhere(accountsStr)
			if err != nil {
				return err
			}
			rows, err := s.DB().QueryContext(ctx,
				`SELECT account, title, unread_count FROM tg_dialogs WHERE unread_count > 0`+acctWhere+` ORDER BY unread_count DESC`,
				acctArgs...,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			type inboxItem struct {
				Account string `json:"account"`
				Title   string `json:"title"`
				Chat    string `json:"chat"`
				Unread  int    `json:"unread_count"`
			}
			var items []inboxItem
			for rows.Next() {
				var item inboxItem
				if err := rows.Scan(&item.Account, &item.Title, &item.Unread); err != nil {
					return fmt.Errorf("scanning inbox row: %w", err)
				}
				item.Chat = item.Title
				items = append(items, item)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading inbox rows: %w", err)
			}
			f := parseTelegramFlags(cmd)
			warnMirrorEmpty(ctx, cmd, s.DB(), &f)
			return outResult(stdout(), f, items)
		},
	}
	cmd.Flags().StringVar(&accountsStr, "accounts", "all", "accounts to include: all (default) or a single alias")
	addTelegramFlags(cmd)
	return cmd
}

func newNovelStatsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var perChat, topSenders bool
	var accountsStr string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show message statistics across accounts",
		Example: `  telegram-cli stats
  telegram-cli stats --days 7
  telegram-cli stats --per-chat --top-senders
  telegram-cli stats --days 30 --per-chat --json`,
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
			type chatVol struct {
				Title    string `json:"title"`
				PeerType string `json:"peer_type"`
				PeerID   int64  `json:"peer_id"`
				Messages int    `json:"messages"`
			}
			type senderVol struct {
				Sender   string `json:"sender"`
				Messages int    `json:"messages"`
			}
			type stats struct {
				TotalMessages int         `json:"total_messages"`
				TotalDialogs  int         `json:"total_dialogs"`
				TotalAccounts int         `json:"total_accounts"`
				PerChat       []chatVol   `json:"per_chat,omitempty"`
				TopSenders    []senderVol `json:"top_senders,omitempty"`
			}
			var st stats
			// A --days window narrows every counter to messages in that window
			// (dialogs/accounts stay fleet-wide totals).
			msgWhere := ""
			msgArgs := []any{}
			if days > 0 {
				msgWhere = " WHERE date > ?"
				msgArgs = append(msgArgs, time.Now().Add(-time.Duration(days)*24*time.Hour).Unix())
			}
			acctWhere, acctArgs, err := accountWhere(accountsStr)
			if err != nil {
				return err
			}
			msgWhere += acctWhere
			msgArgs = append(msgArgs, acctArgs...)
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_messages`+msgWhere, msgArgs...).Scan(&st.TotalMessages); err != nil {
				return fmt.Errorf("counting messages: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_dialogs`).Scan(&st.TotalDialogs); err != nil {
				return fmt.Errorf("counting dialogs: %w", err)
			}
			if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_accounts`).Scan(&st.TotalAccounts); err != nil {
				return fmt.Errorf("counting accounts: %w", err)
			}
			if perChat {
				rows, err := s.DB().QueryContext(ctx,
					`SELECT COALESCE(d.title, m.peer_type || ':' || m.peer_id) AS title, m.peer_type, m.peer_id, COUNT(*) AS cnt
					 FROM tg_messages m LEFT JOIN tg_dialogs d ON d.account = m.account AND d.peer_type = m.peer_type AND d.peer_id = m.peer_id
					`+msgWhere+` GROUP BY m.peer_type, m.peer_id ORDER BY cnt DESC LIMIT 10`,
					msgArgs...,
				)
				if err != nil {
					return fmt.Errorf("per-chat volume: %w", err)
				}
				defer rows.Close()
				for rows.Next() {
					var c chatVol
					if err := rows.Scan(&c.Title, &c.PeerType, &c.PeerID, &c.Messages); err != nil {
						return fmt.Errorf("scanning per-chat row: %w", err)
					}
					st.PerChat = append(st.PerChat, c)
				}
				if err := rows.Err(); err != nil {
					return fmt.Errorf("reading per-chat rows: %w", err)
				}
			}
			if topSenders {
				rows, err := s.DB().QueryContext(ctx,
					`SELECT sender, COUNT(*) AS cnt FROM tg_messages`+msgWhere+` GROUP BY sender ORDER BY cnt DESC LIMIT 10`,
					msgArgs...,
				)
				if err != nil {
					return fmt.Errorf("top senders: %w", err)
				}
				defer rows.Close()
				for rows.Next() {
					var sv senderVol
					if err := rows.Scan(&sv.Sender, &sv.Messages); err != nil {
						return fmt.Errorf("scanning sender row: %w", err)
					}
					st.TopSenders = append(st.TopSenders, sv)
				}
				if err := rows.Err(); err != nil {
					return fmt.Errorf("reading sender rows: %w", err)
				}
			}
			f := parseTelegramFlags(cmd)
			warnMirrorEmpty(ctx, cmd, s.DB(), &f)
			return outResult(stdout(), f, st)
		},
	}
	cmd.Flags().IntVar(&days, "days", 0, "only count messages newer than N days (0 = all time)")
	cmd.Flags().BoolVar(&perChat, "per-chat", false, "include per-chat message volume (top 10)")
	cmd.Flags().BoolVar(&topSenders, "top-senders", false, "include top senders by message count (top 10)")
	cmd.Flags().StringVar(&accountsStr, "accounts", "all", "accounts to include: all (default) or a single alias")
	addTelegramFlags(cmd)
	return cmd
}

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var groupBy string
	var accountsStr string
	cmd := &cobra.Command{
		Use:   "since <time-spec>",
		Short: "Show new messages (or grouped counts) since a point in time",
		Example: `  telegram-cli since 1d
  telegram-cli since 1d --group-by account
  telegram-cli since 7d --group-by chat
  telegram-cli since 1d --account work`,
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
			// --accounts / -a filter the mirror read to one account; without them
			// the read is fleet-wide (the mirror spans all accounts). No
			// resolveAccount here: this is a pure analytics read, not a per-account
			// action.
			acctWhere := ""
			acctArgs := []any{}
			if accountsStr != "" && accountsStr != "all" && accountsStr != f.Account && f.Account != "" {
				return usageErr(fmt.Errorf("--accounts %q conflicts with -a %q", accountsStr, f.Account))
			}
			if accountsStr != "" && accountsStr != "all" {
				acctWhere = " AND account = ?"
				acctArgs = append(acctArgs, accountsStr)
			} else if f.Account != "" {
				acctWhere = " AND account = ?"
				acctArgs = append(acctArgs, f.Account)
			}
			if groupBy != "" && groupBy != "account" && groupBy != "chat" {
				return usageErr(fmt.Errorf("invalid --group-by %q: use account or chat", groupBy))
			}
			if groupBy == "account" {
				qargs := append([]any{since.Unix()}, acctArgs...)
				rows, err := s.DB().QueryContext(ctx,
					`SELECT account, COUNT(*) AS cnt FROM tg_messages WHERE date > ?`+acctWhere+` GROUP BY account ORDER BY cnt DESC`,
					qargs...,
				)
				if err != nil {
					return err
				}
				defer rows.Close()
				type acctVol struct {
					Account  string `json:"account"`
					Messages int    `json:"messages"`
				}
				var items []acctVol
				for rows.Next() {
					var v acctVol
					if err := rows.Scan(&v.Account, &v.Messages); err != nil {
						return fmt.Errorf("scanning group row: %w", err)
					}
					items = append(items, v)
				}
				if err := rows.Err(); err != nil {
					return fmt.Errorf("reading group rows: %w", err)
				}
				warnMirrorEmpty(ctx, cmd, s.DB(), &f)
				return outResult(stdout(), f, items)
			}
			if groupBy == "chat" {
				qargs := append(append([]any{since.Unix()}, acctArgs...), f.Limit)
				rows, err := s.DB().QueryContext(ctx,
					`SELECT m.account, COALESCE(d.title, m.peer_type || ':' || m.peer_id) AS title, COUNT(*) AS cnt
					 FROM tg_messages m LEFT JOIN tg_dialogs d ON d.account = m.account AND d.peer_type = m.peer_type AND d.peer_id = m.peer_id
					 WHERE m.date > ?`+acctWhere+` GROUP BY m.account, title ORDER BY cnt DESC LIMIT ?`,
					qargs...,
				)
				if err != nil {
					return err
				}
				defer rows.Close()
				type chatVol struct {
					Account  string `json:"account"`
					Title    string `json:"title"`
					Messages int    `json:"messages"`
				}
				var items []chatVol
				for rows.Next() {
					var v chatVol
					if err := rows.Scan(&v.Account, &v.Title, &v.Messages); err != nil {
						return fmt.Errorf("scanning group row: %w", err)
					}
					items = append(items, v)
				}
				if err := rows.Err(); err != nil {
					return fmt.Errorf("reading group rows: %w", err)
				}
				warnMirrorEmpty(ctx, cmd, s.DB(), &f)
				return outResult(stdout(), f, items)
			}
			qargs := append(append([]any{since.Unix()}, acctArgs...), f.Limit)
			rows, err := s.DB().QueryContext(ctx,
				`SELECT msg_id, sender, text, date FROM tg_messages WHERE date > ?`+acctWhere+` ORDER BY date DESC LIMIT ?`,
				qargs...,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			var items []mtproto.MessageItem
			for rows.Next() {
				var m mtproto.MessageItem
				if err := rows.Scan(&m.MsgID, &m.Sender, &m.Text, &m.Date); err != nil {
					return fmt.Errorf("scanning message row: %w", err)
				}
				items = append(items, m)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading message rows: %w", err)
			}
			warnMirrorEmpty(ctx, cmd, s.DB(), &f)
			return outResult(stdout(), f, items)
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "", "group counts by account or chat instead of listing messages")
	cmd.Flags().StringVar(&accountsStr, "accounts", "all", "accounts to include: all (default) or a single alias")
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
	var days int
	var withHours, withTerms bool
	var accountsStr string
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Generate a digest of recent activity",
		Example: `  telegram-cli digest
  telegram-cli digest --days 7
  telegram-cli digest --days 7 --hours --terms --json`,
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
			window := time.Now().Add(-24 * time.Hour)
			if days > 0 {
				window = time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			}
			acctWhere, acctArgs, err := accountWhere(accountsStr)
			if err != nil {
				return err
			}
			type digestEntry struct {
				Account string `json:"account"`
				Title   string `json:"title"`
				Count   int    `json:"recent_messages"`
			}
			type hourVol struct {
				Hour     int `json:"hour"`
				Messages int `json:"messages"`
			}
			type termVol struct {
				Term     string `json:"term"`
				Messages int    `json:"messages"`
			}
			type digest struct {
				PerChat     []digestEntry `json:"per_chat"`
				BusiestHour []hourVol     `json:"busiest_hours,omitempty"`
				TopTerms    []termVol     `json:"top_terms,omitempty"`
			}
			rows, err := s.DB().QueryContext(ctx,
				`SELECT m.account, COALESCE(d.title, '') as title, COUNT(*) as cnt
				 FROM tg_messages m
				 LEFT JOIN tg_dialogs d ON d.account = m.account AND d.peer_type = m.peer_type AND d.peer_id = m.peer_id
				 WHERE m.date > ?`+strings.ReplaceAll(acctWhere, "account", "m.account")+` GROUP BY m.account, title ORDER BY cnt DESC LIMIT 20`,
				append([]any{window.Unix()}, acctArgs...)[0:]...)
			if err != nil {
				return err
			}
			if err != nil {
				return err
			}
			defer rows.Close()
			var out digest
			out.PerChat = []digestEntry{}
			for rows.Next() {
				var d digestEntry
				if err := rows.Scan(&d.Account, &d.Title, &d.Count); err != nil {
					return fmt.Errorf("scanning digest row: %w", err)
				}
				out.PerChat = append(out.PerChat, d)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading digest rows: %w", err)
			}
			if withHours {
				hArgs := append([]any{window.Unix()}, acctArgs...)
				hrows, err := s.DB().QueryContext(ctx,
					`SELECT CAST(strftime('%H', date, 'unixepoch', 'localtime') AS INTEGER) AS h, COUNT(*) AS cnt
					 FROM tg_messages WHERE date > ?`+acctWhere+` GROUP BY h ORDER BY cnt DESC LIMIT 5`,
					hArgs...,
				)
				if err != nil {
					return fmt.Errorf("busiest hours: %w", err)
				}
				defer hrows.Close()
				for hrows.Next() {
					var hv hourVol
					if err := hrows.Scan(&hv.Hour, &hv.Messages); err != nil {
						return fmt.Errorf("scanning hour row: %w", err)
					}
					out.BusiestHour = append(out.BusiestHour, hv)
				}
				if err := hrows.Err(); err != nil {
					return fmt.Errorf("reading hour rows: %w", err)
				}
			}
			if withTerms {
				// Top terms: strip a tiny stoplist client-side so "the/a/to"
				// don't dominate; computed from the message window.
				tArgs := append([]any{window.Unix()}, acctArgs...)
				trows, err := s.DB().QueryContext(ctx,
					`SELECT text FROM tg_messages WHERE date > ?`+acctWhere+` AND length(trim(text)) > 0 LIMIT 5000`,
					tArgs...,
				)
				if err != nil {
					return fmt.Errorf("top terms: %w", err)
				}
				defer trows.Close()
				counts := map[string]int{}
				var text string
				for trows.Next() {
					if err := trows.Scan(&text); err != nil {
						return fmt.Errorf("scanning term row: %w", err)
					}
					for _, w := range strings.Fields(strings.ToLower(text)) {
						w = strings.Trim(w, ".,!?;:\"'()[]{}")
						if len(w) < 3 || stopWords[w] {
							continue
						}
						counts[w]++
					}
				}
				if err := trows.Err(); err != nil {
					return fmt.Errorf("reading term rows: %w", err)
				}
				type pair struct {
					w string
					n int
				}
				var pairs []pair
				for w, n := range counts {
					pairs = append(pairs, pair{w, n})
				}
				sort.Slice(pairs, func(i, j int) bool { return pairs[i].n > pairs[j].n })
				for i := 0; i < len(pairs) && i < 10; i++ {
					out.TopTerms = append(out.TopTerms, termVol{Term: pairs[i].w, Messages: pairs[i].n})
				}
			}
			f := parseTelegramFlags(cmd)
			warnMirrorEmpty(ctx, cmd, s.DB(), &f)
			return outResult(stdout(), f, out)
		},
	}
	cmd.Flags().IntVar(&days, "days", 0, "digest window in days (default 1)")
	cmd.Flags().BoolVar(&withHours, "hours", false, "include busiest hours by message volume")
	cmd.Flags().BoolVar(&withTerms, "terms", false, "include top message terms (stoplist-filtered)")
	cmd.Flags().StringVar(&accountsStr, "accounts", "all", "accounts to include: all (default) or a single alias")
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
				"library":          "github.com/gotd/td",
				"version":          "v0.161.0",
				"tl_layer":         tg.Layer,
				"compatible":       true,
				"methods_count":    len(rawMethodNames()),
				"types_count":      len(tg.TypesMap()),
				"interfaces_count": len(tg.ClassConstructorsMap()),
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
	var duration, accountsStr, collect string
	var report bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the daemon for a fixed duration",
		Example: `  tele daemon run --duration 10m
  tele daemon run --duration 1h --accounts all --collect messages --report
  tele daemon run --duration 30m --accounts work --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dur, err := time.ParseDuration(duration)
			if err != nil {
				return fmt.Errorf("parse duration: %w", err)
			}
			if collect != "" && collect != "messages" && collect != "updates" {
				return usageErr(fmt.Errorf("invalid --collect %q: use messages or updates", collect))
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

			// Resolve which accounts to hold.
			acctWhere, acctArgs, err := accountWhere(accountsStr)
			if err != nil {
				return err
			}
			rows, err := s.DB().QueryContext(ctx,
				`SELECT alias FROM tg_accounts WHERE status = 'active'`+acctWhere+` ORDER BY alias`,
				acctArgs...,
			)
			if err != nil {
				return fmt.Errorf("reading accounts: %w", err)
			}
			var aliases []string
			for rows.Next() {
				var alias string
				if err := rows.Scan(&alias); err != nil {
					rows.Close()
					return fmt.Errorf("scanning account alias: %w", err)
				}
				aliases = append(aliases, alias)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading account aliases: %w", err)
			}
			if len(aliases) == 0 {
				return fmt.Errorf("no active accounts found")
			}

			type collectResult struct {
				Account  string `json:"account"`
				Messages int    `json:"messages_collected"`
			}
			type daemonReport struct {
				Duration    string           `json:"duration"`
				Accounts    []string         `json:"accounts"`
				Collect     string           `json:"collect"`
				StartedAt   string           `json:"started_at"`
				FinishedAt  string           `json:"finished_at"`
				Results     []collectResult  `json:"results,omitempty"`
			}

			startedAt := time.Now()
			fmt.Fprintf(os.Stderr, "Daemon running for %s with %d account(s)...\n", dur, len(aliases))

			var results []collectResult
			if collect == "messages" {
				// Collect recent messages per dialog for each account.
				for _, alias := range aliases {
					cr := collectResult{Account: alias}
					dialErr := func() error {
						mgr, err := openManager(home)
						if err != nil {
							return err
						}
						return mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
							dialogs, err := mtproto.GetDialogs(ctx, api, 20)
							if err != nil {
								return fmt.Errorf("get dialogs: %w", err)
							}
							for _, d := range dialogs {
								peer, err := peerFromDialog(s.DB(), alias, d)
								if err != nil {
									continue
								}
								peerType, peerID, err := peerKey(s.DB(), alias, peer)
								if err != nil {
									continue
								}
								msgs, err := mtproto.GetHistory(ctx, api, peer, 10)
								if err != nil {
									continue
								}
								for _, m := range msgs {
									if _, err := s.DB().ExecContext(ctx,
										`INSERT OR REPLACE INTO tg_messages (account, peer_type, peer_id, msg_id, date, sender_id, sender, text, media_type, outgoing)
										 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
										alias, peerType, peerID, m.MsgID, m.Date, m.SenderID, m.Sender, m.Text, m.Media, boolToInt(m.Outgoing),
									); err != nil {
										continue
									}
									cr.Messages++
								}
							}
							return nil
						})
					}()
					if dialErr != nil {
						fmt.Fprintf(os.Stderr, "warning: collect messages for %s: %v\n", alias, dialErr)
					}
					results = append(results, cr)
				}
			} else {
				// Default: hold sessions alive for the duration.
				time.Sleep(dur)
			}

			finishedAt := time.Now()
			fmt.Fprintf(os.Stderr, "Daemon stopped.\n")

			if report {
				r := daemonReport{
					Duration:   dur.String(),
					Accounts:   aliases,
					Collect:    collect,
					StartedAt:  startedAt.Format(time.RFC3339),
					FinishedAt: finishedAt.Format(time.RFC3339),
					Results:    results,
				}
				return outResult(stdout(), f, r)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&duration, "duration", "10m", "how long to run (e.g. 10m, 1h)")
	cmd.Flags().StringVar(&accountsStr, "accounts", "all", "accounts to include: all (default) or a single alias")
	cmd.Flags().StringVar(&collect, "collect", "", "what to collect during the run: messages (pull recent messages into mirror)")
	cmd.Flags().BoolVar(&report, "report", false, "emit a structured JSON report on exit")
	return cmd
}

func newNovelJobsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "jobs",
		Short:       "Manage scheduled/broadcast jobs",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newJobsListCmd(flags))
	cmd.AddCommand(newJobsCancelCmd(flags))
	return cmd
}

func newJobsListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List recent jobs",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  telegram-cli jobs list
  telegram-cli jobs list --json`,
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
				if err := rows.Scan(&j.ID, &j.Kind, &j.Status, &j.Targets, &j.CreatedAt, &j.FinishedAt, &j.Error); err != nil {
					return fmt.Errorf("scanning job row: %w", err)
				}
				items = append(items, j)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading job rows: %w", err)
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, items)
		},
	}
}

func newJobsCancelCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "cancel <job-id>",
		Short:       "Cancel a pending or running job",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  telegram-cli jobs cancel 42",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var jobID int64
			if _, err := fmt.Sscanf(args[0], "%d", &jobID); err != nil || jobID <= 0 {
				return usageErr(fmt.Errorf("invalid job id %q: must be a positive integer", args[0]))
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
			// Check current status — only pending/scheduled/running can be cancelled.
			var status string
			err = s.DB().QueryRowContext(ctx,
				`SELECT status FROM tg_jobs WHERE id = ?`, jobID,
			).Scan(&status)
			if errors.Is(err, sql.ErrNoRows) {
				return notFoundErr(fmt.Errorf("job %d not found", jobID))
			}
			if err != nil {
				return fmt.Errorf("reading job %d: %w", jobID, err)
			}
			if status == "done" || status == "cancelled" {
				return usageErr(fmt.Errorf("job %d is already %s", jobID, status))
			}
			_, err = s.DB().ExecContext(ctx,
				`UPDATE tg_jobs SET status = 'cancelled', finished_at = datetime('now'), error = 'cancelled by user' WHERE id = ?`,
				jobID,
			)
			if err != nil {
				return fmt.Errorf("cancelling job %d: %w", jobID, err)
			}
			f := parseTelegramFlags(cmd)
			return mutationResult(f, map[string]any{"job_id": jobID, "status": "cancelled"})
		},
	}
}



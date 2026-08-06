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

// sendCommandState carries the optional send flags shared by newSendCmd.
// Declared at package scope so the cobra closure can read the parsed values.
var (
	replyTo int64
	atStr   string
)

// sendMessageOptions converts the parsed --reply-to/--at flags into the
// mtproto options struct. A parse error on --at is a usage error (exit 2).
func sendMessageOptions(replyTo int64, at *time.Time) mtproto.SendMessageOptions {
	opts := mtproto.SendMessageOptions{}
	if replyTo > 0 {
		opts.ReplyTo = replyTo
	}
	if at != nil {
		opts.ScheduleAt = at.Unix()
	}
	return opts
}

// parseScheduleAt parses an ISO-8601 timestamp (with or without timezone;
// timezone-less values are interpreted as local time).
func parseScheduleAt(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid --at time %q: use ISO-8601 like 2026-08-04T09:00:00Z", s)
}

func newSendCmd(flags *rootFlags) *cobra.Command {
	var mediaPath string
	cmd := &cobra.Command{
		Use:   "send <chat> <message>",
		Short: "Send a text message (or media with --media) to a chat",
		Example: `  telegram-cli send @username "Hello"
  telegram-cli send @channel "Weekly update" --account work
  telegram-cli send @chat "Reply to you" --reply-to 42
  telegram-cli send @chat "Scheduled post" --at 2026-08-04T09:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "send message")
			}
			if len(args) < 2 {
				return cmd.Help()
			}
			ref, text := args[0], args[1]
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
			at, err := parseScheduleAt(atStr)
			if err != nil {
				return usageErr(err)
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var msgID int64
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				if mediaPath != "" {
					msgID, err = mtproto.UploadAndSendMedia(ctx, api, peer, mediaPath, text)
				} else {
					msgID, err = mtproto.SendMessageWithOptions(ctx, api, peer, text, sendMessageOptions(replyTo, at))
				}
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Sent message %d to %s.\n", msgID, ref)
			payload := map[string]any{"msg_id": msgID, "chat": ref}
			if replyTo != 0 {
				payload["reply_to"] = replyTo
			}
			if at != nil {
				payload["scheduled_at"] = at.UTC().Format(time.RFC3339)
			}
			return mutationResult(parseTelegramFlags(cmd), payload)
		},
	}
	cmd.Flags().StringVar(&mediaPath, "media", "", "path to media file to attach")
	cmd.Flags().Int64Var(&replyTo, "reply-to", 0, "message id to reply to (threads the reply)")
	cmd.Flags().StringVar(&atStr, "at", "", "ISO-8601 time to schedule the message (e.g. 2026-08-04T09:00:00Z)")
	addTelegramFlags(cmd)
	return cmd
}

func newForwardCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forward <from-chat> <to-chat> <msg-id...>",
		Short: "Forward messages from one chat to another",
		Example: `  telegram-cli forward @releases @updates 123 124
  telegram-cli forward @releases @updates 123 124 --account work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "forward messages")
			}
			if len(args) < 3 {
				return cmd.Help()
			}
			fromRef, toRef := args[0], args[1]
			var msgIDs []int64
			for _, a := range args[2:] {
				var id int64
				if _, err := fmt.Sscanf(a, "%d", &id); err != nil || id <= 0 {
					return usageErr(fmt.Errorf("invalid message id %q: must be a positive integer", a))
				}
				msgIDs = append(msgIDs, id)
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				fromPeer, err := resolver.Resolve(ctx, alias, fromRef)
				if err != nil {
					return err
				}
				toPeer, err := resolver.Resolve(ctx, alias, toRef)
				if err != nil {
					return err
				}
				_, err = mtproto.ForwardMessages(ctx, api, fromPeer, toPeer, msgIDs)
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Forwarded %d messages.\n", len(msgIDs))
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"forwarded": len(msgIDs), "from": fromRef, "to": toRef})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newDeleteCmd(flags *rootFlags) *cobra.Command {
	var revoke bool
	cmd := &cobra.Command{
		Use:   "delete <chat> <msg-id...>",
		Short: "Delete messages from a chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "delete messages")
			}
			if len(args) < 2 {
				return cmd.Help()
			}
			var msgIDs []int64
			for _, a := range args[1:] {
				var id int64
				if _, err := fmt.Sscanf(a, "%d", &id); err != nil || id <= 0 {
					return usageErr(fmt.Errorf("invalid message id %q: must be a positive integer", a))
				}
				msgIDs = append(msgIDs, id)
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
			// Irreversible write: require explicit --yes (or --dry-run, handled
			// above). --agent implies --yes, so agent-driven deletes proceed; a
			// bare `delete` from a human is gated with a plan + exit 6.
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would delete %d message(s) from %s (revoke=%v)\n", len(msgIDs), args[0], revoke)
				return confirmationErr(fmt.Errorf("delete requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
			}

			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				return mtproto.DeleteMessages(ctx, api, msgIDs, revoke)
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Deleted %d messages.\n", len(msgIDs))
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"deleted": len(msgIDs), "revoke": revoke})
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "delete for all participants")
	addTelegramFlags(cmd)
	return cmd
}

func newReadCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read <chat>",
		Short: "Mark all messages in a chat as read",
		Example: `  telegram-cli read @username
  telegram-cli read me
  telegram-cli read @username --account work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mark as read")
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			ref := args[0]
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				return mtproto.ReadHistory(ctx, api, peer, 0)
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Marked %s as read.\n", ref)
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"chat": ref, "read": true})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newReactCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "react <chat> <msg-id> <emoji>",
		Short: "Send an emoji reaction to a message",
		Example: `  telegram-cli react @chat 123 👍
  telegram-cli react @chat 123 👍 --account work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "send reaction")
			}
			if len(args) < 3 {
				return cmd.Help()
			}
			ref, emoji := args[0], args[2]
			var msgID int64
			if _, err := fmt.Sscanf(args[1], "%d", &msgID); err != nil || msgID <= 0 {
				return usageErr(fmt.Errorf("invalid message id %q: must be a positive integer", args[1]))
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				return mtproto.SendReaction(ctx, api, peer, msgID, emoji)
			})
			if err != nil {
				return err
			}
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"chat": ref, "msg_id": msgID, "emoji": emoji})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newEditCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <chat> <msg-id> <new-text>",
		Short: "Edit a sent message",
		Example: `  telegram-cli edit @chat 123 "new text"
  telegram-cli edit @chat 123 "new text" --account work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "edit message")
			}
			if len(args) < 3 {
				return cmd.Help()
			}
			ref, text := args[0], args[2]
			var msgID int64
			if _, err := fmt.Sscanf(args[1], "%d", &msgID); err != nil || msgID <= 0 {
				return usageErr(fmt.Errorf("invalid message id %q: must be a positive integer", args[1]))
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				return mtproto.EditMessage(ctx, api, peer, msgID, text)
			})
			if err != nil {
				return err
			}
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"chat": ref, "msg_id": msgID})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newMediaCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "media <chat> <msg-id> [out-dir]",
		Short: "Download media from a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "download media")
			}
			if len(args) < 2 {
				return cmd.Help()
			}
			ref := args[0]
			var msgID int64
			if _, err := fmt.Sscanf(args[1], "%d", &msgID); err != nil || msgID <= 0 {
				return usageErr(fmt.Errorf("invalid message id %q: must be a positive integer", args[1]))
			}
			outDir := "."
			if len(args) > 2 {
				outDir = args[2]
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
			var savedPath string
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
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
						// Get the raw message for download
						history, err2 := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
							Peer:  peer,
							Limit: 1,
						})
						if err2 != nil {
							return err2
						}
						if cm, ok := history.(*tg.MessagesMessagesSlice); ok && len(cm.Messages) > 0 {
							if rawMsg, ok := cm.Messages[0].(*tg.Message); ok {
								savedPath, err = mtproto.DownloadMedia(ctx, api, rawMsg, outDir)
							}
						}
						break
					}
				}
				return err
			})
			if err != nil {
				return err
			}
			if savedPath != "" {
				fmt.Fprintf(os.Stderr, "Saved to %s\n", savedPath)
			} else {
				fmt.Fprintf(os.Stderr, "No media found in message %d\n", msgID)
			}
			return mutationResult(parseTelegramFlags(cmd), map[string]any{"chat": ref, "msg_id": msgID, "path": savedPath, "downloaded": savedPath != ""})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// Ensure imports are used
var _ = strings.TrimSpace

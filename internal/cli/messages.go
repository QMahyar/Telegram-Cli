package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newSendCmd(flags *rootFlags) *cobra.Command {
	var mediaPath string
	cmd := &cobra.Command{
		Use:   "send <chat> <message>",
		Short: "Send a text message (or media with --media) to a chat",
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
			alias, err := resolveAccount(ctx, s, "")
			if err != nil {
				return err
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
					msgID, err = mtproto.SendMessage(ctx, api, peer, text)
				}
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Sent message %d to %s.\n", msgID, ref)
			return nil
		},
	}
	cmd.Flags().StringVar(&mediaPath, "media", "", "path to media file to attach")
	return cmd
}

func newForwardCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forward <from-chat> <to-chat> <msg-id...>",
		Short: "Forward messages from one chat to another",
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
				fmt.Sscanf(a, "%d", &id)
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
			alias, err := resolveAccount(ctx, s, "")
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
			return nil
		},
	}
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
				fmt.Sscanf(a, "%d", &id)
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
			alias, err := resolveAccount(ctx, s, "")
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
			return nil
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "delete for all participants")
	return cmd
}

func newReadCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "read <chat>",
		Short: "Mark all messages in a chat as read",
		Example: `  telegram-cli read @username
  telegram-cli read me`,
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
			alias, err := resolveAccount(ctx, s, "")
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
			return nil
		},
	}
}

func newReactCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "react <chat> <msg-id> <emoji>",
		Short: "Send an emoji reaction to a message",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "send reaction")
			}
			if len(args) < 3 {
				return cmd.Help()
			}
			ref, emoji := args[0], args[2]
			var msgID int64
			fmt.Sscanf(args[1], "%d", &msgID)
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				return mtproto.SendReaction(ctx, api, peer, msgID, emoji)
			})
			return err
		},
	}
}

func newEditCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <chat> <msg-id> <new-text>",
		Short: "Edit a sent message",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "edit message")
			}
			if len(args) < 3 {
				return cmd.Help()
			}
			ref, text := args[0], args[2]
			var msgID int64
			fmt.Sscanf(args[1], "%d", &msgID)
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
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				return mtproto.EditMessage(ctx, api, peer, msgID, text)
			})
			return err
		},
	}
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
			fmt.Sscanf(args[1], "%d", &msgID)
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
			alias, err := resolveAccount(ctx, s, "")
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
			return nil
		},
	}
	return cmd
}

// Ensure imports are used
var _ = strings.TrimSpace

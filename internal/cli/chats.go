package cli

import (
	"context"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newChatsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "chats",
		Short:       "List and inspect Telegram chats (dialogs)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list chats")
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
			f := parseTelegramFlags(cmd)
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var dialogs []mtproto.DialogItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				dialogs, err = mtproto.GetDialogs(ctx, api, f.Limit)
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			return outResult(stdout(), f, dialogs)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newMessagesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "messages <chat>",
		Short:       "Show message history for a chat",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "read messages")
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
			f := parseTelegramFlags(cmd)
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var messages []mtproto.MessageItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := mtproto.NewPeerResolver(s.DB())
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				messages, err = mtproto.GetHistory(ctx, api, peer, f.Limit)
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			return outResult(stdout(), f, messages)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

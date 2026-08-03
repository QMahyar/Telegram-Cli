package cli

import (
	"context"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newSearchCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "search <query>",
		Short:       "Search messages across all chats",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
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
				messages, err = mtproto.SearchMessages(ctx, api, query, f.Limit)
				return err
			})
			if err != nil {
				return err
			}
			return outResult(stdout(), f, messages)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

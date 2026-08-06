package cli

import (
	"context"
	"fmt"
	"os"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

func newBlockCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block <user...>",
		Short: "Block users",
		Example: `  telegram-cli block @spammer
  telegram-cli block @spammer1 @spammer2 --account work`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "block users")
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
				for _, ref := range args {
					peer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					if err := mtproto.BlockUser(ctx, api, peer); err != nil {
						return fmt.Errorf("%s: %w", ref, err)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Blocked %d user(s).\n", len(args))
			return mutationResult(f, map[string]any{"blocked": len(args), "users": args})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newUnblockCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unblock <user...>",
		Short: "Unblock users",
		Example: `  telegram-cli unblock @oldspammer
  telegram-cli unblock @oldspammer --account work`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "unblock users")
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
				for _, ref := range args {
					peer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					if err := mtproto.UnblockUser(ctx, api, peer); err != nil {
						return fmt.Errorf("%s: %w", ref, err)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Unblocked %d user(s).\n", len(args))
			return mutationResult(f, map[string]any{"unblocked": len(args), "users": args})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// blockedItem renders one blocked user for `blocked list`.
type blockedItem struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

func newBlockedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "blocked",
		Short:       "List blocked users",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
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
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var items []blockedItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				users, err := mtproto.GetBlocked(ctx, api, f.Limit)
				if err != nil {
					return err
				}
				for _, u := range users {
					items = append(items, blockedItem{ID: u.ID, Username: u.Username, FirstName: u.FirstName, LastName: u.LastName})
				}
				return nil
			})
			if err != nil {
				return err
			}
			if items == nil {
				items = []blockedItem{}
			}
			return outResult(stdout(), f, items)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

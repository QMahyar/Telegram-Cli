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

func newChatsCmd(flags *rootFlags) *cobra.Command {
	var searchQ, chatType string
	var unreadOnly, pinnedOnly, blockedOnly bool
	cmd := &cobra.Command{
		Use:         "chats",
		Short:       "List and inspect Telegram chats (dialogs)",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Example: `  telegram-cli chats
  telegram-cli chats --unread
  telegram-cli chats --pinned
  telegram-cli chats --type channel
  telegram-cli chats --search "release"
  telegram-cli chats --blocked`,
		Args: cobra.NoArgs,
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
			if chatType != "" && chatType != "user" && chatType != "chat" && chatType != "channel" && chatType != "bot" && chatType != "group" {
				return usageErr(fmt.Errorf("invalid --type %q: use user, chat, group, channel, or bot", chatType))
			}
			if blockedOnly && (searchQ != "" || chatType != "" || unreadOnly || pinnedOnly) {
				return usageErr(fmt.Errorf("--blocked cannot be combined with --search/--type/--unread/--pinned"))
			}
			f := parseTelegramFlags(cmd)
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var dialogs []mtproto.DialogItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				if blockedOnly {
					users, err := mtproto.GetBlocked(ctx, api, f.Limit)
					if err != nil {
						return err
					}
					for _, u := range users {
						dialogs = append(dialogs, mtproto.DialogItem{
							PeerType: "user",
							PeerID:   u.ID,
							Title:    strings.TrimSpace(u.FirstName + " " + u.LastName),
							Username: u.Username,
						})
					}
					return nil
				}
				if searchQ != "" {
					dialogs, err = mtproto.SearchContacts(ctx, api, searchQ, f.Limit)
					return err
				}
				dialogs, err = mtproto.GetDialogs(ctx, api, f.Limit)
				return err
			})
			if err != nil {
				return err
			}
			// Client-side filters over the fetched dialog set.
			filtered := dialogs[:0]
			for _, d := range dialogs {
				if unreadOnly && d.Unread == 0 {
					continue
				}
				if pinnedOnly && !d.Pinned {
					continue
				}
				if chatType != "" {
					matches := d.PeerType == chatType
					if chatType == "group" {
						matches = d.PeerType == "chat" || d.PeerType == "channel"
					}
					if chatType == "bot" {
						// bots are users; flag by unknown username shape is not
						// reliable here, so bot == user until a user-info pass.
						matches = d.PeerType == "user"
					}
					if !matches {
						continue
					}
				}
				filtered = append(filtered, d)
			}
			dialogs = filtered
			markAccountUsed(ctx, s, alias)
			return outResult(stdout(), f, dialogs)
		},
	}
	cmd.Flags().StringVar(&searchQ, "search", "", "search chats/contacts by name (live contacts.search)")
	cmd.Flags().StringVar(&chatType, "type", "", "filter by peer type: user, chat, group, channel, or bot")
	cmd.Flags().BoolVar(&unreadOnly, "unread", false, "only chats with unread messages")
	cmd.Flags().BoolVar(&pinnedOnly, "pinned", false, "only pinned chats")
	cmd.Flags().BoolVar(&blockedOnly, "blocked", false, "list blocked users instead of dialogs (contacts.getBlocked)")
	addTelegramFlags(cmd)
	cmd.AddCommand(newChatsLeaveCmd(flags))
	cmd.AddCommand(newChatsDeleteCmd(flags))
	return cmd
}

// newChatsLeaveCmd leaves a group or channel (messages.deleteChatUser / channels.leaveChannel).
func newChatsLeaveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "leave <chat>",
		Aliases: []string{"exit"},
		Short:   "Leave a group or channel",
		Example: `  telegram-cli chats leave @oldgroup
  telegram-cli chats leave @oldgroup --account work`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "leave chat")
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
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would leave %s\n", ref)
				return confirmationErr(fmt.Errorf("chats leave requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
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
				return mtproto.LeaveChat(ctx, api, peer)
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Left %s.\n", ref)
			return mutationResult(f, map[string]any{"left": ref})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// newChatsDeleteCmd deletes a group or channel (messages.deleteChat / channels.deleteChannel).
func newChatsDeleteCmd(flags *rootFlags) *cobra.Command {
	var revoke bool
	cmd := &cobra.Command{
		Use:     "delete <chat>",
		Aliases: []string{"rm", "destroy"},
		Short:   "Delete a group or channel you own (irreversible)",
		Example: `  telegram-cli chats delete @myoldchannel --yes
  telegram-cli chats delete @myoldgroup --revoke --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "delete chat")
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
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would delete %s (revoke=%v) — this is irreversible\n", ref, revoke)
				return confirmationErr(fmt.Errorf("chats delete requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
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
				return mtproto.DeleteChat(ctx, api, peer, revoke)
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Deleted %s.\n", ref)
			return mutationResult(f, map[string]any{"deleted": ref, "revoke": revoke})
		},
	}
	cmd.Flags().BoolVar(&revoke, "revoke", false, "delete history for all participants (groups)")
	addTelegramFlags(cmd)
	return cmd
}

func newMessagesCmd(flags *rootFlags) *cobra.Command {
	var offsetID, minID, maxID int
	var direction, fromRef, sinceStr, untilStr string
	var local bool
	cmd := &cobra.Command{
		Use:         "messages <chat>",
		Short:       "Show message history for a chat (with paging, date range, and from filters)",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Example: `  telegram-cli messages @chat
  telegram-cli messages @chat --offset 900 --limit 50
  telegram-cli messages @chat --direction oldest --limit 10
  telegram-cli messages @chat --since 1d
  telegram-cli messages @chat --from @user --until 2026-08-01
  telegram-cli messages @chat --local            (query the synced mirror, offline)
  telegram-cli messages @chat --data-source local`,
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
			f := parseTelegramFlags(cmd)
			alias, err := resolveAccount(ctx, s, f.Account)
			if err != nil {
				return err
			}
			// --data-source local / --local: query the mirror offline.
			if local || strings.EqualFold(flags.dataSource, "local") {
				peer, err := offlineResolve(s.DB(), alias, ref)
				if err != nil {
					return notFoundErr(fmt.Errorf("resolve chat %q from mirror: %w (run `telegram-cli sync` to cache peers)", ref, err))
				}
				pt, pid, err := peerKey(s.DB(), alias, peer)
				if err != nil {
					return err
				}
				limit := f.Limit
				if limit <= 0 {
					limit = 30
				}
				msgs, err := mirrorMessages(ctx, s.DB(), alias, pt, fmt.Sprintf("%d", pid), "", time.Time{}, time.Time{}, "", int64(offsetID), limit)
				if err != nil {
					return err
				}
				warnMirrorEmpty(ctx, cmd, s.DB(), &f)
				return outResult(stdout(), f, msgs)
			}
			if direction != "" && direction != "newest" && direction != "oldest" {
				return usageErr(fmt.Errorf("invalid --direction %q: use newest or oldest", direction))
			}
			if offsetID != 0 && direction == "oldest" {
				return usageErr(fmt.Errorf("--offset and --direction oldest are mutually exclusive (oldest starts at the beginning of history)"))
			}
			// Date/from filters route through peer-scoped search (getHistory has
			// no min_date/max_date/from_id).
			sinceT, err := parseSinceSpec(sinceStr)
			if err != nil {
				return usageErr(err)
			}
			untilT, err := parseSinceSpec(untilStr)
			if err != nil {
				return usageErr(err)
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var messages []mtproto.MessageItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				if sinceStr != "" || untilStr != "" || fromRef != "" {
					opts := mtproto.SearchOptions{Limit: f.Limit, Peer: peer}
					if sinceStr != "" {
						opts.MinDate = sinceT.Unix()
					}
					if untilStr != "" {
						opts.MaxDate = untilT.Unix()
					}
					if fromRef != "" {
						fromPeer, err := resolver.Resolve(ctx, alias, fromRef)
						if err != nil {
							return err
						}
						opts.FromID = fromPeer
					}
					messages, err = mtproto.SearchMessagesWithOptions(ctx, api, "", opts)
					return err
				}
				messages, err = mtproto.GetHistoryWithOptions(ctx, api, peer, mtproto.HistoryOptions{
					Limit:     f.Limit,
					OffsetID:  offsetID,
					MaxID:     maxID,
					MinID:     minID,
					Direction: direction,
				})
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			return outResult(stdout(), f, messages)
		},
	}
	cmd.Flags().IntVar(&offsetID, "offset", 0, "page to messages older than this message id (newest-first walk)")
	cmd.Flags().IntVar(&minID, "min-id", 0, "only messages with ids greater than this")
	cmd.Flags().IntVar(&maxID, "max-id", 0, "only messages with ids less than this")
	cmd.Flags().StringVar(&direction, "direction", "", "newest (default) or oldest (start at the beginning of history)")
	cmd.Flags().StringVar(&fromRef, "from", "", "only messages sent by this user")
	cmd.Flags().StringVar(&sinceStr, "since", "", "only messages newer than this (RFC3339, 2026-01-01, or 1d/12h/30m)")
	cmd.Flags().StringVar(&untilStr, "until", "", "only messages older than this (RFC3339, 2026-01-01, or 1d/12h/30m)")
	cmd.Flags().BoolVar(&local, "local", false, "query the synced mirror instead of Telegram servers")
	addTelegramFlags(cmd)
	return cmd
}

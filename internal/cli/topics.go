package cli

import (
	"context"

	"telegram-cli/internal/config"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

type topicItem struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Icon   string `json:"icon_emoji,omitempty"`
	Closed bool   `json:"closed,omitempty"`
}

func newTopicsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "topics <group>",
		Short:       "List topics in a forum group",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list forum topics")
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
			var topics []topicItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, args[0])
				if err != nil {
					return err
				}
				resp, err := api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{
					Peer:       peer,
					OffsetDate: 0,
					OffsetID:   0,
					Limit:      f.Limit,
					Q:          "",
				})
				if err != nil {
					return err
				}
				for _, t := range resp.Topics {
					switch tp := t.(type) {
					case *tg.ForumTopic:
						topics = append(topics, topicItem{ID: int(tp.ID), Title: tp.Title, Icon: iconEmoji(tp.IconEmojiID)})
					case *tg.ForumTopicDeleted:
						topics = append(topics, topicItem{ID: int(tp.ID), Title: "(deleted)"})
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			if topics == nil {
				topics = []topicItem{}
			}
			return outResult(stdout(), f, topics)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func iconEmoji(id int64) string {
	if id > 0 {
		// Icon custom emoji document ID; the CLI cannot resolve it offline.
		return "custom"
	}
	return ""
}

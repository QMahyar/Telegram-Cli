package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"telegram-cli/internal/config"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

type watchEvent struct {
	Account string `json:"account"`
	Chat    string `json:"chat,omitempty"`
	From    string `json:"from,omitempty"`
	MsgID   int    `json:"msg_id"`
	Text    string `json:"text,omitempty"`
	Media   string `json:"media_type,omitempty"`
	Date    int64  `json:"date"`
}

// newWatchCmd runs a bounded real-time update stream. Incoming messages are
// printed as they arrive (human mode) or collected into a JSON list (--json),
// then the stream exits after --duration.
func newWatchCmd(flags *rootFlags) *cobra.Command {
	var duration time.Duration
	cmd := &cobra.Command{
		Use:         "watch",
		Short:       "Monitor new messages in real-time (streams updates)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  tele watch --duration 2m\n  tele watch --json --duration 10s",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "watch for new messages")
			}
			if duration <= 0 {
				duration = 30 * time.Second
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

			// Effective JSON mode: --agent flags.json || --json; the runner
			// skips setting asJSON when --json was passed explicitly.
			jsonOut := flags.asJSON || f.JSON

			// Bound the stream: after --duration elapses the context cancels,
			// client.Run tears down the session, and the command exits.
			watchCtx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()

			var (
				mu     sync.Mutex
				events []watchEvent
			)
			handler := telegram.UpdateHandlerFunc(func(uCtx context.Context, u tg.UpdatesClass) error {
				var fresh []watchEvent
				collectUpdates(alias, u, &fresh)
				if len(fresh) == 0 {
					return nil
				}
				mu.Lock()
				events = append(events, fresh...)
				mu.Unlock()
				if !jsonOut {
					for _, ev := range fresh {
						sender := ev.From
						if sender == "" {
							sender = ev.Chat
						}
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s%s\n",
							ev.Chat, sender, ev.Text, mediaSuffix(ev.Media))
					}
				}
				return nil
			})

			err = mgr.DialAndRunWithUpdates(watchCtx, alias, handler, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				// Hold the stream open until --duration elapses.
				<-ctx.Done()
				return nil
			})
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			if events == nil {
				events = []watchEvent{}
			}
			if jsonOut {
				return outResult(stdout(), f, events)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Watched %s for %s; observed %d message(s).\n", alias, duration.Round(time.Second), len(events))
			return nil
		},
	}
	cmd.Flags().DurationVar(&duration, "duration", 0, "how long to watch (e.g. 30s, 2m; default 30s)")
	addTelegramFlags(cmd)
	return cmd
}

func mediaSuffix(m string) string {
	if m == "" {
		return ""
	}
	return " [" + m + "]"
}

// collectUpdates walks a tg.UpdatesClass and extracts new-message events.
func collectUpdates(alias string, u tg.UpdatesClass, events *[]watchEvent) {
	var updates []tg.UpdateClass
	var chats []tg.ChatClass
	var users []tg.UserClass
	switch t := u.(type) {
	case *tg.Updates:
		updates, chats, users = t.Updates, t.Chats, t.Users
	case *tg.UpdatesCombined:
		updates, chats, users = t.Updates, t.Chats, t.Users
	case *tg.UpdateShort:
		if nu, ok := t.Update.(*tg.UpdateNewMessage); ok {
			addMessageEvent(alias, nu.Message, make(map[int64]string), make(map[int64]string), events)
		}
		return
	case *tg.UpdateShortMessage:
		*events = append(*events, watchEvent{
			Account: alias, MsgID: t.ID, Text: t.Message, Date: int64(t.Date),
			From: fmt.Sprintf("@user_%d", t.UserID), Chat: fmt.Sprintf("@user_%d", t.UserID),
		})
		return
	case *tg.UpdateShortChatMessage:
		*events = append(*events, watchEvent{
			Account: alias, MsgID: t.ID, Text: t.Message, Date: int64(t.Date),
			Chat: fmt.Sprintf("chat_%d", t.ChatID),
		})
		return
	default:
		return
	}
	chatTitles := map[int64]string{}
	for _, c := range chats {
		switch ch := c.(type) {
		case *tg.Chat:
			chatTitles[-ch.ID] = ch.Title
		case *tg.Channel:
			chatTitles[-ch.ID] = ch.Title
		}
	}
	userNames := map[int64]string{}
	for _, cu := range users {
		if us, ok := cu.(*tg.User); ok {
			name := us.Username
			if name == "" {
				name = fmt.Sprintf("%s %s", us.FirstName, us.LastName)
			}
			userNames[us.ID] = name
		}
	}
	for _, upd := range updates {
		switch m := upd.(type) {
		case *tg.UpdateNewMessage:
			addMessageEvent(alias, m.Message, chatTitles, userNames, events)
		case *tg.UpdateNewChannelMessage:
			addMessageEvent(alias, m.Message, chatTitles, userNames, events)
		}
	}
}

func addMessageEvent(alias string, mc tg.MessageClass, chatTitles, userNames map[int64]string, events *[]watchEvent) {
	msg, ok := mc.(*tg.Message)
	if !ok {
		return
	}
	chat := ""
	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		chat = userNames[p.UserID]
		if chat == "" {
			chat = fmt.Sprintf("@user_%d", p.UserID)
		}
	case *tg.PeerChat:
		chat = chatTitles[p.ChatID]
		if chat == "" {
			chat = fmt.Sprintf("chat_%d", p.ChatID)
		}
	case *tg.PeerChannel:
		chat = chatTitles[p.ChannelID]
		if chat == "" {
			chat = fmt.Sprintf("channel_%d", p.ChannelID)
		}
	}
	from := ""
	if id, ok := peerClassID(msg.FromID); ok {
		from = userNames[id]
	}
	*events = append(*events, watchEvent{
		Account: alias,
		Chat:    chat,
		From:    from,
		MsgID:   msg.ID,
		Text:    msg.Message,
		Media:   watchMediaType(msg),
		Date:    int64(msg.Date),
	})
}

// peerClassID extracts the bare peer identifier from a tg.PeerClass wrapper.
func peerClassID(p tg.PeerClass) (int64, bool) {
	switch t := p.(type) {
	case *tg.PeerUser:
		return t.UserID, true
	case *tg.PeerChat:
		return t.ChatID, true
	case *tg.PeerChannel:
		return t.ChannelID, true
	default:
		return 0, false
	}
}

func watchMediaType(m *tg.Message) string {
	switch m.Media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		if _, ok := m.Media.(*tg.MessageMediaDocument); ok {
			return "document"
		}
	}
	return ""
}

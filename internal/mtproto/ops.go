package mtproto

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/gotd/td/tg"
)

// DialogItem is a simplified dialog record for output.
type DialogItem struct {
	PeerType  string `json:"peer_type"`
	PeerID    int64  `json:"peer_id"`
	Title     string `json:"title"`
	Username  string `json:"username"`
	Unread    int    `json:"unread_count"`
	LastMsgID int64  `json:"last_msg_id"`
	Pinned    bool   `json:"pinned"`
}

// MessageItem is a simplified message record for output.
type MessageItem struct {
	MsgID    int64  `json:"msg_id"`
	Date     int64  `json:"date"`
	Sender   string `json:"sender"`
	SenderID int64  `json:"sender_id"`
	Text     string `json:"text"`
	Media    string `json:"media_type,omitempty"`
	Outgoing bool   `json:"outgoing"`
}

// GetDialogs fetches the recent dialog list.
func GetDialogs(ctx context.Context, api *tg.Client, limit int) ([]DialogItem, error) {
	if limit <= 0 {
		limit = 50
	}
	resp, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetDate: 0,
		OffsetID:   0,
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("get dialogs: %w", err)
	}
	return extractDialogs(resp)
}

// GetHistory fetches message history for a peer.
func GetHistory(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, limit int) ([]MessageItem, error) {
	if limit <= 0 {
		limit = 30
	}
	resp, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return extractMessages(resp)
}

// SendMessage sends a plain text message to a peer.
func SendMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, text string) (int64, error) {
	rnd := rand.Int63()
	resp, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: rnd,
	})
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}
	return extractMessageID(resp)
}

// ForwardMessages forwards messages from one peer to another.
func ForwardMessages(ctx context.Context, api *tg.Client, fromPeer, toPeer tg.InputPeerClass, msgIDs []int64) ([]int64, error) {
	rnds := make([]int64, len(msgIDs))
	for i := range rnds {
		rnds[i] = rand.Int63()
	}
	resp, err := api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer,
		ID:      intSliceFromInt64(msgIDs),
		ToPeer:   toPeer,
		RandomID: rnds,
	})
	if err != nil {
		return nil, fmt.Errorf("forward messages: %w", err)
	}
	return extractUpdatesMessageIDs(resp)
}

// DeleteMessages deletes messages (optionally from both sides).
func DeleteMessages(ctx context.Context, api *tg.Client, msgIDs []int64, revoke bool) error {
	_, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		ID:      intSliceFromInt64(msgIDs),
		Revoke:  revoke,
	})
	return err
}

// ReadHistory marks messages as read up to the given maxID.
func ReadHistory(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, maxID int64) error {
	_, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
		Peer:  peer,
		MaxID: int(maxID),
	})
	return err
}

// SendReaction sends an emoji reaction to a message.
func SendReaction(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, msgID int64, emoji string) error {
	_, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     peer,
		MsgID:    int(msgID),
		Reaction: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}},
	})
	return err
}

// EditMessage edits a text message.
func EditMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, msgID int64, text string) error {
	_, err := api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:    peer,
		ID:      int(msgID),
		Message: text,
	})
	return err
}

// SearchMessages searches for messages across all chats.
func SearchMessages(ctx context.Context, api *tg.Client, query string, limit int) ([]MessageItem, error) {
	if limit <= 0 {
		limit = 30
	}
	resp, err := api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
		Q:     query,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return extractMessages(resp)
}

// GetParticipants fetches the member list of a channel/supergroup.
func GetParticipants(ctx context.Context, api *tg.Client, channel tg.InputChannelClass, query string, limit int) ([]*tg.ChatParticipant, error) {
	if limit <= 0 {
		limit = 50
	}
	resp, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: channel,
		Filter:  &tg.ChannelParticipantsSearch{Q: query},
		Offset:  0,
		Limit:   limit,
		Hash:    0,
	})
	if err != nil {
		return nil, fmt.Errorf("get participants: %w", err)
	}
	switch r := resp.(type) {
	case *tg.ChannelsChannelParticipants:
		var participants []*tg.ChatParticipant
		for _, p := range r.Participants {
			if cp, ok := p.(*tg.ChannelParticipant); ok {
				participants = append(participants, &tg.ChatParticipant{UserID: cp.UserID})
			}
		}
		return participants, nil
	default:
		return nil, fmt.Errorf("unexpected response type: %T", resp)
	}
}

// InvokeRaw sends a raw TL method via JSON.
// payload is a JSON object like {"method":"help.getNearestDC","params":{}}.
func InvokeRaw(ctx context.Context, client RawInvoker, payload string) (string, error) {
	resp, err := client.InvokeRawJSON(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("raw invoke: %w", err)
	}
	return resp, nil
}

// RawInvoker is satisfied by *telegram.Client.
type RawInvoker interface {
	InvokeRawJSON(ctx context.Context, jsonRequest string) (jsonResponse string, err error)
}

// --- Helper extractors ---

func extractDialogs(resp tg.MessagesDialogsClass) ([]DialogItem, error) {
	var items []DialogItem
	var users []*tg.User
	var chats []*tg.Chat
	var dialogs []tg.DialogClass

	switch r := resp.(type) {
	case *tg.MessagesDialogsSlice:
		dialogs = r.Dialogs
		users = usersFromClasses(r.Users)
		chats = chatsFromClasses(r.Chats)
	case *tg.MessagesDialogs:
		dialogs = r.Dialogs
		users = usersFromClasses(r.Users)
		chats = chatsFromClasses(r.Chats)
	default:
		return nil, fmt.Errorf("unsupported dialog response: %T", resp)
	}
	for _, d := range dialogs {
		dialog, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		peer := resolvePeerFromDialog(dialog, users, chats)
		items = append(items, DialogItem{
			PeerType:  peer.peerType,
			PeerID:    peer.peerID,
			Title:     peer.title,
			Username:  peer.username,
			Unread:    dialog.UnreadCount,
			LastMsgID: int64(dialog.TopMessage),
			Pinned:    dialog.Pinned,
		})
	}
	return items, nil
}

func extractMessages(resp tg.MessagesMessagesClass) ([]MessageItem, error) {
	var items []MessageItem
	var users map[int64]*tg.User

	switch r := resp.(type) {
	case *tg.MessagesMessagesSlice:
		users = usersByID(r.Users)
	case *tg.MessagesMessages:
		users = usersByID(r.Users)
	case *tg.MessagesChannelMessages:
		users = usersByID(r.Users)
	default:
		return nil, fmt.Errorf("unsupported messages response: %T", resp)
	}

	var messages []tg.MessageClass
	switch r := resp.(type) {
	case *tg.MessagesMessagesSlice:
		messages = r.Messages
	case *tg.MessagesMessages:
		messages = r.Messages
	case *tg.MessagesChannelMessages:
		messages = r.Messages
	}

	for _, m := range messages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}
		sender := "unknown"
		senderID := int64(0)
		if msg.FromID != nil {
			if peer, ok := msg.FromID.(*tg.PeerUser); ok {
				senderID = peer.UserID
				if u, ok := users[senderID]; ok {
					sender = u.Username
					if sender == "" {
						sender = u.FirstName + " " + u.LastName
					}
				}
			}
		}
		mediaType := ""
		if msg.Media != nil {
			mediaType = mediaTypeName(msg.Media)
		}
		outgoing := msg.Out
		items = append(items, MessageItem{
			MsgID:    int64(msg.ID),
			Date:     int64(msg.Date),
			Sender:   sender,
			SenderID: senderID,
			Text:     msg.Message,
			Media:    mediaType,
			Outgoing: outgoing,
		})
	}
	return items, nil
}

func extractMessageID(resp tg.UpdatesClass) (int64, error) {
	switch r := resp.(type) {
	case *tg.UpdateShortSentMessage:
		return int64(r.ID), nil
	case *tg.Updates:
		if len(r.Updates) > 0 {
			for _, u := range r.Updates {
				switch update := u.(type) {
				case *tg.UpdateNewMessage:
					if m, ok := update.Message.(*tg.Message); ok {
						return int64(m.ID), nil
					}
				}
			}
		}
		return 0, nil
	}
	return 0, nil
}

func extractUpdatesMessageIDs(resp tg.UpdatesClass) ([]int64, error) {
	switch r := resp.(type) {
	case *tg.Updates:
		var ids []int64
		for _, u := range r.Updates {
			switch update := u.(type) {
			case *tg.UpdateNewMessage:
				if m, ok := update.Message.(*tg.Message); ok {
					ids = append(ids, int64(m.ID))
				}
			case *tg.UpdateNewChannelMessage:
				if m, ok := update.Message.(*tg.Message); ok {
					ids = append(ids, int64(m.ID))
				}
			}
		}
		return ids, nil
	}
	return nil, nil
}

// peerInfo is used to resolve user/channel info from a dialog.
type peerInfo struct {
	peerType string
	peerID   int64
	title    string
	username string
}

func resolvePeerFromDialog(d *tg.Dialog, users []*tg.User, chats []*tg.Chat) peerInfo {
	peer := d.Peer
	switch p := peer.(type) {
	case *tg.PeerUser:
		for _, u := range users {
			if u.ID == p.UserID {
				name := u.Username
				if name == "" {
					name = strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return peerInfo{"user", u.ID, name, u.Username}
			}
		}
		return peerInfo{"user", p.UserID, fmt.Sprintf("user_%d", p.UserID), ""}
	case *tg.PeerChat:
		for _, c := range chats {
			if c.ID == p.ChatID {
				return peerInfo{"chat", c.ID, c.Title, ""}
			}
		}
		return peerInfo{"chat", p.ChatID, fmt.Sprintf("chat_%d", p.ChatID), ""}
	case *tg.PeerChannel:
		for _, c := range chats {
			if c.ID == p.ChannelID {
				return peerInfo{"channel", c.ID, c.Title, ""}
			}
		}
		return peerInfo{"channel", p.ChannelID, fmt.Sprintf("channel_%d", p.ChannelID), ""}
	}
	return peerInfo{"unknown", 0, "unknown", ""}
}

func usersByID(users []tg.UserClass) map[int64]*tg.User {
	m := make(map[int64]*tg.User, len(users))
	for _, c := range users {
		if u, ok := c.(*tg.User); ok {
			m[u.ID] = u
		}
	}
	return m
}

func usersFromClasses(classes []tg.UserClass) []*tg.User {
	var users []*tg.User
	for _, c := range classes {
		if u, ok := c.(*tg.User); ok {
			users = append(users, u)
		}
	}
	return users
}

func chatsFromClasses(classes []tg.ChatClass) []*tg.Chat {
	var chats []*tg.Chat
	for _, c := range classes {
		if ch, ok := c.(*tg.Chat); ok {
			chats = append(chats, ch)
		}
	}
	return chats
}

func intSliceFromInt64(in []int64) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

func mediaTypeName(media tg.MessageMediaClass) string {
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaGeo:
		return "geo"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaWebPage:
		return "webpage"
	case *tg.MessageMediaPoll:
		return "poll"
	default:
		return fmt.Sprintf("%T", media)
	}
}

package mtproto

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"

	"github.com/gotd/td/tg"
)

// DialogItem is a simplified dialog record for output.
type DialogItem struct {
	PeerType   string `json:"peer_type"`
	PeerID     int64  `json:"peer_id"`
	Title      string `json:"title"`
	Username   string `json:"username"`
	Unread     int    `json:"unread_count"`
	LastMsgID  int64  `json:"last_msg_id"`
	Pinned     bool   `json:"pinned"`
	AccessHash int64  `json:"access_hash,omitempty"`
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
	return GetHistoryWithOptions(ctx, api, peer, HistoryOptions{Limit: limit})
}

// HistoryOptions carries paging/range filters for GetHistoryWithOptions.
type HistoryOptions struct {
	// Limit caps the result set.
	Limit int
	// OffsetID pages to messages older than this id (newest-first walk).
	OffsetID int
	// MaxID returns only messages with IDs less than this (older window).
	MaxID int
	// MinID returns only messages with IDs greater than this (newer window).
	MinID int
	// Direction "oldest" jumps to the beginning of history (offset_id=max int);
	// "newest" (default) starts at the latest messages.
	Direction string
}

// GetHistoryWithOptions fetches message history with paging and id windows.
// Date/from filters are not available on getHistory, so callers with
// --since/--until/--from should use SearchMessagesWithOptions (peer-scoped
// search supports min_date/max_date/from_id directly).
func GetHistoryWithOptions(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, opts HistoryOptions) ([]MessageItem, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 30
	}
	offsetID := opts.OffsetID
	addOffset := 0
	if strings.EqualFold(opts.Direction, "oldest") && offsetID == 0 {
		// messages.getHistory walks backward from offset_id; max int lands
		// at the very beginning of the chat's history.
		offsetID = math.MaxInt32
		addOffset = -limit
	}
	req := &tg.MessagesGetHistoryRequest{
		Peer:      peer,
		OffsetID:  offsetID,
		AddOffset: addOffset,
		Limit:     limit,
		MaxID:     opts.MaxID,
		MinID:     opts.MinID,
	}
	resp, err := api.MessagesGetHistory(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return extractMessages(resp)
}

// SendMessageOptions carries optional send parameters: reply threading and
// scheduling. Zero values disable the feature.
type SendMessageOptions struct {
	// ReplyTo is the message id this message replies to (threads the reply).
	ReplyTo int64
	// ScheduleAt is the unix timestamp to schedule the message for.
	ScheduleAt int64
}

// SendMessage sends a plain text message to a peer.
func SendMessage(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, text string) (int64, error) {
	return SendMessageWithOptions(ctx, api, peer, text, SendMessageOptions{})
}

// SendMessageWithOptions sends a text message with optional reply threading
// (InputReplyToMessage) and scheduling (schedule_date).
func SendMessageWithOptions(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, text string, opts SendMessageOptions) (int64, error) {
	rnd := rand.Int63()
	req := &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: rnd,
	}
	if opts.ScheduleAt > 0 {
		req.ScheduleDate = int(opts.ScheduleAt)
	}
	if opts.ReplyTo > 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: int(opts.ReplyTo)}
	}
	resp, err := api.MessagesSendMessage(ctx, req)
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
		ID:       intSliceFromInt64(msgIDs),
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
		ID:     intSliceFromInt64(msgIDs),
		Revoke: revoke,
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
	return SearchMessagesWithOptions(ctx, api, query, SearchOptions{Limit: limit})
}

// SearchOptions carries the optional scoping for SearchMessagesWithOptions:
// peer scope, from-user, date range, filter type, and offset paging.
type SearchOptions struct {
	// Peer scopes the search to one chat; nil = global search.
	Peer tg.InputPeerClass
	// FromID restricts to messages sent by one user; nil = anyone.
	FromID tg.InputPeerClass
	// Limit caps the result set.
	Limit int
	// OffsetID pages to messages older than this id.
	OffsetID int
	// MinDate / MaxDate bound the message date (unix seconds).
	MinDate int64
	MaxDate int64
	// Filter is the TL message filter (photos, video, ...); nil = all.
	Filter tg.MessagesFilterClass
}

// MessageFilterForType maps a friendly filter name to the TL constructor.
func MessageFilterForType(t string) (tg.MessagesFilterClass, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "all":
		return &tg.InputMessagesFilterEmpty{}, nil
	case "photo", "photos":
		return &tg.InputMessagesFilterPhotos{}, nil
	case "video", "videos":
		return &tg.InputMessagesFilterVideo{}, nil
	case "document", "file", "files":
		return &tg.InputMessagesFilterDocument{}, nil
	case "url", "link", "links":
		return &tg.InputMessagesFilterURL{}, nil
	case "gif", "animation":
		return &tg.InputMessagesFilterGif{}, nil
	case "voice", "voice-message":
		return &tg.InputMessagesFilterVoice{}, nil
	case "music", "audio":
		return &tg.InputMessagesFilterMusic{}, nil
	case "sticker", "stickers":
		return &tg.InputMessagesFilterPhotoVideo{}, nil // stickers are image-based; closest TL filter
	case "poll", "polls":
		return &tg.InputMessagesFilterPoll{}, nil
	case "geo", "location":
		return &tg.InputMessagesFilterGeo{}, nil
	case "pinned":
		return &tg.InputMessagesFilterPinned{}, nil
	default:
		return nil, fmt.Errorf("unknown --type %q (valid: photo, video, document, url, gif, voice, music, sticker, poll, geo, pinned)", t)
	}
}

// SearchMessagesWithOptions searches messages with peer scope, from-user,
// date range, filter type, and offset paging.
func SearchMessagesWithOptions(ctx context.Context, api *tg.Client, query string, opts SearchOptions) ([]MessageItem, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 30
	}
	peer := opts.Peer
	if peer == nil {
		peer = &tg.InputPeerEmpty{} // global search across all chats
	}
	filter := opts.Filter
	if filter == nil {
		filter = &tg.InputMessagesFilterEmpty{}
	}
	req := &tg.MessagesSearchRequest{
		Q:      query,
		Peer:   peer,
		Filter: filter,
		Limit:  limit,
	}
	if opts.OffsetID > 0 {
		req.OffsetID = int(opts.OffsetID)
	}
	if opts.MinDate > 0 {
		req.MinDate = int(opts.MinDate)
	}
	if opts.MaxDate > 0 {
		req.MaxDate = int(opts.MaxDate)
	}
	if opts.FromID != nil {
		req.SetFromID(opts.FromID)
	}
	resp, err := api.MessagesSearch(ctx, req)
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

// HistoryFromResponse extracts MessageItems from a MessagesGetHistory response.
func HistoryFromResponse(resp tg.MessagesMessagesClass) ([]MessageItem, error) {
	return extractMessages(resp)
}

// ResolveUsernameLive resolves an @username against the live session via
// contacts.resolveUsername and returns the matching InputPeerClass. Use it to
// wire a PeerResolver.Live fallback inside DialAndRun.
func ResolveUsernameLive(ctx context.Context, api *tg.Client, username string) (tg.InputPeerClass, error) {
	res, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, fmt.Errorf("resolve username @%s: %w", username, err)
	}
	switch p := res.Peer.(type) {
	case *tg.PeerUser:
		for _, c := range res.Users {
			if u, ok := c.(*tg.User); ok && u.ID == p.UserID {
				return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}, nil
			}
		}
		return &tg.InputPeerUser{UserID: p.UserID}, nil
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}, nil
	case *tg.PeerChannel:
		for _, c := range res.Chats {
			if ch, ok := c.(*tg.Channel); ok && ch.ID == p.ChannelID {
				return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}, nil
			}
		}
		return &tg.InputPeerChannel{ChannelID: p.ChannelID}, nil
	default:
		return nil, fmt.Errorf("unexpected resolved peer type %T", res.Peer)
	}
}

// --- Helper extractors ---

func extractDialogs(resp tg.MessagesDialogsClass) ([]DialogItem, error) {
	var items []DialogItem
	var users []*tg.User
	var chats []tg.ChatClass
	var dialogs []tg.DialogClass

	switch r := resp.(type) {
	case *tg.MessagesDialogsSlice:
		dialogs = r.Dialogs
		users = usersFromClasses(r.Users)
		chats = r.Chats
	case *tg.MessagesDialogs:
		dialogs = r.Dialogs
		users = usersFromClasses(r.Users)
		chats = r.Chats
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
			PeerType:   peer.peerType,
			PeerID:     peer.peerID,
			Title:      peer.title,
			Username:   peer.username,
			Unread:     dialog.UnreadCount,
			LastMsgID:  int64(dialog.TopMessage),
			Pinned:     dialog.Pinned,
			AccessHash: peer.accessHash,
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
	peerType   string
	peerID     int64
	title      string
	username   string
	accessHash int64
}

func resolvePeerFromDialog(d *tg.Dialog, users []*tg.User, chats []tg.ChatClass) peerInfo {
	peer := d.Peer
	switch p := peer.(type) {
	case *tg.PeerUser:
		for _, u := range users {
			if u.ID == p.UserID {
				name := u.Username
				if name == "" {
					name = strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return peerInfo{"user", u.ID, name, u.Username, u.AccessHash}
			}
		}
		return peerInfo{"user", p.UserID, fmt.Sprintf("user_%d", p.UserID), "", 0}
	case *tg.PeerChat:
		for _, c := range chats {
			if ch, ok := c.(*tg.Chat); ok && ch.ID == p.ChatID {
				return peerInfo{"chat", ch.ID, ch.Title, "", 0}
			}
		}
		return peerInfo{"chat", p.ChatID, fmt.Sprintf("chat_%d", p.ChatID), "", 0}
	case *tg.PeerChannel:
		for _, c := range chats {
			if ch, ok := c.(*tg.Channel); ok && ch.ID == p.ChannelID {
				return peerInfo{"channel", ch.ID, ch.Title, "", ch.AccessHash}
			}
		}
		return peerInfo{"channel", p.ChannelID, fmt.Sprintf("channel_%d", p.ChannelID), "", 0}
	}
	return peerInfo{"unknown", 0, "unknown", "", 0}
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

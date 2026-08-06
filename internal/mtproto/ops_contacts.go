package mtproto

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"
)

// inputUserFromPeer converts a resolved InputPeerClass to the InputUserClass
// required by contacts/users/channels admin calls. Returns an error when the
// peer is not a user (e.g. a channel or legacy chat).
func inputUserFromPeer(peer tg.InputPeerClass) (tg.InputUserClass, error) {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		return &tg.InputUser{UserID: p.UserID, AccessHash: p.AccessHash}, nil
	case *tg.InputPeerSelf:
		return &tg.InputUserSelf{}, nil
	default:
		return nil, fmt.Errorf("expected a user peer, got %T", peer)
	}
}

// inputChannelFromPeer converts a resolved InputPeerClass to InputChannelClass.
func inputChannelFromPeer(peer tg.InputPeerClass) (tg.InputChannelClass, error) {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		return &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash}, nil
	default:
		return nil, fmt.Errorf("expected a channel peer, got %T", peer)
	}
}

// AddContact adds a user to the account's contact list.
func AddContact(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, firstName, lastName, phone string) error {
	user, err := inputUserFromPeer(peer)
	if err != nil {
		return err
	}
	_, err = api.ContactsAddContact(ctx, &tg.ContactsAddContactRequest{
		ID:                       user,
		FirstName:                firstName,
		LastName:                 lastName,
		Phone:                    phone,
		AddPhonePrivacyException: true,
	})
	if err != nil {
		return fmt.Errorf("add contact: %w", err)
	}
	return nil
}

// DeleteContacts removes users from the account's contact list.
func DeleteContacts(ctx context.Context, api *tg.Client, peers []tg.InputPeerClass) (int, error) {
	ids := make([]tg.InputUserClass, 0, len(peers))
	for _, p := range peers {
		u, err := inputUserFromPeer(p)
		if err != nil {
			return 0, err
		}
		ids = append(ids, u)
	}
	if _, err := api.ContactsDeleteContacts(ctx, ids); err != nil {
		return 0, fmt.Errorf("delete contacts: %w", err)
	}
	return len(ids), nil
}

// GetFullUser fetches the full user profile (about, common chats, blocked status).
func GetFullUser(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) (*tg.UsersUserFull, error) {
	user, err := inputUserFromPeer(peer)
	if err != nil {
		return nil, err
	}
	full, err := api.UsersGetFullUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("get full user: %w", err)
	}
	return full, nil
}

// BlockUser blocks a user.
func BlockUser(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) error {
	if _, err := api.ContactsBlock(ctx, &tg.ContactsBlockRequest{ID: peer}); err != nil {
		return fmt.Errorf("block user: %w", err)
	}
	return nil
}

// UnblockUser unblocks a user.
func UnblockUser(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) error {
	if _, err := api.ContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: peer}); err != nil {
		return fmt.Errorf("unblock user: %w", err)
	}
	return nil
}

// SearchContacts searches chats/contacts by name via contacts.search.
func SearchContacts(ctx context.Context, api *tg.Client, q string, limit int) ([]DialogItem, error) {
	if limit <= 0 {
		limit = 30
	}
	found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: q, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("search contacts: %w", err)
	}
	// Index chats/users by id so each result peer gets a title.
	chatTitles := make(map[int64]string)
	userNames := make(map[int64]string)
	for _, c := range found.Chats {
		if ch, ok := c.(*tg.Channel); ok {
			chatTitles[ch.ID] = ch.Title
		}
		if g, ok := c.(*tg.Chat); ok {
			chatTitles[-g.ID] = g.Title
		}
	}
	for _, u := range found.Users {
		if usr, ok := u.(*tg.User); ok {
			name := usr.FirstName + " " + usr.LastName
			userNames[usr.ID] = name
		}
	}
	var out []DialogItem
	for _, res := range found.Results {
		item := DialogItem{}
		switch p := res.(type) {
		case *tg.PeerUser:
			item.PeerType = "user"
			item.PeerID = p.UserID
			item.Title = strings.TrimSpace(userNames[p.UserID])
			if item.Title == "" {
				item.Title = userNames[p.UserID]
			}
		case *tg.PeerChat:
			item.PeerType = "chat"
			item.PeerID = p.ChatID
			item.Title = chatTitles[-p.ChatID]
		case *tg.PeerChannel:
			item.PeerType = "channel"
			item.PeerID = p.ChannelID
			item.Title = chatTitles[p.ChannelID]
		default:
			continue
		}
		if item.Title == "" {
			item.Title = "(untitled)"
		}
		out = append(out, item)
	}
	return out, nil
}

// GetBlocked lists blocked users.
func GetBlocked(ctx context.Context, api *tg.Client, limit int) ([]*tg.User, error) {
	if limit <= 0 {
		limit = 30
	}
	resp, err := api.ContactsGetBlocked(ctx, &tg.ContactsGetBlockedRequest{Offset: 0, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("get blocked: %w", err)
	}
	switch r := resp.(type) {
	case *tg.ContactsBlocked:
		var out []*tg.User
		for _, uc := range r.Users {
			if u, ok := uc.(*tg.User); ok {
				out = append(out, u)
			}
		}
		return out, nil
	case *tg.ContactsBlockedSlice:
		var out []*tg.User
		for _, uc := range r.Users {
			if u, ok := uc.(*tg.User); ok {
				out = append(out, u)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected getBlocked response %T", resp)
	}
}

// LeaveChat leaves a legacy group chat or a channel.
func LeaveChat(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) error {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		ch, err := inputChannelFromPeer(p)
		if err != nil {
			return err
		}
		if _, err := api.ChannelsLeaveChannel(ctx, ch); err != nil {
			return fmt.Errorf("leave channel: %w", err)
		}
		return nil
	case *tg.InputPeerChat:
		// Leaving a legacy group requires the current user: messages.deleteChatUser
		// with our own InputUserSelf.
		if _, err := api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
			ChatID: p.ChatID,
			UserID: &tg.InputUserSelf{},
		}); err != nil {
			return fmt.Errorf("leave chat: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("cannot leave %T: only groups and channels can be left", peer)
	}
}

// DeleteChat deletes a legacy group chat or a channel, optionally revoking the
// history for all participants (channels always delete permanently).
func DeleteChat(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, revoke bool) error {
	switch p := peer.(type) {
	case *tg.InputPeerChannel:
		ch, err := inputChannelFromPeer(p)
		if err != nil {
			return err
		}
		if _, err := api.ChannelsDeleteChannel(ctx, ch); err != nil {
			return fmt.Errorf("delete channel: %w", err)
		}
		return nil
	case *tg.InputPeerChat:
		if _, err := api.MessagesDeleteHistory(ctx, &tg.MessagesDeleteHistoryRequest{
			Peer:   p,
			Revoke: revoke,
		}); err != nil {
			return fmt.Errorf("delete chat history: %w", err)
		}
		if _, err := api.MessagesDeleteChat(ctx, p.ChatID); err != nil {
			return fmt.Errorf("delete chat: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("cannot delete %T: only groups and channels can be deleted", peer)
	}
}

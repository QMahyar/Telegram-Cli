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

type contactItem struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Bot       bool   `json:"bot,omitempty"`
}

func userToContact(u *tg.User) contactItem {
	return contactItem{
		ID:        u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
		Phone:     u.Phone,
		Bot:       u.Bot,
	}
}

func newContactsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "contacts [query]",
		Short:       "List, search, add, remove, or inspect Telegram contacts",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Args:        cobra.MaximumNArgs(1),
		Example:     "  telegram-cli contacts\n  telegram-cli contacts alice --limit 10\n  telegram-cli contacts add @alice --first-name Alice\n  telegram-cli contacts remove @alice\n  telegram-cli contacts info @alice",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list search contacts")
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
			var contacts []contactItem
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				if len(args) > 0 {
					res, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{
						Q:     args[0],
						Limit: f.Limit,
					})
					if err != nil {
						return err
					}
					for _, c := range res.Users {
						if u, ok := c.(*tg.User); ok {
							contacts = append(contacts, userToContact(u))
						}
					}
					return nil
				}
				resp, err := api.ContactsGetContacts(ctx, 0)
				if err != nil {
					return err
				}
				switch r := resp.(type) {
				case *tg.ContactsContacts:
					users := make(map[int64]*tg.User, len(r.Users))
					for _, uc := range r.Users {
						if u, ok := uc.(*tg.User); ok {
							users[u.ID] = u
						}
					}
					for _, c := range r.Contacts {
						if u, ok := users[c.UserID]; ok {
							contacts = append(contacts, userToContact(u))
						}
					}
				case *tg.ContactsContactsNotModified:
					return nil
				default:
					return nil
				}
				return nil
			})
			if err != nil {
				return err
			}
			if contacts == nil {
				contacts = []contactItem{}
			}
			return outResult(stdout(), f, contacts)
		},
	}
	addTelegramFlags(cmd)
	cmd.AddCommand(newContactsAddCmd(flags))
	cmd.AddCommand(newContactsRemoveCmd(flags))
	cmd.AddCommand(newContactsInfoCmd(flags))
	return cmd
}

// newContactsAddCmd adds a user to the contact list (contacts.addContact).
func newContactsAddCmd(flags *rootFlags) *cobra.Command {
	var firstName, lastName, phone string
	cmd := &cobra.Command{
		Use:   "add <user>",
		Short: "Add a user to your contacts",
		Example: `  telegram-cli contacts add @alice --first-name Alice
  telegram-cli contacts add @alice --first-name Alice --last-name Wonder
  telegram-cli contacts add +15551234567 --first-name Bob --phone +15551234567`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "add contact")
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
				return mtproto.AddContact(ctx, api, peer, firstName, lastName, phone)
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Added %s to contacts.\n", ref)
			return mutationResult(f, map[string]any{"added": ref})
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name for the contact")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name for the contact")
	cmd.Flags().StringVar(&phone, "phone", "", "phone number (for phone-number contacts)")
	addTelegramFlags(cmd)
	return cmd
}

// newContactsRemoveCmd removes users from the contact list (contacts.deleteContacts).
func newContactsRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <user...>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove users from your contacts",
		Example: `  telegram-cli contacts remove @alice @bob`,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "remove contacts")
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
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "would remove %d contact(s)\n", len(args))
				return confirmationErr(fmt.Errorf("contacts remove requires --yes confirmation"), "re-run with --yes to proceed, or --dry-run to preview")
			}
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var removed int
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peers := make([]tg.InputPeerClass, 0, len(args))
				for _, ref := range args {
					peer, err := resolver.Resolve(ctx, alias, ref)
					if err != nil {
						return err
					}
					peers = append(peers, peer)
				}
				n, err := mtproto.DeleteContacts(ctx, api, peers)
				removed = n
				return err
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			fmt.Fprintf(os.Stderr, "Removed %d contact(s).\n", removed)
			return mutationResult(f, map[string]any{"removed": removed})
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// newContactsInfoCmd shows a full user profile (users.getFullUser).
func newContactsInfoCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <user>",
		Aliases: []string{"get", "show"},
		Short:   "Show a full user profile: about, common chats, blocked status",
		Example: `  telegram-cli contacts info @alice`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			var result map[string]any
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				full, err := mtproto.GetFullUser(ctx, api, peer)
				if err != nil {
					return err
				}
				u := full.Users[0]
				user, _ := u.(*tg.User)
				result = map[string]any{
					"id":           user.ID,
					"username":     user.Username,
					"first_name":   user.FirstName,
					"last_name":    user.LastName,
					"phone":        user.Phone,
					"bot":          user.Bot,
					"about":        full.FullUser.About,
					"blocked":      full.FullUser.Blocked,
					"common_chats": full.FullUser.CommonChatsCount,
					"verified":     user.Verified,
					"premium":      user.Premium,
				}
				return nil
			})
			if err != nil {
				return err
			}
			markAccountUsed(ctx, s, alias)
			return outResult(stdout(), f, result)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

package cli

import (
	"context"

	"telegram-cli/internal/config"

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
		Short:       "List or search Telegram contacts",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		Example:     "  tele contacts\n  tele contacts alice --limit 10",
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
			alias, err := resolveAccount(ctx, s, "")
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
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
	return cmd
}

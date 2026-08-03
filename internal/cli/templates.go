package cli

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

type templateEntry struct {
	Name      string `json:"name"`
	Text      string `json:"text"`
	UpdatedAt string `json:"updated_at"`
}

func newTemplatesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "templates",
		Short:       "Manage broadcast templates",
		Annotations: map[string]string{"mcp:read-only": "true", "cli:api-resource": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(
		newTemplatesListCmd(flags),
		newTemplatesAddCmd(flags),
		newTemplatesShowCmd(flags),
		newTemplatesRemoveCmd(flags),
	)
	return cmd
}

func newTemplatesListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List broadcast templates",
		Annotations: map[string]string{"mcp:read-only": "true"},
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
			rows, err := s.DB().QueryContext(ctx,
				`SELECT name, text, updated_at FROM tg_templates ORDER BY name`)
			if err != nil {
				return err
			}
			defer rows.Close()
			var entries []templateEntry
			for rows.Next() {
				var e templateEntry
				if err := rows.Scan(&e.Name, &e.Text, &e.UpdatedAt); err != nil {
					return err
				}
				entries = append(entries, e)
			}
			if entries == nil {
				entries = []templateEntry{}
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, entries)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newTemplatesAddCmd(flags *rootFlags) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create or update a broadcast template",
		Example: `  tele templates add welcome --text "Welcome to the group!"
  tele templates add promo --text "Flash sale ends soon!"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if text == "" {
				return fmt.Errorf("--text is required")
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
			_, err = s.DB().ExecContext(ctx,
				`INSERT INTO tg_templates (name, text, updated_at) VALUES (?, ?, ?)
				 ON CONFLICT(name) DO UPDATE SET text = excluded.text, updated_at = excluded.updated_at`,
				args[0], text, time.Now().UTC().Format(time.RFC3339),
			)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Saved template %q.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "template message text")
	return cmd
}

func newTemplatesShowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "show <name>",
		Short:       "Show a template's text",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
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
			var e templateEntry
			err = s.DB().QueryRowContext(ctx,
				`SELECT name, text, updated_at FROM tg_templates WHERE name = ?`, args[0],
			).Scan(&e.Name, &e.Text, &e.UpdatedAt)
			if err == sql.ErrNoRows {
				return fmt.Errorf("template %q not found", args[0])
			}
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, e)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func newTemplatesRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a broadcast template",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
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
			res, err := s.DB().ExecContext(ctx, `DELETE FROM tg_templates WHERE name = ?`, args[0])
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("template %q not found", args[0])
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Removed template %q.\n", args[0])
			return nil
		},
	}
	return cmd
}

// resolveTemplateText looks up a broadcast template in the mirror store.
// Returns the template text, or an error when the template is unknown.
func resolveTemplateText(ctx context.Context, db *sql.DB, name string) (string, error) {
	var text string
	err := db.QueryRowContext(ctx, `SELECT text FROM tg_templates WHERE name = ?`, name).Scan(&text)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("template %q not found (create it with: templates add %s --text \"...\")", name, name)
	}
	return text, err
}

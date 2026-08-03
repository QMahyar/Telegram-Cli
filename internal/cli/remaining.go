package cli

import (
	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

func newWatchCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "watch",
		Short:       "Monitor new messages in real-time (streams updates)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: implement update stream using gotd/td updates.Manager
			return cmd.Help()
		},
	}
	return cmd
}

func newExportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "export [chat]",
		Short:       "Export chat history or media to local files",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := config.HomeDir(flags.homePath)
			if err != nil {
				return err
			}
			_ = home
			// TODO: implement export (messages to JSON, media to files)
			return cmd.Help()
		},
	}
	return cmd
}

func newTopicsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "topics <group>",
		Short:       "List topics in a forum group",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: implement topics listing
			return cmd.Help()
		},
	}
	return cmd
}

func newContactsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "contacts",
		Short:       "List or search Telegram contacts",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: implement contacts list
			return cmd.Help()
		},
	}
	return cmd
}

func newRawCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw <method> [params-json]",
		Short: "Invoke a raw MTProto method (advanced)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: implement raw invoke
			return cmd.Help()
		},
	}
	return cmd
}

func newCapabilitiesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "capabilities",
		Short:       "Show what this CLI can do",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: list all commands with descriptions
			return cmd.Parent().Help()
		},
	}
	return cmd
}

func newTemplatesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "templates",
		Short:       "Manage broadcast templates",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	return cmd
}

func newAuditCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "audit",
		Short:       "Show audit log of CLI operations",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: read from tg_audit table
			return cmd.Help()
		},
	}
	return cmd
}

func newConfigCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "config",
		Short:       "Show or set CLI configuration",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := config.HomeDir(flags.homePath)
			cmd.Printf("Home: %s\n", home)
			return nil
		},
	}
	return cmd
}

func newSQLCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "sql <query>",
		Short:       "Run a read-only SQL query against the mirror database",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: execute SQL query on mirror DB
			return cmd.Help()
		},
	}
	return cmd
}

func newMirrorCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "mirror",
		Short:       "Show mirror sync status",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newMirrorStatusCmd(flags))
	return cmd
}

func newMirrorStatusCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show mirror database statistics",
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
			type mirrorStatus struct {
				Accounts  int `json:"accounts"`
				Dialogs   int `json:"dialogs"`
				Messages  int `json:"messages"`
				Jobs      int `json:"jobs"`
			}
			var ms mirrorStatus
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_accounts`).Scan(&ms.Accounts)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_dialogs`).Scan(&ms.Dialogs)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_messages`).Scan(&ms.Messages)
			s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tg_jobs`).Scan(&ms.Jobs)
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, ms)
		},
	}
}

package cli

import (
	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

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

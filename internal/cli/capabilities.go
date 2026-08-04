package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

type capabilityEntry struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Group       string `json:"group,omitempty"`
}

func newCapabilitiesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "capabilities",
		Short:       "Show what this CLI can do",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var caps []capabilityEntry
			for _, c := range cmd.Root().Commands() {
				if !c.IsAvailableCommand() || c.Name() == "help" || c.Hidden {
					continue
				}
				caps = append(caps, capabilityEntry{
					Command:     c.Name(),
					Description: c.Short,
					Group:       c.Annotations["group"],
				})
			}
			sort.Slice(caps, func(i, j int) bool { return caps[i].Command < caps[j].Command })
			f := parseTelegramFlags(cmd)
			return outResult(stdout(), f, caps)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

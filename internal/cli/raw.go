package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"telegram-cli/internal/config"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

// newRawCmd invokes a raw MTProto method via the schema-driven JSON gateway
// (gotd's tdjson bridge). The first argument is the TL method name (e.g.
// "help.getNearestDc") and the optional second argument is a JSON object of
// parameters. The response is returned as MTProto JSON.
func newRawCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "raw <method> [params-json]",
		Short:   "Invoke a raw MTProto method (advanced)",
		Aliases: []string{"invoke"},
		Args:    cobra.RangeArgs(1, 2),
		Long: `Invokes any MTProto method by its TL name, converting JSON parameters
to the binary wire format and back. This is the schema-driven raw gateway:
anything the Telegram TL layer exposes can be called here, bypassing the
hand-written friendly commands.`,
		Example: `  telegram-cli raw help.getNearestDc
  telegram-cli raw account.getAccountTTL
  telegram-cli raw messages.getDialogs '{"limit": 10}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "invoke raw MTProto method "+args[0])
			}
			method := args[0]
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
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			envelope := map[string]any{"@type": method}
			if len(args) > 1 {
				var params map[string]any
				if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
					return fmt.Errorf("params must be a JSON object: %w", err)
				}
				for k, v := range params {
					envelope[k] = v
				}
			}
			payload, err := json.Marshal(envelope)
			if err != nil {
				return err
			}
			var response string
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				response, err = api.InvokeJSON(ctx, string(payload), true)
				return err
			})
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			if f.JSON {
				var out any
				if json.Unmarshal([]byte(response), &out) == nil {
					return outResult(stdout(), f, out)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), response)
			return nil
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

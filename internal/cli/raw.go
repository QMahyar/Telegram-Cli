package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	var schemaFlag string
	cmd := &cobra.Command{
		Use:     "raw <method> [params-json]",
		Short:   "Invoke a raw MTProto method (advanced)",
		Aliases: []string{"invoke"},
		Args: func(cmd *cobra.Command, args []string) error {
			schemaFlag, _ := cmd.Flags().GetString("schema")
			if schemaFlag != "" {
				if len(args) > 1 {
					return fmt.Errorf("accepts at most 1 arg with --schema, received %d", len(args))
				}
				return nil
			}
			if len(args) >= 1 && args[0] == "list" {
				if len(args) > 2 {
					return fmt.Errorf("raw list accepts at most 1 interface name, received %d", len(args)-1)
				}
				return nil
			}
			return cobra.RangeArgs(1, 2)(cmd, args)
		},
		Long: `Invokes any MTProto method by its TL name, converting JSON parameters
		to the binary wire format and back. This is the schema-driven raw gateway:
		anything the Telegram TL layer exposes can be called here, bypassing the
		hand-written friendly commands.

		"raw list [interface]" enumerates the compiled TL methods; "raw --schema
		<method>" prints a method's parameters; "--dry-run" validates the method
		name and JSON parameter shape without dialing Telegram.`,
		Example: `  telegram-cli raw list
  telegram-cli raw list messages
  telegram-cli raw --schema messages.getDialogs
  telegram-cli raw messages.getDialogs '{"limit": 10}' --dry-run
  telegram-cli raw help.getNearestDc
  telegram-cli raw account.getAccountTTL
  telegram-cli raw messages.getDialogs '{"limit": 10}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// "raw list [interface]" is a pure registry read — no dial, no
			// account context required.
			if len(args) > 0 && args[0] == "list" {
				return rawList(cmd, args[1:], schemaFlag)
			}
			method := ""
			if len(args) > 0 {
				method = args[0]
			}
			// --schema <method> mode: prints the schema without dialing. With
			// no positional args the flag value is the method.
			if schemaFlag != "" {
				if method != "" && method != schemaFlag {
					return usageErr(fmt.Errorf("conflicting method %q and --schema %q", method, schemaFlag))
				}
				method = schemaFlag
			}
			if method == "" {
				return cmd.Help()
			}
			// --schema mode prints the parameter schema — a pure registry read.
			if schemaFlag != "" {
				f := parseTelegramFlags(cmd)
				schema, err := rawMethodSchema(method)
				if err != nil {
					return usageErr(err)
				}
				return outResult(stdout(), f, map[string]any{"method": method, "params": schema})
			}
			// Validate the method name against the compiled schema before
			// anything else — an unknown method is a usage error (exit 2), not
			// a network failure.
			if !rawMethodExists(method) {
				return usageErr(fmt.Errorf("unknown TL method %q — run `raw list` or check the spelling", method))
			}
			// Parse + validate params before dialing so bad shapes fail fast.
			var params map[string]any
			if len(args) > 1 {
				if err := json.Unmarshal([]byte(args[1]), &params); err != nil {
					return usageErr(fmt.Errorf("params must be a JSON object: %w", err))
				}
				if err := validateRawParams(method, params); err != nil {
					return usageErr(err)
				}
			}
			if dryRunOK(flags) {
				f := parseTelegramFlags(cmd)
				schema, _ := rawMethodSchema(method)
				return outResult(stdout(), f, map[string]any{
					"method":       method,
					"params_valid": true,
					"params":       params,
					"schema":       schema,
				})
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
			envelope := map[string]any{"@type": method}
			for k, v := range params {
				envelope[k] = v
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
				return rawErrHint(method, err)
			}
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
	cmd.Flags().StringVar(&schemaFlag, "schema", "", "print the parameter schema for a method (no dial; accepts an interface to list)")
	addTelegramFlags(cmd)
	return cmd
}

// rawList enumerates compiled TL methods, optionally filtered by the first
// namespace segment (e.g. "messages"). With --schema <method> it prints the
// schema for one method instead.
func rawList(cmd *cobra.Command, args []string, schemaFlag string) error {
	f := parseTelegramFlags(cmd)
	if schemaFlag != "" {
		schema, err := rawMethodSchema(schemaFlag)
		if err != nil {
			return usageErr(err)
		}
		return outResult(stdout(), f, map[string]any{"method": schemaFlag, "params": schema})
	}
	names := rawMethodNames()
	if len(args) > 0 {
		prefix := strings.TrimPrefix(args[0], ".") + "."
		var filtered []string
		for _, n := range names {
			if strings.HasPrefix(n, prefix) {
				filtered = append(filtered, n)
			}
		}
		names = filtered
	}
	if len(names) == 0 {
		return notFoundErr(fmt.Errorf("no TL methods match %q — try `raw list`", strings.Join(args, " ")))
	}
	return outResult(stdout(), f, names)
}

// rawErrHint maps common TD errors to actionable hints (see P2-3).
func rawErrHint(method string, err error) error {
	return fmt.Errorf("raw %s failed: %w%s", method, err, rawErrSuffix(err))
}

func rawErrSuffix(err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "flood_wait") || strings.Contains(lower, "flood") && strings.Contains(lower, "wait"):
		return " — flood wait: retry after the cooldown (see accounts health)"
	case strings.Contains(lower, "peer_id_invalid"):
		return " — resolve the peer with @username first (or sync to cache it)"
	case strings.Contains(lower, "auth_key_unregistered") || strings.Contains(lower, "session_revoked") || strings.Contains(lower, "unauthorized"):
		return " — session revoked: re-run accounts add for this alias"
	case strings.Contains(lower, "method_not_found"):
		return " — method unknown to the server: wrong TL layer or typo (try raw list)"
	case strings.Contains(lower, "phone_number_banned"):
		return " — phone banned: do not retry"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "unexpected eof") || strings.Contains(lower, "eof"):
		return " — network/session issue: run accounts health to probe"
	default:
		return ""
	}
}

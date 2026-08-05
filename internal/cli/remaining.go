package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

// newConfigCmd builds the real config command tree: show (default), path,
// get, set, unset. Reads are read-only; set/unset edit the resolved
// config.toml atomically and never persist env overrides.
func newConfigCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or set CLI configuration (config.toml)",
		Long: `Inspect and edit the CLI's TOML configuration file.

Keys:
  base_url       API base URL (env override: TELEGRAM_BASE_URL)
  auth_header    static Authorization header value; show/get never echo it back
  headers.<name> a static request header, e.g. headers.X-Tenant my-tenant

Reads never touch the file; set and unset rewrite it in place at the resolved
path (--config, TELEGRAM_CONFIG, or the platform config directory).`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErr(fmt.Errorf("unknown argument %q for %q\nRun '%s --help' for available subcommands", args[0], cmd.CommandPath(), cmd.CommandPath()))
			}
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			return showConfig(cmd, flags, cfg)
		},
	}
	cmd.AddCommand(
		newConfigPathCmd(flags),
		newConfigGetCmd(flags),
		newConfigSetCmd(flags),
		newConfigUnsetCmd(flags),
	)
	return cmd
}

func newConfigPathCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "path",
		Short:       "Print the resolved config file path",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"config_path": cfg.Path}, flags)
			}
			cmd.Println(cfg.Path)
			return nil
		},
	}
}

func newConfigGetCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "get <key>",
		Short:       "Print one configuration value (empty when unset)",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			value, err := configValue(cfg, key)
			if err != nil {
				return usageErr(err)
			}
			// auth_header is a credential: never echo it back.
			if key == "auth_header" && value != "" {
				value = "<redacted>"
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"key": key, "value": value}, flags)
			}
			cmd.Println(value)
			return nil
		},
	}
}

func newConfigSetCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value in the config file",
		Args:  cobra.ExactArgs(2),
		Annotations: map[string]string{
			"cli:typed-exit-codes": "0,2,10",
		},
		Example: `  telegram-cli config set base_url https://api.example.com
  telegram-cli config set headers.X-Tenant my-tenant
  telegram-cli config unset headers.X-Tenant`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if strings.TrimSpace(value) == "" {
				return usageErr(fmt.Errorf("value for %q must not be empty", key))
			}
			cfg, err := config.LoadForEdit(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := setConfigValue(cfg, key, value); err != nil {
				return usageErr(err)
			}
			if err := config.Save(cfg.Path, cfg); err != nil {
				return configErr(err)
			}
			if key == "auth_header" && !flags.asJSON {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: wrote a credential to %s; keep this file private (0o600)\n", cfg.Path)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"key": key, "config_path": cfg.Path, "updated": true}, flags)
			}
			cmd.Printf("set %s in %s\n", key, cfg.Path)
			return nil
		},
	}
}

func newConfigUnsetCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "unset <key>",
		Short:       "Remove a configuration value from the config file",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"cli:typed-exit-codes": "0,2,10"},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			cfg, err := config.LoadForEdit(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			if err := unsetConfigValue(cfg, key); err != nil {
				return usageErr(err)
			}
			if err := config.Save(cfg.Path, cfg); err != nil {
				return configErr(err)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"key": key, "config_path": cfg.Path, "updated": true}, flags)
			}
			cmd.Printf("unset %s in %s\n", key, cfg.Path)
			return nil
		},
	}
}

func showConfig(cmd *cobra.Command, flags *rootFlags, cfg *config.Config) error {
	home, err := config.HomeDir(flags.homePath)
	if err != nil {
		return err
	}
	if flags.asJSON {
		view := map[string]any{
			"config_path": cfg.Path,
			"home_dir":    home,
			"base_url":    cfg.BaseURL,
		}
		if len(cfg.Headers) > 0 {
			view["headers"] = cfg.Headers
		}
		if cfg.AuthHeader() != "" {
			view["auth_header"] = "<redacted>"
		}
		if cfg.AuthSource != "" {
			view["auth_source"] = cfg.AuthSource
		}
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}

	rows := [][]string{
		{"config_path", cfg.Path},
		{"home_dir", home},
		{"base_url", cfg.BaseURL},
	}
	names := make([]string, 0, len(cfg.Headers))
	for k := range cfg.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		rows = append(rows, []string{"headers." + k, cfg.Headers[k]})
	}
	if cfg.AuthHeader() != "" {
		rows = append(rows, []string{"auth_header", "<redacted>"})
	}
	if cfg.AuthSource != "" {
		rows = append(rows, []string{"auth_source", cfg.AuthSource})
	}
	return flags.printTable(cmd, []string{"KEY", "VALUE"}, rows)
}

// configKeyGrammar returns the human-readable key grammar for error messages.
const configKeyGrammar = "valid keys: base_url, auth_header, headers.<name>"

// splitConfigKey parses a config key into its kind and (for headers) name.
func splitConfigKey(key string) (kind, name string, err error) {
	switch {
	case key == "base_url":
		return "base_url", "", nil
	case key == "auth_header":
		return "auth_header", "", nil
	case strings.HasPrefix(key, "headers.") && len(key) > len("headers."):
		return "headers", key[len("headers."):], nil
	default:
		return "", "", fmt.Errorf("unknown config key %q (%s)", key, configKeyGrammar)
	}
}

func configValue(cfg *config.Config, key string) (string, error) {
	kind, name, err := splitConfigKey(key)
	if err != nil {
		return "", err
	}
	switch kind {
	case "base_url":
		return cfg.BaseURL, nil
	case "auth_header":
		return cfg.AuthHeader(), nil
	case "headers":
		if cfg.Headers == nil {
			return "", nil
		}
		return cfg.Headers[name], nil
	}
	return "", fmt.Errorf("unknown config key %q (%s)", key, configKeyGrammar)
}

func setConfigValue(cfg *config.Config, key, value string) error {
	kind, name, err := splitConfigKey(key)
	if err != nil {
		return err
	}
	switch kind {
	case "base_url":
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("base_url must be an absolute http(s) URL (got %q)", value)
		}
		cfg.BaseURL = value
	case "auth_header":
		cfg.AuthHeaderVal = value
	case "headers":
		if cfg.Headers == nil {
			cfg.Headers = map[string]string{}
		}
		cfg.Headers[name] = value
	}
	return nil
}

func unsetConfigValue(cfg *config.Config, key string) error {
	kind, name, err := splitConfigKey(key)
	if err != nil {
		return err
	}
	switch kind {
	case "base_url":
		cfg.BaseURL = ""
	case "auth_header":
		cfg.AuthHeaderVal = ""
	case "headers":
		if cfg.Headers != nil {
			delete(cfg.Headers, name)
		}
	}
	return nil
}

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

// sqlGuard rejects anything that is not a read-only query against the mirror
// database. Only SELECT, PRAGMA, EXPLAIN, and WITH (CTE) statements are
// allowed; multi-statement input is rejected outright.
func sqlGuard(query string) error {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return usageErr(fmt.Errorf("empty query"))
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "select"),
		strings.HasPrefix(lower, "pragma"),
		strings.HasPrefix(lower, "explain"),
		strings.HasPrefix(lower, "with"):
	default:
		return usageErr(fmt.Errorf("read-only mirror queries only: SELECT, PRAGMA, EXPLAIN, WITH"))
	}
	// sqlite3 does not allow multiple statements in a single Prepare anyway,
	// but reject semicolons that split into a second statement explicitly.
	if parts := strings.Split(trimmed, ";"); len(parts) > 1 {
		if strings.TrimSpace(parts[len(parts)-1]) != "" {
			return usageErr(fmt.Errorf("multiple statements are not allowed"))
		}
	}
	return nil
}

func newSQLCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sql <query>",
		Short: "Run a read-only SQL query against the mirror database",
		Example: `  telegram-cli sql "SELECT COUNT(*) FROM tg_messages"
  telegram-cli sql "SELECT alias, phone FROM tg_accounts" --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if err := sqlGuard(query); err != nil {
				return err
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

			rows, err := s.DB().QueryContext(ctx, query)
			if err != nil {
				return fmt.Errorf("query failed: %w", err)
			}
			defer rows.Close()

			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			var results []map[string]any
			for rows.Next() {
				values := make([]any, len(cols))
				scanTargets := make([]any, len(cols))
				for i := range values {
					scanTargets[i] = &values[i]
				}
				if err := rows.Scan(scanTargets...); err != nil {
					return err
				}
				row := make(map[string]any, len(cols))
				for i, col := range cols {
					row[col] = normalizeSQLValue(values[i])
				}
				results = append(results, row)
			}
			if err := rows.Err(); err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			if len(results) == 0 {
				results = []map[string]any{}
			}
			return outResult(stdout(), f, results)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// normalizeSQLValue converts sqlite driver values into JSON-safe ones
// ([]byte blobs become strings, nil stays null).
func normalizeSQLValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case nil:
		return nil
	case int64:
		return t
	case float64:
		return t
	case bool:
		return t
	case string:
		return t
	case sql.RawBytes:
		return string(t)
	default:
		if b, err := json.Marshal(t); err == nil {
			return json.RawMessage(b)
		}
		return fmt.Sprintf("%v", t)
	}
}

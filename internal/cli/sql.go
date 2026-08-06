package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"telegram-cli/internal/config"
	"telegram-cli/internal/store"

	"github.com/spf13/cobra"
)

// sqlWriteRE matches SQL data-modification keywords as whole words. Applied to
// a WITH-prefixed query (after string literals are stripped) so a data-
// modifying CTE like `WITH x AS (...) INSERT INTO ...` — which the prefix
// allow-list would otherwise admit — is rejected with a clear message instead
// of reaching the database. Read-only CTEs that merely reference a column
// named e.g. "updated" or a LIKE '%delete%' literal never match because
// literal strings are removed first and the match is word-boundary-anchored.
var sqlWriteRE = regexp.MustCompile(`(?i)\b(CREATE|DROP|INSERT|UPDATE|DELETE|REPLACE|ALTER|ATTACH|DETACH|REINDEX|VACUUM|TRUNCATE)\b`)

// stripSQLStrings removes single-quoted string literals from a SQL query so
// keyword detection never false-positives on user data inside literals.
// SQLite string literals may embed a quote as two adjacent single quotes;
// that shape is consumed by the same scan. Not a SQL lexer: it exists only
// to defang the word-boundary check below, and the real write rejection is
// the driver-level mode=ro guard in OpenReadOnlyContext.
func stripSQLStrings(q string) string {
	var sb strings.Builder
	inStr := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\'' {
			if !inStr {
				inStr = true
			} else if i+1 < len(q) && q[i+1] == '\'' {
				// escaped quote inside the literal; skip both.
				sb.WriteByte(' ')
				i++
				continue
			} else {
				inStr = false
			}
			sb.WriteByte(' ')
			continue
		}
		if inStr {
			sb.WriteByte(' ')
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// sqlGuard rejects anything that is not a read-only query against the mirror
// database. Only SELECT, PRAGMA, EXPLAIN, and WITH (CTE) statements are
// allowed; multi-statement input is rejected outright.
//
// The prefix allow-list alone is NOT a write barrier: SQLite supports data-
// modifying CTEs (`WITH x AS (...) INSERT/UPDATE/DELETE/REPLACE ...`), so a
// WITH-prefixed query is additionally scanned for write keywords. The final
// guarantee is the connection itself: newSQLCmd opens the store with
// OpenReadOnlyContext (mode=ro), which rejects any write — direct, CTE-
// wrapped, or PRAGMA — at the driver level. This guard exists so users get a
// clear "read-only" message instead of a raw SQLITE_READONLY error.
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
	// EXPLAIN never executes its statement (it only reports the plan), so the
	// write-keyword scan is skipped for it. WITH, on the other hand, can wrap
	// a data-modifying statement, so it is scanned after literal stripping.
	if strings.HasPrefix(lower, "with") {
		if sqlWriteRE.MatchString(stripSQLStrings(trimmed)) {
			return usageErr(fmt.Errorf("read-only mirror queries only: data-modifying statements are not allowed inside WITH"))
		}
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
			dbPath := config.DefaultDBPath(home)
			// Read-only open: the sql command must never mutate the mirror
			// database. OpenReadOnlyContext (mode=ro) rejects direct AND CTE-
			// wrapped writes at the driver level, which the statement-level
			// sqlGuard prefix check alone cannot guarantee (see sqlGuard).
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return notFoundErr(fmt.Errorf("mirror database not found at %s — run a sync or any telegram command first to create it", dbPath))
			}
			s, err := store.OpenReadOnlyContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("open mirror database (read-only): %w", err)
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
			// Only warn for mirror tables; arbitrary sql may target other tables.
			if strings.Contains(strings.ToLower(query), "tg_messages") {
				warnMirrorEmpty(ctx, cmd, s.DB(), &f)
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

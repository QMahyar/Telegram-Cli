package cli

import (
	"context"
	"database/sql"
	"time"

	"telegram-cli/internal/config"

	"github.com/spf13/cobra"
)

type auditEntry struct {
	ID      int64  `json:"id"`
	At      string `json:"at"`
	Account string `json:"account"`
	Command string `json:"command"`
	Target  string `json:"target"`
	Params  string `json:"params,omitempty"`
	Result  string `json:"result,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func newAuditCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "audit",
		Short:       "Show audit log of CLI operations",
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

			f := parseTelegramFlags(cmd)
			limit := f.Limit
			rows, err := s.DB().QueryContext(ctx,
				`SELECT id, at, account, command, target, params, result, detail FROM tg_audit ORDER BY id DESC LIMIT ?`,
				limit,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			var entries []auditEntry
			for rows.Next() {
				var e auditEntry
				if err := rows.Scan(&e.ID, &e.At, &e.Account, &e.Command, &e.Target, &e.Params, &e.Result, &e.Detail); err != nil {
					return err
				}
				entries = append(entries, e)
			}
			if entries == nil {
				entries = []auditEntry{}
			}
			return outResult(stdout(), f, entries)
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

// writeAuditRecord appends one row to the append-only mutating-op trail.
// Failures are intentionally non-fatal: commands proceed even if the DB write
// cannot be performed.
func writeAuditRecord(ctx context.Context, db *sql.DB, account, command, target, params, result, detail string) {
	_, _ = db.ExecContext(ctx,
		`INSERT INTO tg_audit (at, account, command, target, params, result, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), account, command, target, params, result, detail,
	)
}

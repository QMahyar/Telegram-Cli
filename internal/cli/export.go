package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"telegram-cli/internal/config"
	"telegram-cli/internal/mtproto"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
)

type exportLine struct {
	Account string `json:"account"`
	Chat    string `json:"chat"`
	MsgID   int64  `json:"msg_id"`
	Date    string `json:"date"`
	Sender  string `json:"sender"`
	Text    string `json:"text"`
	Media   string `json:"media_type,omitempty"`
}

func newExportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "export <chat> [out-dir]",
		Short:       "Export chat history to a local JSONL file",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  telegram-cli export @channel ~/exports
  telegram-cli export 7528129992 . --limit 200`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "export chat history")
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			ref := args[0]
			outDir := "export"
			if len(args) > 1 {
				outDir = args[1]
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
			alias, err := resolveAccount(ctx, s, "")
			if err != nil {
				return err
			}
			f := parseTelegramFlags(cmd)
			mgr, err := openManager(home)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			fileName := fmt.Sprintf("export-%s-%s.jsonl", safeFileName(ref), time.Now().UTC().Format("20060102T150405"))
			filePath := filepath.Join(outDir, fileName)
			outFile, err := os.Create(filePath)
			if err != nil {
				return err
			}
			defer outFile.Close()
			writer := bufio.NewWriter(outFile)
			defer writer.Flush()

			count := 0
			err = mgr.DialAndRun(ctx, alias, func(ctx context.Context, client *telegram.Client, api *tg.Client) error {
				resolver := liveResolver(s.DB(), api)
				peer, err := resolver.Resolve(ctx, alias, ref)
				if err != nil {
					return err
				}
				pageSize := f.Limit
				if pageSize <= 0 {
					pageSize = 100
				}
				offsetID := 0
				pages := 0
				for {
					pages++
					if pages > paginatedGetMaxPages {
						fmt.Fprintf(cmd.ErrOrStderr(), "stopped after %d pages (safety limit).\n", paginatedGetMaxPages)
						break
					}
					resp, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
						Peer:     peer,
						OffsetID: offsetID,
						Limit:    pageSize,
					})
					if err != nil {
						return err
					}
					msgs, err := mtproto.HistoryFromResponse(resp)
					if err != nil {
						return err
					}
					if len(msgs) == 0 {
						break
					}
					oldest := msgs[0].MsgID
					for _, m := range msgs {
						if m.MsgID >= oldest {
							oldest = m.MsgID
						}
					}
					_ = oldest
					for _, m := range msgs {
						count++
						if err := json.NewEncoder(writer).Encode(exportLine{
							Account: alias,
							Chat:    ref,
							MsgID:   m.MsgID,
							Date:    time.Unix(m.Date, 0).UTC().Format(time.RFC3339),
							Sender:  m.Sender,
							Text:    m.Text,
							Media:   m.Media,
						}); err != nil {
							return err
						}
					}
					if err := writer.Flush(); err != nil {
						return err
					}
					// Advance the offset past this page. History pages are
					// returned newest-first; the last message of the page is
					// the oldest seen so far.
					offsetID = int(msgs[len(msgs)-1].MsgID)
					if f.Limit > 0 && count >= f.Limit {
						break
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Exported %d messages to %s\n", count, filePath)
			return nil
		},
	}
	addTelegramFlags(cmd)
	return cmd
}

func safeFileName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// Telegram-specific configuration. Lives in its own file so it survives
// regeneration (do not add fields to the emitted Config struct).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"telegram-cli/internal/cliutil"
)

// TelegramDirName is the per-user directory holding sessions, the mirror DB,
// jobs, and audit state.
const TelegramDirName = "telegram-cli"

// EnvAppID / EnvAppHash are the canonical app-credential env vars
// (same names Telethon/Pyrogram/GramJS users already know).
const (
	EnvAppID   = "TELEGRAM_API_ID"
	EnvAppHash = "TELEGRAM_API_HASH"
	EnvHome    = "TELEGRAM_HOME_DIR"
)

// AppCredentials returns the Telegram app credentials (api_id, api_hash)
// from the environment. Both are required for MTProto login.
func AppCredentials() (appID int, appHash string, err error) {
	idStr := os.Getenv(EnvAppID)
	appHash = os.Getenv(EnvAppHash)
	if idStr == "" || appHash == "" {
		return 0, "", fmt.Errorf("missing Telegram app credentials: set %s and %s (create them at https://my.telegram.org/apps)", EnvAppID, EnvAppHash)
	}
	id, convErr := strconv.Atoi(idStr)
	if convErr != nil || id <= 0 {
		return 0, "", fmt.Errorf("%s must be a positive integer (got %q)", EnvAppID, idStr)
	}
	return id, appHash, nil
}

// HomeDir resolves the root data directory for sessions, databases and jobs.
// Explicit override (root --home flag) wins; then TELEGRAM_HOME_DIR; then the
// CLI's shared data directory (which itself honors the --home flag wiring).
func HomeDir(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if env := os.Getenv(EnvHome); env != "" {
		return filepath.Abs(env)
	}
	return cliutil.DataDir()
}

// SessionsDir is the per-account session root: <home>/sessions/<alias>/.
func SessionsDir(home string) string { return filepath.Join(home, "sessions") }

// DefaultDBPath is the unified mirror database: <home>/telegram.db.
func DefaultDBPath(home string) string { return filepath.Join(home, "telegram.db") }

// JobsDBPath shares the mirror DB; kept as a named accessor for clarity.
func JobsDBPath(home string) string { return DefaultDBPath(home) }

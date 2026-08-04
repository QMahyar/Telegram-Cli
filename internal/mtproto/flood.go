package mtproto

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/gotd/td/tgerr"
)

// FloodKind classifies Telegram flood/ban errors.
type FloodKind int

const (
	FloodNone   FloodKind = iota
	FloodWait             // FLOOD_WAIT_N — temporary, retry after N seconds
	FloodPeer             // PEER_FLOOD — permanent cooldown on a specific peer
	FloodBanned           // USER_BANNED / CHANNEL_PRIVATE etc — account-level ban
)

// ClassifyError inspects a gotd/td error and returns (kind, wait duration, scope).
func ClassifyError(err error) (FloodKind, time.Duration, string) {
	if err == nil {
		return FloodNone, 0, ""
	}
	if tgerr.Is(err, "PEER_FLOOD") {
		return FloodPeer, 10 * time.Minute, "send"
	}
	if tgerr.Is(err, "USER_BANNED") || tgerr.Is(err, "USER_DEACTIVATED") || tgerr.Is(err, "CHANNEL_PRIVATE") {
		return FloodBanned, 0, "global"
	}
	// FLOOD_WAIT_N
	// Use tgerr.As (errors.As) instead of a direct type assertion so the error
	// still classifies when the caller wrapped it with fmt.Errorf("%w", ...)
	// — the direct `err.(*tgerr.Error)` assert below silently returned
	// FloodNone for wrapped errors, disabling retry/backoff.
	if rpcErr, ok := tgerr.As(err); ok && rpcErr.IsType("FLOOD_WAIT") {
		n := rpcErr.Argument
		if n <= 0 {
			n = 30
		}
		return FloodWait, time.Duration(n) * time.Second, "global"
	}
	return FloodNone, 0, ""
}

// CooldownManager reads and writes per-account flood cooldowns in SQLite.
type CooldownManager struct {
	db *sql.DB
}

// NewCooldownManager wraps a *sql.DB (the same mirror DB).
func NewCooldownManager(db *sql.DB) *CooldownManager {
	return &CooldownManager{db: db}
}

// Record stores a cooldown after a flood error.
func (cm *CooldownManager) Record(ctx context.Context, account, scope string, kind FloodKind, until time.Time, seconds int) error {
	kindStr := "FLOOD_WAIT"
	switch kind {
	case FloodPeer:
		kindStr = "PEER_FLOOD"
	case FloodBanned:
		kindStr = "BANNED"
	}
	if scope == "" {
		scope = "global"
	}
	_, err := cm.db.ExecContext(ctx,
		`INSERT INTO tg_cooldowns (account, scope, kind, until_unix, seconds)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (account, scope, kind) DO UPDATE SET
		   until_unix = excluded.until_unix, seconds = excluded.seconds, updated_at = datetime('now')`,
		account, scope, kindStr, until.Unix(), seconds,
	)
	return err
}

// Until returns the earliest cooldown expiry for the given account+scope.
// Returns zero time if there is no active cooldown.
func (cm *CooldownManager) Until(ctx context.Context, account, scope string) time.Time {
	if scope == "" {
		scope = "global"
	}
	var until int64
	err := cm.db.QueryRowContext(ctx,
		`SELECT until_unix FROM tg_cooldowns WHERE account = ? AND scope = ? AND until_unix > ? ORDER BY until_unix LIMIT 1`,
		account, scope, time.Now().Unix(),
	).Scan(&until)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(until, 0)
}

// IsBanned returns true if the account has a non-expired ban record.
func (cm *CooldownManager) IsBanned(ctx context.Context, account string) bool {
	var count int
	cm.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tg_cooldowns WHERE account = ? AND kind = 'BANNED' AND until_unix > ?`,
		account, time.Now().Unix(),
	).Scan(&count)
	return count > 0
}

// RetryWithBackoff executes fn, retrying on FLOOD_WAIT errors with
// exponential backoff. Returns after maxRetries or on non-flood errors.
func RetryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		kind, wait, _ := ClassifyError(lastErr)
		if kind == FloodNone || kind == FloodBanned {
			return lastErr
		}
		if kind == FloodPeer {
			return lastErr // don't retry peer floods — they need longer cooldowns
		}
		// FloodWait: respect the server-requested wait
		backoff := time.Duration(float64(wait) * math.Pow(1.5, float64(attempt)))
		if backoff > 10*time.Minute {
			backoff = 10 * time.Minute
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return fmt.Errorf("exhausted %d retries: %w", maxRetries, lastErr)
}

// FloodWaitShortcut sleeps for the FLOOD_WAIT duration if the error is a flood wait.
// Returns true if it waited (caller should retry).
func FloodWaitShortcut(ctx context.Context, err error) (bool, error) {
	kind, wait, _ := ClassifyError(err)
	switch kind {
	case FloodNone:
		return false, nil
	case FloodBanned:
		return false, fmt.Errorf("account banned: %w", err)
	case FloodPeer:
		return false, fmt.Errorf("peer flood: %w", err)
	case FloodWait:
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(wait):
			return true, nil
		}
	}
	return false, nil
}

// Package mtproto provides the multi-account Telegram MTProto client
// wrapping gotd/td. It owns session lifecycle, peer resolution, flood
// backoff and the one-shot client execution model used by every CLI command.
package mtproto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// Account holds the metadata for one logged-in Telegram account.
type Account struct {
	Alias      string `json:"alias"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	Phone      string `json:"phone"`
	DCID       int    `json:"dc_id"`
	SessionDir string `json:"session_dir"`
	Status     string `json:"status"` // active | locked
}

// Manager manages the set of logged-in accounts and provides
// one-shot client execution (DialAndRun).
type Manager struct {
	Home     string // root data dir (<home>/sessions/<alias>/)
	APIID    int
	APIHash  string
}

// NewManager creates a manager. Call config.AppCredentials() to get apiID/apiHash.
func NewManager(home string) (*Manager, error) {
	id, hash, err := appCredentials()
	if err != nil {
		return nil, err
	}
	return &Manager{Home: home, APIID: id, APIHash: hash}, nil
}

func appCredentials() (int, string, error) {
	// Load .env file from working directory or parents
	cwd, _ := os.Getwd()
	loadDotEnv(cwd)

	idStr := os.Getenv("TELEGRAM_API_ID")
	hash := os.Getenv("TELEGRAM_API_HASH")
	if idStr == "" || hash == "" {
		return 0, "", fmt.Errorf("set TELEGRAM_API_ID and TELEGRAM_API_HASH via env vars or .env file (https://my.telegram.org/apps)")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return 0, "", fmt.Errorf("TELEGRAM_API_ID must be a positive integer")
	}
	return id, hash, nil
}

// SessionDir returns the session storage directory for the given account alias.
func (m *Manager) SessionDir(alias string) string {
	return filepath.Join(m.Home, "sessions", alias)
}

// clientOpts returns the common telegram.Options for one-shot execution.
func (m *Manager) clientOpts(sessionDir string) telegram.Options {
	return telegram.Options{
		Device:   telegram.DeviceTDesktopWindows(),
		Resolver: telegram.TDesktopResolver(),
		SessionStorage: &session.FileStorage{Path: filepath.Join(sessionDir, "session.json")},
		// NoUpdates: true — we don't need persistent update handling
		// for one-shot CLI commands. QR login will need this turned on.
		NoUpdates: true,
	}
}

// ClientFunc is the callback executed inside a one-shot gotd client.Run.
type ClientFunc func(ctx context.Context, client *telegram.Client, api *tg.Client) error

// DialAndRun opens a gotd client for the given account, runs the callback,
// and closes the client. This is the primary execution model for all commands.
func (m *Manager) DialAndRun(ctx context.Context, alias string, fn ClientFunc) error {
	dir := m.SessionDir(alias)
	if _, err := os.Stat(filepath.Join(dir, "session.json")); os.IsNotExist(err) {
		return fmt.Errorf("account %q has no session — run: telegram-cli accounts add %s", alias, alias)
	}
	return m.dial(ctx, dir, fn)
}

// DialAndRunUnchecked is like DialAndRun but does not verify the session file
// exists (used during login when the session doesn't exist yet).
func (m *Manager) DialAndRunUnchecked(ctx context.Context, alias string, fn ClientFunc) error {
	dir := m.SessionDir(alias)
	return m.dial(ctx, dir, fn)
}

func (m *Manager) dial(ctx context.Context, sessionDir string, fn ClientFunc) error {
	opts := m.clientOpts(sessionDir)
	client := telegram.NewClient(m.APIID, m.APIHash, opts)
	return client.Run(ctx, func(ctx context.Context) error {
		return fn(ctx, client, client.API())
	})
}

// Status checks whether the session is authorized and returns user info.
func Status(ctx context.Context, client *telegram.Client) (*auth.Status, error) {
	return client.Auth().Status(ctx)
}

// Logout terminates the session and removes local data.
func Logout(ctx context.Context, client *telegram.Client) error {
	_, err := client.API().AuthLogOut(ctx)
	return err
}

// EnsureDir creates the session directory for an account alias.
func (m *Manager) EnsureDir(alias string) (string, error) {
	dir := m.SessionDir(alias)
	return dir, os.MkdirAll(dir, 0o700)
}

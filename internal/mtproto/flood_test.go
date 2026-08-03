package mtproto

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantKind  FloodKind
		wantMin   time.Duration
		wantMax   time.Duration
		wantScope string
	}{
		{
			name:      "nil error",
			err:       nil,
			wantKind:  FloodNone,
			wantMin:   0,
			wantMax:   0,
			wantScope: "",
		},
		{
			name:      "PEER_FLOOD",
			err:       tgerr.New(400, "PEER_FLOOD"),
			wantKind:  FloodPeer,
			wantMin:   10 * time.Minute,
			wantMax:   10 * time.Minute,
			wantScope: "send",
		},
		{
			name:      "USER_BANNED",
			err:       tgerr.New(400, "USER_BANNED"),
			wantKind:  FloodBanned,
			wantMin:   0,
			wantMax:   0,
			wantScope: "global",
		},
		{
			name:      "USER_DEACTIVATED",
			err:       tgerr.New(400, "USER_DEACTIVATED"),
			wantKind:  FloodBanned,
			wantMin:   0,
			wantMax:   0,
			wantScope: "global",
		},
		{
			name:      "CHANNEL_PRIVATE",
			err:       tgerr.New(400, "CHANNEL_PRIVATE"),
			wantKind:  FloodBanned,
			wantMin:   0,
			wantMax:   0,
			wantScope: "global",
		},
		{
			name:      "FLOOD_WAIT_30",
			err:       tgerr.New(420, "FLOOD_WAIT_30"),
			wantKind:  FloodWait,
			wantMin:   30 * time.Second,
			wantMax:   30 * time.Second,
			wantScope: "global",
		},
		{
			name:      "FLOOD_WAIT_60",
			err:       tgerr.New(420, "FLOOD_WAIT_60"),
			wantKind:  FloodWait,
			wantMin:   60 * time.Second,
			wantMax:   60 * time.Second,
			wantScope: "global",
		},
		{
			name:      "FLOOD_WAIT with no number",
			err:       tgerr.New(420, "FLOOD_WAIT"),
			wantKind:  FloodWait,
			wantMin:   30 * time.Second,
			wantMax:   30 * time.Second,
			wantScope: "global",
		},
		{
			name:      "unrelated error",
			err:       tgerr.New(400, "BAD_REQUEST"),
			wantKind:  FloodNone,
			wantMin:   0,
			wantMax:   0,
			wantScope: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotWait, gotScope := ClassifyError(tt.err)
			if gotKind != tt.wantKind {
				t.Errorf("ClassifyError() kind = %v, want %v", gotKind, tt.wantKind)
			}
			if gotWait < tt.wantMin || gotWait > tt.wantMax {
				t.Errorf("ClassifyError() wait = %v, want [%v, %v]", gotWait, tt.wantMin, tt.wantMax)
			}
			if gotScope != tt.wantScope {
				t.Errorf("ClassifyError() scope = %v, want %v", gotScope, tt.wantScope)
			}
		})
	}
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("succeeds immediately", func(t *testing.T) {
		calls := 0
		err := RetryWithBackoff(context.Background(), 3, func() error {
			calls++
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("retries on FLOOD_WAIT", func(t *testing.T) {
		calls := 0
		err := RetryWithBackoff(context.Background(), 2, func() error {
			calls++
			if calls < 3 {
				return tgerr.New(420, "FLOOD_WAIT_1")
			}
			return nil
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("does not retry PEER_FLOOD", func(t *testing.T) {
		calls := 0
		err := RetryWithBackoff(context.Background(), 3, func() error {
			calls++
			return tgerr.New(400, "PEER_FLOOD")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("does not retry USER_BANNED", func(t *testing.T) {
		calls := 0
		err := RetryWithBackoff(context.Background(), 3, func() error {
			calls++
			return tgerr.New(400, "USER_BANNED")
		})
		if err == nil {
			t.Error("expected error, got nil")
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		calls := 0
		err := RetryWithBackoff(ctx, 100, func() error {
			calls++
			return tgerr.New(420, "FLOOD_WAIT_10")
		})
		if err != context.DeadlineExceeded {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
	})
}

func TestFloodWaitShortcut(t *testing.T) {
	t.Run("no flood", func(t *testing.T) {
		waited, err := FloodWaitShortcut(context.Background(), nil)
		if waited {
			t.Error("expected not waited")
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("FLOOD_WAIT returns waited=true", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		waited, err := FloodWaitShortcut(ctx, tgerr.New(420, "FLOOD_WAIT_1"))
		if !waited {
			t.Error("expected waited=true")
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("PEER_FLOOD returns error", func(t *testing.T) {
		waited, err := FloodWaitShortcut(context.Background(), tgerr.New(400, "PEER_FLOOD"))
		if waited {
			t.Error("expected not waited")
		}
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("USER_BANNED returns error", func(t *testing.T) {
		waited, err := FloodWaitShortcut(context.Background(), tgerr.New(400, "USER_BANNED"))
		if waited {
			t.Error("expected not waited")
		}
		if err == nil {
			t.Error("expected error")
		}
	})
}

// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"telegram-cli/internal/cli"
	"telegram-cli/internal/cliutil"
	mcptools "telegram-cli/internal/mcp"
)

// Transport selection order: --transport flag, then TELEGRAM_MCP_TRANSPORT env,
// then the first transport declared in the spec (see MCPConfig.Transport).
// The flag surface lets one binary serve stdio locally and streamable HTTP
// when hosted in a container or remote sandbox, matching the Anthropic
// guidance that production agents need a remote option.

const (
	defaultHTTPAddr = ":7777"
)

// version is the MCP server's version, overridable at build time via ldflags.
var version = "0.0.0-dev"

func main() {
	// Move a pre-0.1.5 XDG-scattered install into ~/.telegram-cli before
	// any path resolution; no-op unless platform defaults are in play.
	if err := cliutil.MigrateLegacyLayout(); err != nil {
		fmt.Fprintf(os.Stderr, "telegram-cli data migration failed: %v\n", err)
		os.Exit(1)
	}
	// Pin the learn-event surface for this process and every walker
	// shell-out child, so usage events record surface=mcp.
	_ = os.Setenv("TELEGRAM_LEARN_SURFACE", "mcp")
	if err := cli.BindMCPServerProfile(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP client-profile bind failed: %v\n", err)
		os.Exit(1)
	}
	s := server.NewMCPServer(
		"Telegram",
		version,
		server.WithToolCapabilities(false),
	)

	mcptools.RegisterTools(s)

	transport := flag.String("transport", defaultTransport(), "MCP transport: stdio | http")
	addr := flag.String("addr", defaultHTTPAddr, "bind address for http transport (host:port or :port)")
	apiKey := flag.String("api-key", "", "API key for HTTP bearer token authentication (required for http transport)")
	flag.Parse()

	if *apiKey == "" {
		*apiKey = os.Getenv("TELEGRAM_MCP_API_KEY")
	}

	switch strings.ToLower(*transport) {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
	case "http":
		httpSrv := server.NewStreamableHTTPServer(s)

		var handler http.Handler = httpSrv

		if *apiKey != "" {
			handler = authMiddleware(handler, *apiKey)
			handler = rateLimitMiddleware(handler)
			fmt.Fprintf(os.Stderr, "telegram-mcp serving MCP over streamable HTTP at %s (authenticated)\n", *addr)
		} else {
			fmt.Fprintf(os.Stderr, "telegram-mcp serving MCP over streamable HTTP at %s (WARNING: no authentication)\n", *addr)
			fmt.Fprintf(os.Stderr, "hint: set --api-key or TELEGRAM_MCP_API_KEY for production use\n")
		}

		if err := http.ListenAndServe(*addr, handler); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown --transport %q (supported: stdio, http)\n", *transport)
		os.Exit(2)
	}
}

// defaultTransport reads TELEGRAM_MCP_TRANSPORT env when set, otherwise falls back
// to "stdio" so running the binary with no args keeps today's behavior.
// Container-hosted agents can pin the transport via env without a flag, which
// matches how hosted-agent process supervisors typically pass configuration.
func defaultTransport() string {
	if t := os.Getenv("TELEGRAM_MCP_TRANSPORT"); t != "" {
		return t
	}
	return "stdio"
}

// authMiddleware validates Bearer token authentication.
func authMiddleware(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			http.Error(w, "invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, prefix)
		if token != apiKey {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware implements a simple token bucket rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	maxTokens float64
	refillRate float64
	lastRefill time.Time
}

func newRateLimiter(maxTokens, refillRate float64) *rateLimiter {
	return &rateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens = min(rl.maxTokens, rl.tokens+elapsed*rl.refillRate)
	rl.lastRefill = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// rateLimitMiddleware adds rate limiting to HTTP requests.
func rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := newRateLimiter(10, 1) // 10 requests burst, 1 per second sustained
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

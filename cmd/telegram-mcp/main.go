// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

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
	flag.Parse()

	switch strings.ToLower(*transport) {
	case "stdio":
		if err := server.ServeStdio(s); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
	case "http":
		httpSrv := server.NewStreamableHTTPServer(s)
		fmt.Fprintf(os.Stderr, "telegram-mcp serving MCP over streamable HTTP at %s\n", *addr)
		if err := httpSrv.Start(*addr); err != nil {
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

// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelInboxHelpWires smoke-tests that the inbox command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelInboxHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"inbox", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("inbox --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "inbox"} {
		if !strings.Contains(help, want) {
			t.Fatalf("inbox --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestInboxCommand_MissingAccount(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"inbox"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	// inbox without account uses last-used account (not an error if accounts exist)
	if err != nil {
		t.Errorf("inbox without account should use last-used, got error: %v", err)
	}
}

func TestInboxCommand_InvalidFlag(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"inbox", "--nonexistent-flag"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid flag")
	}
}

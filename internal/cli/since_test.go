// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelSinceHelpWires smoke-tests that the since command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelSinceHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"since", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("since --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "since"} {
		if !strings.Contains(help, want) {
			t.Fatalf("since --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestSinceCommand_MissingAccount(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"since"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	// since without time-spec shows help (not an error)
	if err != nil {
		t.Errorf("since without time-spec should show help, got error: %v", err)
	}
}

func TestSinceCommand_InvalidFlag(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"since", "--nonexistent-flag"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid flag")
	}
}

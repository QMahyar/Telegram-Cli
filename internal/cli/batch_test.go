// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelBatchHelpWires smoke-tests that the batch command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelBatchHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"batch", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("batch --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "batch"} {
		if !strings.Contains(help, want) {
			t.Fatalf("batch --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestBatchCommand_MissingAccount(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"batch", "forward"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	// batch forward shows help when called without args (not an error)
	if err != nil {
		t.Errorf("batch forward without args should show help, got error: %v", err)
	}
}

func TestBatchCommand_InvalidFlag(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"batch", "forward", "--nonexistent-flag"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid flag")
	}
}

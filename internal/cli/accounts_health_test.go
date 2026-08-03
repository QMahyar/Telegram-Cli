// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNovelAccountsHealthHelpWires smoke-tests that the accounts health command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAccountsHealthHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"accounts", "health", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("accounts health --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "health"} {
		if !strings.Contains(help, want) {
			t.Fatalf("accounts health --help missing %q in output:\n%s", want, help)
		}
	}
}

package main

import (
	"os"
	"testing"
)

func runWithArgs(args ...string) int {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = append([]string{"dothuntcli"}, args...)
	return run()
}

// Keep these exit codes stable: they matter in scripts/agents.
func TestRun_NoArgs_Exit2(t *testing.T) {
	if got := runWithArgs(); got != 2 {
		t.Fatalf("exit=%d, want 2", got)
	}
}

func TestRun_UnknownCommand_Exit2(t *testing.T) {
	if got := runWithArgs("nope"); got != 2 {
		t.Fatalf("exit=%d, want 2", got)
	}
}

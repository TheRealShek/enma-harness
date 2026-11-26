package main

import (
	"testing"

	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

func TestCLISessionLazyPersistenceAndCommands(t *testing.T) {
	store := &threads.Store{BaseDir: t.TempDir()}
	runner := NewRunner(store, "/proj/x", &tools.Session{ProjectRoot: "/proj/x"})
	cli := &cliSession{runner: runner}

	// Slash command in empty state should not create any thread on disk.
	if !cli.handleCommand("/rename my-thread") {
		t.Fatal("expected /rename to be handled as slash command")
	}

	list, err := store.List("/proj/x")
	if err != nil || len(list) != 0 {
		t.Fatalf("expected 0 saved threads before first prompt, got %d (err=%v)", len(list), err)
	}

	// First real prompt creates thread lazily with pending name and saves events.
	runner.createThreadIfNeeded("ignored auto name")
	runner.appendUserMessage("hello")
	runner.toolSession.Tainted = true
	runner.persistTaintState()

	if runner.activeThread == nil || runner.activeThread.Name != "my-thread" {
		t.Fatalf("expected active thread named my-thread, got %+v", runner.activeThread)
	}

	// /new resets state and clears taint; /resume restores both messages and taint.
	cli.handleCommand("/new")

	if runner.activeThread != nil || runner.toolSession.Tainted {
		t.Fatalf("expected /new to reset active thread and taint")
	}

	cli.handleCommand("/resume my-thread")

	if runner.activeThread == nil || !runner.toolSession.Tainted || len(runner.conversation) != 2 {
		t.Fatalf("expected resumed tainted thread with 2 messages, got active=%v tainted=%v len=%d",
			runner.activeThread != nil, runner.toolSession.Tainted, len(runner.conversation))
	}
}

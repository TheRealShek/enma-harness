package main

import (
	"testing"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

func TestCLISessionLazyPersistenceAndCommands(t *testing.T) {
	store := &threads.Store{BaseDir: t.TempDir()}
	cli := &cliSession{
		store:        store,
		projectRoot:  "/proj/x",
		conversation: []Message{{Role: "system", Content: config.DefaultSystemPrompt}},
		toolSession:  &tools.Session{ProjectRoot: "/proj/x"},
	}

	// Slash command in empty state should not create any thread on disk.
	if !cli.handleCommand("/rename my-thread") {
		t.Fatal("expected /rename to be handled as slash command")
	}

	list, err := store.List("/proj/x")
	if err != nil || len(list) != 0 {
		t.Fatalf("expected 0 saved threads before first prompt, got %d (err=%v)", len(list), err)
	}

	// First real prompt creates thread lazily with pending name and saves messages.
	cli.createThreadIfNeeded("ignored auto name")
	cli.appendMessage(Message{Role: "user", Content: "hello"})
	cli.toolSession.Tainted = true
	cli.persistTaintState()

	if cli.activeThread == nil || cli.activeThread.Name != "my-thread" {
		t.Fatalf("expected active thread named my-thread, got %+v", cli.activeThread)
	}

	// /new resets state and clears taint; /resume restores both messages and taint.
	cli.handleCommand("/new")

	if cli.activeThread != nil || cli.toolSession.Tainted {
		t.Fatalf("expected /new to reset active thread and taint")
	}

	cli.handleCommand("/resume my-thread")

	if cli.activeThread == nil || !cli.toolSession.Tainted || len(cli.conversation) != 2 {
		t.Fatalf("expected resumed tainted thread with 2 messages, got active=%v tainted=%v len=%d",
			cli.activeThread != nil, cli.toolSession.Tainted, len(cli.conversation))
	}
}

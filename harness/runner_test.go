package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealshek/enma-harness/llm"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

func TestRunnerEventsFoldAndApproval(t *testing.T) {
	root := t.TempDir()
	store := &threads.Store{BaseDir: t.TempDir()}

	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	toolSession := &tools.Session{
		ProjectRoot: root,
		Approve: func(action, target string, tainted bool) bool {
			return false
		},
	}

	r := NewRunner(store, root, toolSession)

	callNum := 0
	r.chatFn = func(ctx context.Context, messages []llm.Message, onToken func(string, bool)) (llm.Message, llm.Usage, error) {
		callNum++
		switch callNum {
		case 1:
			return llm.Message{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{{
					ID:       "tc_1",
					Type:     "function",
					Function: llm.Function{Name: "write_file", Arguments: `{"path":".mcp.json","content":"{}"}`},
				}},
			}, llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}, nil
		default:
			return llm.Message{Role: "assistant", Content: "done"}, llm.Usage{PromptTokens: 20, CompletionTokens: 3, TotalTokens: 23}, nil
		}
	}

	if err := r.RunPrompt(context.Background(), "try writing mcp", TurnHooks{}); err != nil {
		t.Fatalf("RunPrompt failed: %v", err)
	}

	meta, events, err := store.Load(r.ActiveThread.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	wantTypes := []string{
		threads.EventThreadStarted,
		threads.EventUserMessage,
		threads.EventLLMResponse,
		threads.EventToolApproval,
		threads.EventToolResult,
		threads.EventLLMResponse,
	}

	if len(events) != len(wantTypes) {
		t.Fatalf("len(events) = %d, want %d (%+v)", len(events), len(wantTypes), events)
	}

	for i, want := range wantTypes {
		if events[i].Type != want || events[i].Sequence != int64(i+1) {
			t.Errorf("event[%d] = (%d, %s), want (%d, %s)", i, events[i].Sequence, events[i].Type, i+1, want)
		}
	}

	var approval toolApprovalData
	if err := json.Unmarshal(events[3].Data, &approval); err != nil {
		t.Fatal(err)
	}

	if approval.Approved || approval.ToolCallID != "tc_1" {
		t.Errorf("approval = %+v, want Approved=false ToolCallID=tc_1", approval)
	}

	// Resuming into a fresh Runner must reconstruct the exact same conversation.
	resumed := NewRunner(store, root, &tools.Session{ProjectRoot: root})
	if _, err := resumed.LoadThread(meta.ID); err != nil {
		t.Fatalf("LoadThread failed: %v", err)
	}

	if len(resumed.conversation) != len(r.conversation) || resumed.eventSeq != 6 {
		t.Fatalf("resumed conversation len=%d seq=%d, want len=%d seq=6",
			len(resumed.conversation), resumed.eventSeq, len(r.conversation))
	}
}

func TestRunnerLogsLLMErrorAndRespectsMaxTurns(t *testing.T) {
	store := &threads.Store{BaseDir: t.TempDir()}
	r := NewRunner(store, t.TempDir(), nil)
	r.chatFn = func(context.Context, []llm.Message, func(string, bool)) (llm.Message, llm.Usage, error) {
		return llm.Message{}, llm.Usage{}, errors.New("backend down")
	}

	if err := r.RunPrompt(context.Background(), "hi", TurnHooks{}); err == nil {
		t.Fatal("expected error from RunPrompt")
	}

	_, events, err := store.Load(r.ActiveThread.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 3 || events[2].Type != threads.EventLLMError {
		t.Fatalf("expected llm.error as 3rd event, got %+v", events)
	}

	// Verify maxTurns stops infinite tool loops.
	r.Reset("")
	r.MaxTurns = 1
	r.chatFn = func(context.Context, []llm.Message, func(string, bool)) (llm.Message, llm.Usage, error) {
		return llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:       "tc_loop",
				Type:     "function",
				Function: llm.Function{Name: "list_files", Arguments: `{"path":"."}`},
			}},
		}, llm.Usage{}, nil
	}

	if err := r.RunPrompt(context.Background(), "loop", TurnHooks{}); err == nil {
		t.Fatal("expected maxTurns error")
	}
}

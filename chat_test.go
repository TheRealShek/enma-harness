package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withMockTransport(t *testing.T, fn roundTripFunc) {
	t.Helper()

	orig := http.DefaultTransport
	http.DefaultTransport = fn

	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}

func sseResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestChatThinkTagParsing(t *testing.T) {
	stream := strings.Join([]string{
		": keep-alive",
		"",
		`data: {"choices":[{"delta":{"content":"Hi <thi"}}]}`,
		`data: {"choices":[{"delta":{"content":"nk>plan"}}]}`,
		`data: {"choices":[{"delta":{"content":"ning</think> done"}}]}`,
		`data: {"choices":[{"delta":{"content":" stray</think>end"}}]}`,
		"data: [DONE]",
		"",
	}, "\n")

	withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return sseResponse(stream), nil
	})

	type tokenEvent struct {
		text       string
		isThinking bool
	}

	var events []tokenEvent

	reply, _, err := chat([]Message{{Role: "user", Content: "hello"}}, func(tok string, thinking bool) {
		events = append(events, tokenEvent{text: tok, isThinking: thinking})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "<thi" + "nk>" splits the tag across chunks, so "<think>plan" stays in Content while
	// the chunk-local "</think>" match routes "ning" and " stray" to Reasoning.
	wantContent := "Hi <think>plan doneend"
	if reply.Content != wantContent {
		t.Errorf("Content = %q, want %q", reply.Content, wantContent)
	}

	wantReasoning := "ning stray"
	if reply.Reasoning != wantReasoning {
		t.Errorf("Reasoning = %q, want %q", reply.Reasoning, wantReasoning)
	}

	if len(events) == 0 {
		t.Fatal("expected onToken events, got none")
	}
}

func TestChatThinkTagAcrossWholeChunks(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"pre<think>step 1"}}]}`,
		`data: {"choices":[{"delta":{"content":" step 2</think>post"}}]}`,
		"data: [DONE]",
	}, "\n")

	withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return sseResponse(stream), nil
	})

	var contentTokens, thinkingTokens []string

	reply, _, err := chat([]Message{{Role: "user", Content: "hi"}}, func(tok string, thinking bool) {
		if thinking {
			thinkingTokens = append(thinkingTokens, tok)
		} else {
			contentTokens = append(contentTokens, tok)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply.Content != "prepost" {
		t.Errorf("Content = %q, want %q", reply.Content, "prepost")
	}

	if reply.Reasoning != "step 1 step 2" {
		t.Errorf("Reasoning = %q, want %q", reply.Reasoning, "step 1 step 2")
	}

	if got := strings.Join(contentTokens, ""); got != "prepost" {
		t.Errorf("content tokens = %q, want %q", got, "prepost")
	}

	if got := strings.Join(thinkingTokens, ""); got != "step 1 step 2" {
		t.Errorf("thinking tokens = %q, want %q", got, "step 1 step 2")
	}
}

func TestChatReasoningAndToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		"not-data: ignored",
		"data: {invalid json}",
		`data: {"choices":[{"delta":{"reasoning":"r1 "}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"r2"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read_"}},{"index":1,"id":"call_2","type":"function","function":{"name":"list_files"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"{\"path\":\"a.go\"}"}},{"index":1,"function":{"arguments":"{}"}}]}}]}`,
		"data: [DONE]",
		`data: {"choices":[{"delta":{"content":"after done"}}]}`,
	}, "\n")

	withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return sseResponse(stream), nil
	})

	reply, _, err := chat([]Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply.Reasoning != "r1 r2" {
		t.Errorf("Reasoning = %q, want %q", reply.Reasoning, "r1 r2")
	}

	if reply.Content != "" {
		t.Errorf("Content = %q, want empty (data after [DONE] must be ignored)", reply.Content)
	}

	if len(reply.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %d, want 2", len(reply.ToolCalls))
	}

	if reply.ToolCalls[0].ID != "call_1" || reply.ToolCalls[0].Type != "function" ||
		reply.ToolCalls[0].Function.Name != "read_file" ||
		reply.ToolCalls[0].Function.Arguments != `{"path":"a.go"}` {
		t.Errorf("ToolCalls[0] = %+v, unexpected", reply.ToolCalls[0])
	}

	if reply.ToolCalls[1].ID != "call_2" || reply.ToolCalls[1].Function.Name != "list_files" ||
		reply.ToolCalls[1].Function.Arguments != "{}" {
		t.Errorf("ToolCalls[1] = %+v, unexpected", reply.ToolCalls[1])
	}
}

func TestChatUsageAndFallback(t *testing.T) {
	t.Run("uses explicit usage from stream", func(t *testing.T) {
		stream := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"ok"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			"data: [DONE]",
		}, "\n")

		withMockTransport(t, func(req *http.Request) (*http.Response, error) {
			return sseResponse(stream), nil
		})

		_, usage, err := chat([]Message{{Role: "user", Content: "hello"}}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if usage.PromptTokens != 10 || usage.CompletionTokens != 5 || usage.TotalTokens != 15 {
			t.Errorf("usage = %+v, want {10, 5, 15}", usage)
		}
	})

	t.Run("estimates usage when omitted", func(t *testing.T) {
		stream := strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning":"12345","content":"abcd"}}]}`,
			"data: [DONE]",
		}, "\n")

		withMockTransport(t, func(req *http.Request) (*http.Response, error) {
			return sseResponse(stream), nil
		})

		_, usage, err := chat([]Message{{Role: "user", Content: "12345678"}}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Prompt: (8+3)/4 = 2. Completion: (4+3)/4 + (5+3)/4 = 1 + 2 = 3. Total = 5.
		if usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
			t.Errorf("usage = %+v, want Prompt=2 Completion=3 Total=5", usage)
		}
	})
}

func TestChatHTTPError(t *testing.T) {
	withMockTransport(t, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("upstream down")),
			Header:     make(http.Header),
		}, nil
	})

	_, _, err := chat([]Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error for 502 response, got nil")
	}

	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream down") {
		t.Errorf("unexpected error message: %v", err)
	}
}

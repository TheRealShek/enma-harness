package cli

import (
	"testing"
	"time"

	"github.com/therealshek/enma-harness/llm"
)

func TestStreamRendererHiddenThinking(t *testing.T) {
	r := &streamRenderer{showThinking: false, startedAt: time.Now().Add(-5 * time.Second)}

	r.onToken("internal thought", true)

	if !r.startedThinking || r.thinkingEnded {
		t.Fatalf("unexpected thinking state: started=%v ended=%v", r.startedThinking, r.thinkingEnded)
	}

	r.onToken("hello", false)

	if !r.thinkingEnded || !r.startedAssistant {
		t.Fatalf("expected thinking to end and assistant to start: ended=%v assistant=%v", r.thinkingEnded, r.startedAssistant)
	}

	r.finish(llm.Message{Content: "hello"}, llm.Usage{}, nil)
}

func TestFormatThoughtDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{400 * time.Millisecond, "Thought for <1s"},
		{12 * time.Second, "Thought for 12s"},
		{60 * time.Second, "Thought for 1m"},
		{75 * time.Second, "Thought for 1m 15s"},
	}

	for _, tc := range cases {
		if got := formatThoughtDuration(tc.in); got != tc.want {
			t.Errorf("formatThoughtDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

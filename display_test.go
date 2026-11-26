package main

import "testing"

func TestStreamRendererHiddenThinking(t *testing.T) {
	r := &streamRenderer{showThinking: false}

	r.onToken("internal thought", true)

	if !r.startedThinking || r.thinkingEnded {
		t.Fatalf("unexpected thinking state: started=%v ended=%v", r.startedThinking, r.thinkingEnded)
	}

	r.onToken("hello", false)

	if !r.thinkingEnded || !r.startedAssistant {
		t.Fatalf("expected thinking to end and assistant to start: ended=%v assistant=%v", r.thinkingEnded, r.startedAssistant)
	}

	r.finish(Message{Content: "hello"}, Usage{}, nil)
}

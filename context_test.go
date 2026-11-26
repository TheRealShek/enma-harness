package main

import (
	"testing"

	"github.com/therealshek/enma-harness/config"
)

func TestFormatWithCommas(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1000000, "1,000,000"},
	}

	for _, tc := range cases {
		if got := formatWithCommas(tc.in); got != tc.want {
			t.Errorf("formatWithCommas(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCountToolTokens(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user message"},
		{Role: "tool", Content: "1234"},
		{Role: "assistant", Content: "assistant reply"},
		{Role: "tool", Content: "12345"},
	}

	// Tool messages: (4+3)/4 = 1 and (5+3)/4 = 2.
	if got := countToolTokens(messages); got != 3 {
		t.Errorf("countToolTokens = %d, want 3", got)
	}

	if got := countToolTokens(nil); got != 0 {
		t.Errorf("countToolTokens(nil) = %d, want 0", got)
	}
}

func TestCountReasoningTokens(t *testing.T) {
	cases := []struct {
		name  string
		usage Usage
		reply Message
		want  int
	}{
		{
			name: "prefers explicit usage details",
			usage: Usage{
				CompletionTokensDetails: CompletionTokensDetails{ReasoningTokens: 42},
			},
			reply: Message{Reasoning: "12345678", Content: "<think>1234</think>"},
			want:  42,
		},
		{
			name:  "falls back to reply.Reasoning",
			reply: Message{Reasoning: "12345", Content: "<think>1234</think>"},
			want:  2,
		},
		{
			name:  "falls back to think tags in Content",
			reply: Message{Content: "before<think>12345678</think>after"},
			want:  2,
		},
		{
			name:  "ignores unclosed or inverted think tags",
			reply: Message{Content: "</think>abc<think>"},
			want:  0,
		},
		{
			name:  "returns zero when absent",
			reply: Message{Content: "plain answer"},
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countReasoningTokens(tc.usage, tc.reply); got != tc.want {
				t.Errorf("countReasoningTokens = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCalculateContextMetrics(t *testing.T) {
	usage := Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      4096,
	}
	conversation := []Message{
		{Role: "user", Content: "hi"},
		{Role: "tool", Content: "12345678"},
	}
	reply := Message{Reasoning: "abcd"}

	metrics := calculateContextMetrics(usage, conversation, reply)

	if metrics.TotalTokens != 4096 {
		t.Errorf("TotalTokens = %d, want 4096", metrics.TotalTokens)
	}

	if metrics.MaxTokens != config.MaxContextTokens {
		t.Errorf("MaxTokens = %d, want %d", metrics.MaxTokens, config.MaxContextTokens)
	}

	if metrics.PercentUsed != 50 {
		t.Errorf("PercentUsed = %v, want 50", metrics.PercentUsed)
	}

	if metrics.InputTokens != 1000 || metrics.OutputTokens != 500 {
		t.Errorf("Input/Output = %d/%d, want 1000/500", metrics.InputTokens, metrics.OutputTokens)
	}

	if metrics.ToolTokens != 2 {
		t.Errorf("ToolTokens = %d, want 2", metrics.ToolTokens)
	}

	if metrics.ReasoningTokens != 1 {
		t.Errorf("ReasoningTokens = %d, want 1", metrics.ReasoningTokens)
	}
}

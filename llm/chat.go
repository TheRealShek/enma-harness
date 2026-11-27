package llm

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/tools"
)

type streamAccumulator struct {
	content    strings.Builder
	reasoning  strings.Builder
	toolCalls  []ToolCall
	usage      Usage
	inThinkTag bool
}

// Chat streams a completion from the configured default endpoint and model.
func Chat(messages []Message, onToken func(token string, isThinking bool)) (Message, Usage, error) {
	return ChatWithEndpoint(context.Background(), config.ChatCompletionsURL, config.DefaultModel, messages, onToken)
}

// ChatWithEndpoint posts messages as an SSE stream, separates reasoning from content, and assembles tool calls.
func ChatWithEndpoint(ctx context.Context, apiURL, model string, messages []Message, onToken func(token string, isThinking bool)) (Message, Usage, error) {
	// Build and send the streaming chat completions request.
	reqBody := Request{
		Model:    cmp.Or(model, config.DefaultModel),
		Messages: messages,
		Tools:    tools.Specs(),
		Stream:   true,
		StreamOptions: &StreamOption{
			IncludeUsage: true,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, Usage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmp.Or(apiURL, config.ChatCompletionsURL), bytes.NewReader(body))
	if err != nil {
		return Message{}, Usage{}, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Message{}, Usage{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)

		return Message{}, Usage{}, fmt.Errorf("status %d: %s", resp.StatusCode, data)
	}

	// Stream SSE chunks and finalize the assembled reply and token usage.
	acc, err := readSSEStream(resp.Body, onToken)
	if err != nil {
		return Message{}, Usage{}, err
	}

	return acc.finalize(messages)
}

// readSSEStream parses OpenAI-compatible SSE data lines into a streamAccumulator.
func readSSEStream(body io.Reader, onToken func(token string, isThinking bool)) (*streamAccumulator, error) {
	acc := &streamAccumulator{}

	scanner := bufio.NewScanner(body)
	buf := make([]byte, config.StreamInitialBufferBytes)
	scanner.Buffer(buf, config.StreamMaxBufferBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
			acc.usage = *chunk.Usage
		}

		for _, choice := range chunk.Choices {
			acc.applyDelta(choice.Delta, onToken)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("streaming error: %w", err)
	}

	return acc, nil
}

// applyDelta routes one streamed delta into reasoning, content, and tool-call buffers.
func (a *streamAccumulator) applyDelta(delta StreamDelta, onToken func(token string, isThinking bool)) {
	// Route explicit reasoning fields before inspecting inline <think> tags.
	if reasoningToken := cmp.Or(delta.Reasoning, delta.ReasoningContent); reasoningToken != "" {
		a.emit(reasoningToken, true, onToken)
	}

	if delta.Content != "" {
		a.consumeContent(delta.Content, onToken)
	}

	if len(delta.ToolCalls) > 0 {
		a.mergeToolCalls(delta.ToolCalls)
	}
}

// consumeContent splits inline <think>...</think> blocks out of delta.Content across chunk boundaries.
func (a *streamAccumulator) consumeContent(text string, onToken func(token string, isThinking bool)) {
	for len(text) > 0 {
		if a.inThinkTag {
			before, after, found := strings.Cut(text, "</think>")
			a.emit(before, true, onToken)

			if !found {
				return
			}

			a.inThinkTag = false
			text = after

			continue
		}

		if before, after, found := strings.Cut(text, "<think>"); found {
			a.emit(before, false, onToken)
			a.inThinkTag = true

			text = after

			continue
		}

		// Handle an orphan closing </think> tag when no opening tag was seen in-stream.
		if before, after, found := strings.Cut(text, "</think>"); found {
			a.emit(before, true, onToken)

			text = after

			continue
		}

		a.emit(text, false, onToken)

		return
	}
}

// emit appends non-empty text to the reasoning or content buffer and forwards it to onToken.
func (a *streamAccumulator) emit(text string, isThinking bool, onToken func(token string, isThinking bool)) {
	if text == "" {
		return
	}

	if isThinking {
		a.reasoning.WriteString(text)
	} else {
		a.content.WriteString(text)
	}

	if onToken != nil {
		onToken(text, isThinking)
	}
}

// mergeToolCalls reassembles chunked tool call names and JSON arguments by their stream index.
func (a *streamAccumulator) mergeToolCalls(deltas []StreamToolCall) {
	for _, tc := range deltas {
		for len(a.toolCalls) <= tc.Index {
			a.toolCalls = append(a.toolCalls, ToolCall{Type: "function"})
		}

		dst := &a.toolCalls[tc.Index]
		dst.ID = cmp.Or(tc.ID, dst.ID)
		dst.Type = cmp.Or(tc.Type, dst.Type)
		dst.Function.Name += tc.Function.Name
		dst.Function.Arguments += tc.Function.Arguments
	}
}

// finalize builds the assistant Message and estimates token usage when the server omits it.
func (a *streamAccumulator) finalize(messages []Message) (Message, Usage, error) {
	reply := Message{
		Role:      "assistant",
		Content:   a.content.String(),
		Reasoning: a.reasoning.String(),
		ToolCalls: a.toolCalls,
	}

	usage := a.usage
	if usage.CompletionTokens == 0 {
		contentTokens := (len(reply.Content) + 3) / 4
		reasoningTokens := (len(reply.Reasoning) + 3) / 4
		usage.CompletionTokens = contentTokens + reasoningTokens
	}

	if usage.PromptTokens == 0 {
		for _, msg := range messages {
			usage.PromptTokens += (len(msg.Content) + 3) / 4
		}
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return reply, usage, nil
}

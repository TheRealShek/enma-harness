package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/tools"
)

func chat(messages []Message, onToken func(token string, isThinking bool)) (Message, Usage, error) {
	reqBody := Request{
		Model:    config.DefaultModel,
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

	resp, err := http.Post(config.ChatCompletionsURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return Message{}, Usage{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)

		return Message{}, Usage{}, fmt.Errorf("status %d: %s", resp.StatusCode, data)
	}

	var reply Message

	reply.Role = "assistant"

	var usage Usage

	var contentBuilder strings.Builder

	var reasoningBuilder strings.Builder

	var toolCalls []ToolCall

	var inThinkTag bool

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, config.StreamInitialBufferBytes)
	scanner.Buffer(buf, config.StreamMaxBufferBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
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
			usage = *chunk.Usage
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			reasoningToken := delta.Reasoning
			if reasoningToken == "" && delta.ReasoningContent != "" {
				reasoningToken = delta.ReasoningContent
			}

			if reasoningToken != "" {
				reasoningBuilder.WriteString(reasoningToken)

				if onToken != nil {
					onToken(reasoningToken, true)
				}
			}

			if delta.Content != "" {
				text := delta.Content

				for len(text) > 0 {
					if !inThinkTag {
						idx := strings.Index(text, "<think>")
						if idx != -1 {
							if idx > 0 {
								before := text[:idx]
								contentBuilder.WriteString(before)

								if onToken != nil {
									onToken(before, false)
								}
							}

							inThinkTag = true

							text = text[idx+7:]

							continue
						}

						closeIdx := strings.Index(text, "</think>")
						if closeIdx != -1 {
							thinkingText := text[:closeIdx]
							if len(thinkingText) > 0 {
								reasoningBuilder.WriteString(thinkingText)

								if onToken != nil {
									onToken(thinkingText, true)
								}
							}

							text = text[closeIdx+8:]

							continue
						}

						contentBuilder.WriteString(text)

						if onToken != nil {
							onToken(text, false)
						}

						break
					}

					closeIdx := strings.Index(text, "</think>")
					if closeIdx != -1 {
						thinkingText := text[:closeIdx]
						if len(thinkingText) > 0 {
							reasoningBuilder.WriteString(thinkingText)

							if onToken != nil {
								onToken(thinkingText, true)
							}
						}

						inThinkTag = false

						text = text[closeIdx+8:]

						continue
					}

					reasoningBuilder.WriteString(text)

					if onToken != nil {
						onToken(text, true)
					}

					break
				}
			}

			for _, tc := range delta.ToolCalls {
				idx := tc.Index

				for len(toolCalls) <= idx {
					toolCalls = append(toolCalls, ToolCall{
						Type: "function",
					})
				}

				if tc.ID != "" {
					toolCalls[idx].ID = tc.ID
				}

				if tc.Type != "" {
					toolCalls[idx].Type = tc.Type
				}

				if tc.Function.Name != "" {
					toolCalls[idx].Function.Name += tc.Function.Name
				}

				if tc.Function.Arguments != "" {
					toolCalls[idx].Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Message{}, Usage{}, fmt.Errorf("streaming error: %w", err)
	}

	reply.Content = contentBuilder.String()
	reply.Reasoning = reasoningBuilder.String()
	reply.ToolCalls = toolCalls

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

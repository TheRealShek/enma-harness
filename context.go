package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/therealshek/enma-harness/config"
)

func formatWithCommas(number int) string {
	digits := strconv.Itoa(number)
	if len(digits) <= 3 {
		return digits
	}

	var formatted strings.Builder

	firstGroupLen := len(digits) % 3
	if firstGroupLen == 0 {
		firstGroupLen = 3
	}

	formatted.WriteString(digits[:firstGroupLen])

	for i := firstGroupLen; i < len(digits); i += 3 {
		formatted.WriteByte(',')
		formatted.WriteString(digits[i : i+3])
	}

	return formatted.String()
}

func countToolTokens(messages []Message) int {
	total := 0

	for _, msg := range messages {
		if msg.Role == "tool" {
			total += (len(msg.Content) + 3) / 4
		}
	}

	return total
}

func countReasoningTokens(usage Usage, reply Message) int {
	if usage.CompletionTokensDetails.ReasoningTokens > 0 {
		return usage.CompletionTokensDetails.ReasoningTokens
	}

	if len(reply.Reasoning) > 0 {
		return (len(reply.Reasoning) + 3) / 4
	}

	start := strings.Index(reply.Content, "<think>")
	end := strings.Index(reply.Content, "</think>")

	if start != -1 && end != -1 && end > start {
		return (len(reply.Content[start+7:end]) + 3) / 4
	}

	return 0
}

func calculateContextMetrics(usage Usage, conversation []Message, reply Message) ContextMetrics {
	return ContextMetrics{
		TotalTokens:     usage.TotalTokens,
		MaxTokens:       config.MaxContextTokens,
		PercentUsed:     float64(usage.TotalTokens) / float64(config.MaxContextTokens) * 100,
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CompletionTokens,
		ToolTokens:      countToolTokens(conversation),
		ReasoningTokens: countReasoningTokens(usage, reply),
	}
}

func printContextBadge(metrics ContextMetrics) {
	badge := fmt.Sprintf("[Context: %s / %s tokens (%.0f%%) | in: %s • out: %s • tool: %s",
		formatWithCommas(metrics.TotalTokens),
		formatWithCommas(metrics.MaxTokens),
		metrics.PercentUsed,
		formatWithCommas(metrics.InputTokens),
		formatWithCommas(metrics.OutputTokens),
		formatWithCommas(metrics.ToolTokens),
	)

	if metrics.ReasoningTokens > 0 {
		badge += fmt.Sprintf(" • reason: %s", formatWithCommas(metrics.ReasoningTokens))
	}

	badge += "]"
	fmt.Println(styleMuted(badge))
}

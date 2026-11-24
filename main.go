package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/therealshek/enma-harness/tools"
)

const (
	url              = "http://localhost:11434/v1/chat/completions"
	model            = "qwen3:4b"
	systemPrompt     = "Answer directly. Avoid meta commentary, self-evaluation, and unnecessary explanations. Explain only when asked or when needed for clarity. Keep responses concise but complete."
	maxContextTokens = 8192
)

func main() {
	conversation := []Message{{Role: "system", Content: systemPrompt}}
	scanner := bufio.NewScanner(os.Stdin)
	needUserInput := true

	for {
		if needUserInput {
			fmt.Print(styleUser("You:") + " ")
			if !scanner.Scan() {
				break
			}
			conversation = append(conversation, Message{Role: "user", Content: scanner.Text()})
			fmt.Println()
		}

		reply, usage, err := chat(conversation)
		if err != nil {
			fmt.Println(styleError("Error:"), err)
			needUserInput = true
			if scanner.Text() == "" {
				continue
			}
		}
		conversation = append(conversation, reply)

		if reply.Content != "" {
			fmt.Println(styleAssistant("Qwen:"), reply.Content)
			printContextBadge(calculateContextMetrics(usage, conversation, reply))
			fmt.Println()
		}

		needUserInput = len(reply.ToolCalls) == 0
		for _, toolCall := range reply.ToolCalls {
			fmt.Println(styleTool(fmt.Sprintf("tool: %s(%s)", toolCall.Function.Name, toolCall.Function.Arguments)))
			fmt.Println()
			toolOutput := tools.Execute(toolCall.Function.Name, toolCall.Function.Arguments)
			conversation = append(conversation, Message{Role: "tool", ToolCallID: toolCall.ID, Content: toolOutput})
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

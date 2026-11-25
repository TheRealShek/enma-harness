package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/therealshek/enma-harness/tools"
)

const (
	url              = "http://localhost:11434/v1/chat/completions"
	model            = "qwen3:4b"
	systemPrompt     = "Answer directly. Avoid meta commentary, self-evaluation, and unnecessary explanations. Keep internal reasoning concise and minimal. Explain only when asked or when needed for clarity. Keep responses concise but complete."
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

		var (
			startedThinking  bool
			thinkingEnded    bool
			startedAssistant bool
		)

		onToken := func(token string, isThinking bool) {
			if isThinking {
				if !startedThinking {
					fmt.Println(styleMuted("│ [thinking]"))
					fmt.Print(styleMuted("│ "))

					startedThinking = true
				}

				lines := strings.Split(token, "\n")

				for i, line := range lines {
					if i > 0 {
						fmt.Print("\n" + styleMuted("│ "))
					}

					if line != "" {
						fmt.Print(styleMuted(line))
					}
				}

				return
			}

			if startedThinking && !thinkingEnded {
				fmt.Println()
				fmt.Println()

				thinkingEnded = true
			}

			if !startedAssistant {
				fmt.Print(styleAssistant("Qwen:") + " ")

				startedAssistant = true
			}

			fmt.Print(token)
		}

		reply, usage, err := chat(conversation, onToken)
		if err != nil {
			fmt.Println(styleError("Error:"), err)

			needUserInput = true

			if scanner.Text() == "" {
				continue
			}
		}

		conversation = append(conversation, reply)

		if startedThinking && !thinkingEnded {
			fmt.Println()
			fmt.Println()

			thinkingEnded = true
		}

		if !startedAssistant && reply.Content != "" {
			fmt.Println(styleAssistant("Qwen:"), reply.Content)
			printContextBadge(calculateContextMetrics(usage, conversation, reply))
			fmt.Println()
		} else if startedAssistant {
			fmt.Println()
			printContextBadge(calculateContextMetrics(usage, conversation, reply))
			fmt.Println()
		}

		needUserInput = len(reply.ToolCalls) == 0

		for _, toolCall := range reply.ToolCalls {
			fmt.Println(styleTool("tool: " + toolCall.Function.Name))

			var args map[string]any

			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil && len(args) > 0 {
				keys := make([]string, 0, len(args))

				for k := range args {
					keys = append(keys, k)
				}

				sort.Strings(keys)

				for _, k := range keys {
					valStr := fmt.Sprintf("%v", args[k])
					if len(valStr) > 60 {
						valStr = valStr[:57] + "..."
					}

					valStr = strings.ReplaceAll(valStr, "\n", " ")
					fmt.Println(styleMuted(fmt.Sprintf("  %s: %s", k, valStr)))
				}
			} else if strings.TrimSpace(toolCall.Function.Arguments) != "" {
				fmt.Println(styleMuted("  args: " + toolCall.Function.Arguments))
			}

			toolOutput := tools.Execute(toolCall.Function.Name, toolCall.Function.Arguments)

			if strings.HasPrefix(toolOutput, "error:") {
				fmt.Println(styleError("  └─ " + toolOutput))
			} else {
				lines := strings.Split(toolOutput, "\n")
				if len(lines) > 0 {
					fmt.Println(styleSuccess("  └─ ") + styleMuted(lines[0]))

					for _, line := range lines[1:] {
						if strings.TrimSpace(line) != "" {
							fmt.Println(styleMuted("     " + line))
						}
					}
				}
			}

			fmt.Println()

			conversation = append(conversation, Message{Role: "tool", ToolCallID: toolCall.ID, Content: toolOutput})
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

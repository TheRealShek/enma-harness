package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/tools"
)

func main() {
	// Initialize conversation, stdin scanner, and tool approval session.
	conversation := []Message{{Role: "system", Content: config.DefaultSystemPrompt}}
	scanner := bufio.NewScanner(os.Stdin)
	needUserInput := true

	wd, _ := os.Getwd()
	session := &tools.Session{
		ProjectRoot: wd,
		Approve: func(action, target string, tainted bool) bool {
			if tainted {
				fmt.Println(styleError("Warning: session has read untrusted data. Review what the model is trying to do before approving."))
			}

			fmt.Printf("%s %s (%s) [y/N]: ", styleTool("Approval required:"), action, target)

			if !scanner.Scan() {
				return false
			}

			ans := strings.TrimSpace(strings.ToLower(scanner.Text()))

			return ans == "y" || ans == "yes"
		},
	}

	for {
		// Prompt for user input when the previous turn has no pending tool calls.
		if needUserInput {
			fmt.Print(styleUser("You:") + " ")

			if !scanner.Scan() {
				break
			}

			conversation = append(conversation, Message{Role: "user", Content: scanner.Text()})

			fmt.Println()
		}

		// Stream thinking and answer tokens from the model.
		renderer := newStreamRenderer()

		reply, usage, err := chat(conversation, renderer.onToken)
		if err != nil {
			renderer.stopSpinner()
			fmt.Println(styleError("Error:"), err)

			needUserInput = true

			if scanner.Text() == "" {
				continue
			}
		}

		// Save the assistant reply and print context usage.
		conversation = append(conversation, reply)

		renderer.finish(reply, usage, conversation)

		// Run requested tool calls, display results, and append tool outputs.
		needUserInput = len(reply.ToolCalls) == 0

		for _, toolCall := range reply.ToolCalls {
			printToolArgs(toolCall.Function.Name, toolCall.Function.Arguments)

			toolOutput := tools.ExecuteWithSession(session, toolCall.Function.Name, toolCall.Function.Arguments)
			printToolOutput(toolOutput)

			conversation = append(conversation, Message{Role: "tool", ToolCallID: toolCall.ID, Content: toolOutput})
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

func main() {
	// Initialize conversation, stdin scanner, thread store, and tool approval session.
	scanner := bufio.NewScanner(os.Stdin)
	needUserInput := true

	wd, _ := os.Getwd()

	var store *threads.Store

	if !config.InMemoryOnly {
		var err error

		store, err = threads.DefaultStore()
		if err != nil {
			fmt.Println(styleError("Warning: thread persistence disabled:"), err)
		}
	}

	toolSession := &tools.Session{
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

	cli := &cliSession{
		store:        store,
		projectRoot:  wd,
		conversation: []Message{{Role: "system", Content: config.DefaultSystemPrompt}},
		toolSession:  toolSession,
		scanner:      scanner,
	}

	for {
		// Prompt for user input when the previous turn has no pending tool calls.
		if needUserInput {
			fmt.Print(styleUser("You:") + " ")

			if !scanner.Scan() {
				break
			}

			input := scanner.Text()
			if strings.TrimSpace(input) == "" {
				continue
			}

			if cli.handleCommand(input) {
				continue
			}

			cli.createThreadIfNeeded(input)
			cli.appendMessage(Message{Role: "user", Content: input})

			fmt.Println()
		}

		// Stream thinking and answer tokens from the model.
		renderer := newStreamRenderer()

		reply, usage, err := chat(cli.conversation, renderer.onToken)
		if err != nil {
			renderer.stopSpinner()
			fmt.Println(styleError("Error:"), err)

			needUserInput = true

			if scanner.Text() == "" {
				continue
			}
		}

		// Save the assistant reply and print context usage.
		cli.appendMessage(reply)

		renderer.finish(reply, usage, cli.conversation)

		// Run requested tool calls, display results, and append tool outputs.
		needUserInput = len(reply.ToolCalls) == 0

		for _, toolCall := range reply.ToolCalls {
			printToolArgs(toolCall.Function.Name, toolCall.Function.Arguments)

			toolOutput := tools.ExecuteWithSession(toolSession, toolCall.Function.Name, toolCall.Function.Arguments)

			cli.persistTaintState()
			printToolOutput(toolOutput)

			cli.appendMessage(Message{Role: "tool", ToolCallID: toolCall.ID, Content: toolOutput})
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

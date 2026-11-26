package main

import (
	"bufio"
	"context"
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

	// Wire the headless runner and CLI command session.
	runner := NewRunner(store, wd, toolSession)
	runner.onWarning = func(msg string) {
		fmt.Println(styleError(msg))
	}

	cli := &cliSession{
		runner:  runner,
		scanner: scanner,
	}

	// Read user prompts and handle slash commands before running a turn.
	for {
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

		fmt.Println()

		// Stream the model response and tool executions through the runner.
		renderer := newStreamRenderer()

		_ = runner.RunPrompt(context.Background(), input, TurnHooks{
			OnToken: renderer.onToken,
			OnReply: func(reply Message, usage Usage, conversation []Message) {
				renderer.finish(reply, usage, conversation)
			},
			OnToolStart:  printToolArgs,
			OnToolResult: printToolOutput,
			OnError: func(err error) {
				renderer.stopSpinner()
				fmt.Println(styleError("Error:"), err)
			},
		})
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

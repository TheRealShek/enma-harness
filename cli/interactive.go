package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/harness"
	"github.com/therealshek/enma-harness/llm"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

// RunInteractive wires the store, runner, and terminal loop for interactive CLI sessions.
func RunInteractive(ctx context.Context) error {
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

	runner := harness.NewRunner(store, wd, newToolSession(wd, scanner))
	runner.OnWarning = func(msg string) {
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

		_ = runner.RunPrompt(ctx, input, harness.TurnHooks{
			OnToken: renderer.onToken,
			OnReply: func(reply llm.Message, usage llm.Usage, conversation []llm.Message) {
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

	return scanner.Err()
}

// newToolSession creates a tool session that prompts on stdin for sensitive or tainted actions.
func newToolSession(wd string, scanner *bufio.Scanner) *tools.Session {
	return &tools.Session{
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
}

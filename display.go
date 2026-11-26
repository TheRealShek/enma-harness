package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/therealshek/enma-harness/config"
)

var thinkingSpinnerFrames = []string{"◇", "◈", "◆", "◈"}

type streamRenderer struct {
	showThinking     bool
	startedThinking  bool
	thinkingEnded    bool
	startedAssistant bool

	spinnerMu   sync.Mutex
	spinnerStop chan struct{}
	spinnerDone chan struct{}
}

// newStreamRenderer creates a streamRenderer configured from config.ShowThinkingTraces.
func newStreamRenderer() *streamRenderer {
	return &streamRenderer{
		showThinking: config.ShowThinkingTraces,
	}
}

// onToken prints streamed thinking traces (or a loading indicator when hidden) and assistant text.
func (r *streamRenderer) onToken(token string, isThinking bool) {
	if isThinking {
		if !r.showThinking {
			if !r.startedThinking {
				r.startedThinking = true
				r.startSpinner()
			}

			return
		}

		if !r.startedThinking {
			fmt.Println(styleMuted("│ [thinking]"))
			fmt.Print(styleMuted("│ "))

			r.startedThinking = true
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

	if r.startedThinking && !r.thinkingEnded {
		r.stopSpinner()

		if r.showThinking {
			fmt.Println()
			fmt.Println()
		}

		r.thinkingEnded = true
	}

	if !r.startedAssistant {
		fmt.Print(styleAssistant("Qwen:") + " ")

		r.startedAssistant = true
	}

	fmt.Print(token)
}

// finish closes any open thinking block, prints unstreamed content, and shows context metrics.
func (r *streamRenderer) finish(reply Message, usage Usage, conversation []Message) {
	if r.startedThinking && !r.thinkingEnded {
		r.stopSpinner()

		if r.showThinking {
			fmt.Println()
			fmt.Println()
		}

		r.thinkingEnded = true
	}

	if !r.startedAssistant && reply.Content != "" {
		fmt.Println(styleAssistant("Qwen:"), reply.Content)
		printContextBadge(calculateContextMetrics(usage, conversation, reply))
		fmt.Println()
	} else if r.startedAssistant {
		fmt.Println()
		printContextBadge(calculateContextMetrics(usage, conversation, reply))
		fmt.Println()
	}
}

func (r *streamRenderer) startSpinner() {
	if !isStdoutTTY() {
		return
	}

	r.spinnerMu.Lock()
	defer r.spinnerMu.Unlock()

	if r.spinnerStop != nil {
		return
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	r.spinnerStop = stop
	r.spinnerDone = done

	go func() {
		defer close(done)

		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		idx := 0
		fmt.Print("\r" + styleMuted(thinkingSpinnerFrames[idx]+" Thinking..."))

		for {
			select {
			case <-stop:
				fmt.Print("\r\033[K")

				return
			case <-ticker.C:
				idx = (idx + 1) % len(thinkingSpinnerFrames)
				fmt.Print("\r" + styleMuted(thinkingSpinnerFrames[idx]+" Thinking..."))
			}
		}
	}()
}

func (r *streamRenderer) stopSpinner() {
	r.spinnerMu.Lock()
	stop := r.spinnerStop
	done := r.spinnerDone
	r.spinnerStop = nil
	r.spinnerDone = nil
	r.spinnerMu.Unlock()

	if stop == nil {
		return
	}

	close(stop)
	<-done
}

func isStdoutTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

// printToolArgs displays the tool name and a sorted preview of its JSON arguments.
func printToolArgs(name, rawArgs string) {
	fmt.Println(styleTool("tool: " + name))

	var args map[string]any

	if err := json.Unmarshal([]byte(rawArgs), &args); err == nil && len(args) > 0 {
		for _, k := range slices.Sorted(maps.Keys(args)) {
			valStr := fmt.Sprintf("%v", args[k])
			if len(valStr) > config.ToolArgPreviewMaxChars {
				valStr = valStr[:config.ToolArgPreviewMaxChars-3] + "..."
			}

			valStr = strings.ReplaceAll(valStr, "\n", " ")
			fmt.Println(styleMuted(fmt.Sprintf("  %s: %s", k, valStr)))
		}
	} else if strings.TrimSpace(rawArgs) != "" {
		fmt.Println(styleMuted("  args: " + rawArgs))
	}
}

// printToolOutput formats and prints the result or error returned by a tool call.
func printToolOutput(output string) {
	if strings.HasPrefix(output, "error:") {
		fmt.Println(styleError("  └─ " + output))
		fmt.Println()

		return
	}

	first, rest, hasRest := strings.Cut(output, "\n")
	fmt.Println(styleSuccess("  └─ ") + styleMuted(first))

	if hasRest {
		for line := range strings.SplitSeq(rest, "\n") {
			if strings.TrimSpace(line) != "" {
				fmt.Println(styleMuted("     " + line))
			}
		}
	}

	fmt.Println()
}

package cli

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
	"github.com/therealshek/enma-harness/harness"
	"github.com/therealshek/enma-harness/llm"
)

var thinkingSpinnerFrames = []string{"◇", "◈", "◆", "◈"}

type streamRenderer struct {
	showThinking     bool
	startedAt        time.Time
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
		startedAt:    time.Now(),
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

	r.endThinking()

	if !r.startedAssistant {
		fmt.Print(styleAssistant("Qwen:") + " ")

		r.startedAssistant = true
	}

	fmt.Print(token)
}

// finish closes any open thinking block, prints unstreamed content, and shows context metrics.
func (r *streamRenderer) finish(reply llm.Message, usage llm.Usage, conversation []llm.Message) {
	r.endThinking()

	if !r.startedAssistant && reply.Content != "" {
		fmt.Println(styleAssistant("Qwen:"), reply.Content)
		fmt.Println(styleMuted(harness.FormatContextBadge(harness.CalculateContextMetrics(usage, conversation, reply))))
		fmt.Println()
	} else if r.startedAssistant {
		fmt.Println()
		fmt.Println(styleMuted(harness.FormatContextBadge(harness.CalculateContextMetrics(usage, conversation, reply))))
		fmt.Println()
	}
}

// endThinking stops the spinner and prints total elapsed thinking time when reasoning traces are hidden.
func (r *streamRenderer) endThinking() {
	if !r.startedThinking || r.thinkingEnded {
		return
	}

	r.stopSpinner()

	if r.showThinking {
		fmt.Println()
		fmt.Println()
	} else if !r.startedAt.IsZero() {
		fmt.Println(styleMuted(formatThoughtDuration(time.Since(r.startedAt))))
	}

	r.thinkingEnded = true
}

// formatThoughtDuration formats elapsed thinking time in seconds or minutes and seconds.
func formatThoughtDuration(d time.Duration) string {
	if d < time.Second {
		return "Thought for <1s"
	}

	secs := int(d.Round(time.Second).Seconds())
	if secs < 60 {
		return fmt.Sprintf("Thought for %ds", secs)
	}

	mins := secs / 60
	rem := secs % 60

	if rem == 0 {
		return fmt.Sprintf("Thought for %dm", mins)
	}

	return fmt.Sprintf("Thought for %dm %ds", mins, rem)
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

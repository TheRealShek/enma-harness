package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/eval"
	"github.com/therealshek/enma-harness/harness"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

// RunEvalCommand parses flags and executes `enma eval` or `-diff`, returning a process exit code.
func RunEvalCommand(args []string) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	tasksDir := fs.String("tasks", config.DefaultEvalTasksDir, "path to eval tasks directory")
	resultsDir := fs.String("results", config.DefaultEvalResultsDir, "path to eval results directory")
	runs := fs.Int("runs", config.DefaultEvalRuns, "number of trials per task")
	filter := fs.String("filter", "", "substring filter for task names")
	model := fs.String("model", config.DefaultModel, "model name override")
	apiURL := fs.String("url", config.ChatCompletionsURL, "chat completions endpoint URL")
	diffArg := fs.String("diff", "", "compare two run IDs or directories as runA,runB")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *diffArg != "" {
		return runEvalDiff(*diffArg, *resultsDir)
	}

	summary, err := eval.Run(context.Background(), eval.Config{
		TasksDir:   *tasksDir,
		ResultsDir: *resultsDir,
		Filter:     *filter,
		Model:      *model,
		Runs:       *runs,
		RunTrial: func(ctx context.Context, workDir string, task eval.Task, store *threads.Store) (string, error) {
			toolSession := &tools.Session{
				ProjectRoot: workDir,
				Approve:     eval.ApproveFuncForPolicy(task.ApprovalPolicy, workDir),
			}

			runner := harness.NewRunner(store, workDir, toolSession)
			runner.Model = *model
			runner.APIURL = *apiURL
			runner.MaxTurns = task.MaxTurns
			runner.PendingThreadName = task.Name

			runErr := runner.RunPrompt(ctx, task.Prompt, harness.TurnHooks{})

			threadID := ""
			if runner.ActiveThread != nil {
				threadID = runner.ActiveThread.ID
			}

			return threadID, runErr
		},
		OnProgress: func(taskName string, trial, total int, passed bool, durationMS int64) {
			status := styleError("FAIL")
			if passed {
				status = styleSuccess("PASS")
			}

			fmt.Printf("[%s] %s (%d/%d) %dms\n", status, taskName, trial, total, durationMS)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError("Eval failed:"), err)

		return 1
	}

	fmt.Println()
	fmt.Print(eval.FormatTable(summary))
	fmt.Printf("Saved to %s\n", filepath.Join(*resultsDir, summary.RunID))

	return 0
}

func runEvalDiff(spec, resultsDir string) int {
	aSpec, bSpec, ok := strings.Cut(spec, ",")
	if !ok || strings.TrimSpace(aSpec) == "" || strings.TrimSpace(bSpec) == "" {
		fmt.Fprintln(os.Stderr, styleError("Usage: enma eval -diff <runA>,<runB>"))

		return 2
	}

	resolveRun := func(s string) string {
		s = strings.TrimSpace(s)
		if info, err := os.Stat(s); err == nil && info.IsDir() {
			return s
		}

		return filepath.Join(resultsDir, s)
	}

	a, err := eval.LoadOrBuildSummary(resolveRun(aSpec))
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError("Error loading run A:"), err)

		return 1
	}

	b, err := eval.LoadOrBuildSummary(resolveRun(bSpec))
	if err != nil {
		fmt.Fprintln(os.Stderr, styleError("Error loading run B:"), err)

		return 1
	}

	fmt.Print(eval.FormatDiff(a, b))

	return 0
}

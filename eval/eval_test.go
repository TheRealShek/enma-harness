package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealshek/enma-harness/threads"
)

func TestRunAndLazySummaryRebuild(t *testing.T) {
	tasksDir := t.TempDir()
	resultsDir := t.TempDir()

	wsDir := filepath.Join(tasksDir, "sample-task", "workspace")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(wsDir, "answer.txt"), []byte("wrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	taskJSON := `{"prompt":"fix answer.txt","check":["sh","-c","grep -q ok answer.txt"],"max_turns":3,"timeout_sec":10}`
	if err := os.WriteFile(filepath.Join(tasksDir, "sample-task", "task.json"), []byte(taskJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	trialNum := 0

	summary, err := Run(context.Background(), Config{
		TasksDir:   tasksDir,
		ResultsDir: resultsDir,
		Model:      "test-model",
		Runs:       2,
		RunTrial: func(ctx context.Context, workDir string, task Task, store *threads.Store) (string, error) {
			trialNum++

			meta, err := store.Create(workDir, "test-model", task.Name, false)
			if err != nil {
				return "", err
			}

			ev, _ := threads.NewEvent(1, threads.EventLLMResponse, map[string]any{
				"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
			})
			_ = store.AppendEvent(meta.ID, ev)

			// Pass on trial 1, leave wrong on trial 2.
			if trialNum == 1 {
				_ = os.WriteFile(filepath.Join(workDir, "answer.txt"), []byte("ok\n"), 0o644)
			}

			return meta.ID, nil
		},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if summary.Passed != 1 || summary.Total != 2 {
		t.Fatalf("summary = %d/%d, want 1/2", summary.Passed, summary.Total)
	}

	// Removing summary.json should still allow lazy reconstruction from events.jsonl.
	runDir := filepath.Join(resultsDir, summary.RunID)
	_ = os.Remove(filepath.Join(runDir, summaryFileName))

	rebuilt, err := LoadOrBuildSummary(runDir)
	if err != nil {
		t.Fatalf("LoadOrBuildSummary failed: %v", err)
	}

	if rebuilt.Passed != 1 || rebuilt.Total != 2 || len(rebuilt.Tasks) != 1 || rebuilt.Tasks[0].AvgTokens != 15 {
		t.Fatalf("rebuilt = %+v, want 1/2 with AvgTokens=15", rebuilt)
	}

	if table := FormatTable(rebuilt); !strings.Contains(table, "1/2") {
		t.Errorf("unexpected table:\n%s", table)
	}
}

func TestApproveFuncForPolicy(t *testing.T) {
	root := t.TempDir()

	deny := ApproveFuncForPolicy("deny_all", root)
	if deny("write sensitive file while tainted", filepath.Join(root, "AGENTS.md"), true) {
		t.Fatal("deny_all should never approve")
	}

	allow := ApproveFuncForPolicy("allow_project", root)
	if !allow("write sensitive file while tainted", filepath.Join(root, "AGENTS.md"), true) {
		t.Fatal("allow_project should approve in-project file")
	}

	if allow("write outside project", "/tmp/outside.txt", true) {
		t.Fatal("allow_project must deny outside-project writes")
	}

	if allow("write sensitive repo file", filepath.Join(root, ".git/hooks/pre-commit"), false) {
		t.Fatal("allow_project must deny .git writes")
	}
}

func TestLoadBuiltInTasks(t *testing.T) {
	tasks, err := LoadTasks(filepath.Join("..", "evals", "tasks"), "")
	if err != nil {
		t.Fatalf("LoadTasks failed: %v", err)
	}

	if len(tasks) != 10 {
		t.Fatalf("len(tasks) = %d, want 10", len(tasks))
	}
}

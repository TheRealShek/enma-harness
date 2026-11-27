package eval

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/threads"
)

const (
	taskFileName    = "task.json"
	summaryFileName = "summary.csv"
	workspaceDir    = "workspace"
	maxCheckOutput  = 8 * 1024
)

// Task describes one benchmark task loaded from evals/tasks/<name>/task.json.
type Task struct {
	Name           string   `json:"-"`
	WorkspaceDir   string   `json:"-"`
	Prompt         string   `json:"prompt"`
	Check          []string `json:"check"`
	MaxTurns       int      `json:"max_turns"`
	TimeoutSec     int      `json:"timeout_sec"`
	ApprovalPolicy string   `json:"approval_policy"`
}

// TrialRunner executes one prompt inside workDir using store and returns the created thread ID.
type TrialRunner func(ctx context.Context, workDir string, task Task, store *threads.Store) (threadID string, err error)

// Config controls an eval run.
type Config struct {
	TasksDir   string
	ResultsDir string
	Filter     string
	Model      string
	Runs       int
	RunTrial   TrialRunner
	OnProgress func(taskName string, trial, total int, passed bool, durationMS int64)
}

// CheckData is the payload written to threads.EventEvalCheck in events.jsonl.
type CheckData struct {
	Command         []string `json:"command"`
	ExitCode        int      `json:"exit_code"`
	Passed          bool     `json:"passed"`
	DurationMS      int64    `json:"duration_ms"`
	TrialDurationMS int64    `json:"trial_duration_ms"`
	RunError        string   `json:"run_error,omitempty"`
	Output          string   `json:"output,omitempty"`
}

// TrialMetrics holds aggregated metrics folded from one trial's events.jsonl.
type TrialMetrics struct {
	ThreadID         string `json:"thread_id"`
	Task             string `json:"task"`
	Trial            int    `json:"trial"`
	Passed           bool   `json:"passed"`
	RunError         string `json:"run_error,omitempty"`
	CheckExitCode    int    `json:"check_exit_code"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	ReasoningTokens  int    `json:"reasoning_tokens"`
	ToolCalls        int    `json:"tool_calls"`
	ToolErrors       int    `json:"tool_errors"`
	ApprovalsDenied  int    `json:"approvals_denied"`
	DurationMS       int64  `json:"duration_ms"`
}

// TaskSummary aggregates all trials for a single task.
type TaskSummary struct {
	Task          string         `json:"task"`
	Passed        int            `json:"passed"`
	Total         int            `json:"total"`
	PassRate      float64        `json:"pass_rate"`
	AvgTokens     float64        `json:"avg_tokens"`
	AvgToolCalls  float64        `json:"avg_tool_calls"`
	AvgDurationMS float64        `json:"avg_duration_ms"`
	Trials        []TrialMetrics `json:"trials"`
}

// RunSummary aggregates all tasks in an eval run and is written lazily at the end of Run.
type RunSummary struct {
	RunID       string        `json:"run_id"`
	Model       string        `json:"model"`
	RunsPerTask int           `json:"runs_per_task"`
	StartedAt   time.Time     `json:"started_at"`
	DurationMS  int64         `json:"duration_ms"`
	Passed      int           `json:"passed"`
	Total       int           `json:"total"`
	PassRate    float64       `json:"pass_rate"`
	Tasks       []TaskSummary `json:"tasks"`
}

// DefaultResultsDir returns the default directory where eval run results are stored.
func DefaultResultsDir() string {
	return config.DefaultEvalResultsDir
}

// LoadTasks reads and validates task directories under tasksDir.
func LoadTasks(tasksDir, filter string) ([]Task, error) {
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks directory %s: %w", tasksDir, err)
	}

	var tasks []Task

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		name := e.Name()
		if filter != "" && !strings.Contains(name, filter) {
			continue
		}

		t, err := loadTask(tasksDir, name)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, t)
	}

	slices.SortFunc(tasks, func(a, b Task) int {
		return cmp.Compare(a.Name, b.Name)
	})

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no matching tasks found in %s", tasksDir)
	}

	return tasks, nil
}

// loadTask reads and validates one task directory's task.json and workspace folder.
func loadTask(tasksDir, name string) (Task, error) {
	taskDir := filepath.Join(tasksDir, name)

	raw, err := os.ReadFile(filepath.Join(taskDir, taskFileName))
	if err != nil {
		return Task{}, fmt.Errorf("failed to read %s/%s: %w", name, taskFileName, err)
	}

	var t Task
	if err := json.Unmarshal(raw, &t); err != nil {
		return Task{}, fmt.Errorf("failed to parse %s/%s: %w", name, taskFileName, err)
	}

	t.Name = name
	t.WorkspaceDir = filepath.Join(taskDir, workspaceDir)
	t.MaxTurns = cmp.Or(t.MaxTurns, config.DefaultEvalMaxTurns)
	t.TimeoutSec = cmp.Or(t.TimeoutSec, config.DefaultEvalTimeoutSec)
	t.ApprovalPolicy = cmp.Or(strings.TrimSpace(t.ApprovalPolicy), "deny_all")

	if strings.TrimSpace(t.Prompt) == "" {
		return Task{}, fmt.Errorf("task %s has empty prompt", name)
	}

	if len(t.Check) == 0 {
		return Task{}, fmt.Errorf("task %s has empty check command", name)
	}

	info, err := os.Stat(t.WorkspaceDir)
	if err != nil || !info.IsDir() {
		return Task{}, fmt.Errorf("task %s missing %s directory", name, workspaceDir)
	}

	return t, nil
}

// ApproveFuncForPolicy builds a headless tool approval callback for an eval task.
func ApproveFuncForPolicy(policy, projectRoot string) func(action, target string, tainted bool) bool {
	if policy != "allow_project" {
		return func(string, string, bool) bool {
			return false
		}
	}

	cleanRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		cleanRoot = filepath.Clean(projectRoot)
	}

	return func(action, target string, tainted bool) bool {
		if strings.Contains(action, "harness state") {
			return false
		}

		cleanTarget := filepath.Clean(target)

		rel, err := filepath.Rel(cleanRoot, cleanTarget)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}

		for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
			if part == ".git" || part == config.HarnessDirName {
				return false
			}
		}

		return true
	}
}

// Run executes each task Runs times in isolated temp workspaces and writes summary.json lazily at the end.
func Run(ctx context.Context, cfg Config) (*RunSummary, error) {
	if cfg.RunTrial == nil {
		return nil, errors.New("RunTrial callback is required")
	}

	cfg.Runs = max(1, cfg.Runs)

	tasks, err := LoadTasks(cfg.TasksDir, cfg.Filter)
	if err != nil {
		return nil, err
	}

	resultsDir := cfg.ResultsDir
	if resultsDir == "" {
		resultsDir = DefaultResultsDir()
	}

	runID, err := newRunID()
	if err != nil {
		return nil, err
	}

	runDir := filepath.Join(resultsDir, runID)
	store := &threads.Store{BaseDir: filepath.Join(runDir, "threads")}
	startedAt := time.Now().UTC()

	summary := &RunSummary{
		RunID:       runID,
		Model:       cmp.Or(cfg.Model, config.DefaultModel),
		RunsPerTask: cfg.Runs,
		StartedAt:   startedAt,
	}

	for _, task := range tasks {
		ts := TaskSummary{Task: task.Name, Total: cfg.Runs}

		for i := range cfg.Runs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			metrics := runSingleTrial(ctx, task, i+1, store, cfg.RunTrial)
			ts.Trials = append(ts.Trials, metrics)

			if metrics.Passed {
				ts.Passed++
			}

			if cfg.OnProgress != nil {
				cfg.OnProgress(task.Name, i+1, cfg.Runs, metrics.Passed, metrics.DurationMS)
			}
		}

		finalizeTaskSummary(&ts)
		summary.Tasks = append(summary.Tasks, ts)
		summary.Passed += ts.Passed
		summary.Total += ts.Total
	}

	summary.DurationMS = time.Since(startedAt).Milliseconds()
	if summary.Total > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.Total)
	}

	if err := writeSummary(filepath.Join(runDir, summaryFileName), summary); err != nil {
		return nil, err
	}

	return summary, nil
}

func runSingleTrial(ctx context.Context, task Task, trialNum int, store *threads.Store, runTrial TrialRunner) TrialMetrics {
	start := time.Now()
	metrics := TrialMetrics{Task: task.Name, Trial: trialNum}

	tempDir, err := os.MkdirTemp("", "enma-eval-*")
	if err != nil {
		metrics.RunError = err.Error()
		metrics.DurationMS = time.Since(start).Milliseconds()

		return metrics
	}

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	workDir := filepath.Join(tempDir, workspaceDir)
	if err := copyDir(task.WorkspaceDir, workDir); err != nil {
		metrics.RunError = err.Error()
		metrics.DurationMS = time.Since(start).Milliseconds()

		return metrics
	}

	trialCtx, cancel := context.WithTimeout(ctx, time.Duration(task.TimeoutSec)*time.Second)
	defer cancel()

	threadID, runErr := runTrial(trialCtx, workDir, task, store)
	metrics.ThreadID = threadID

	if runErr != nil {
		metrics.RunError = runErr.Error()
	}

	checkCtx, checkCancel := context.WithTimeout(ctx, time.Duration(task.TimeoutSec)*time.Second)
	defer checkCancel()

	check := runCheckCommand(checkCtx, workDir, task.Check)
	check.RunError = metrics.RunError
	check.TrialDurationMS = time.Since(start).Milliseconds()

	metrics.CheckExitCode = check.ExitCode
	metrics.Passed = runErr == nil && check.Passed
	metrics.DurationMS = check.TrialDurationMS

	if threadID != "" {
		appendCheckEvent(store, threadID, check)
		foldThreadMetrics(store, threadID, &metrics)
	}

	return metrics
}

func runCheckCommand(ctx context.Context, dir string, argv []string) CheckData {
	start := time.Now()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir

	var buf bytes.Buffer

	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	out := buf.String()
	if len(out) > maxCheckOutput {
		out = out[:maxCheckOutput] + "\n... (truncated)"
	}

	return CheckData{
		Command:    argv,
		ExitCode:   exitCode,
		Passed:     err == nil,
		DurationMS: time.Since(start).Milliseconds(),
		Output:     out,
	}
}

func appendCheckEvent(store *threads.Store, threadID string, check CheckData) {
	_, events, err := store.Load(threadID)
	if err != nil {
		return
	}

	var lastSeq int64
	for _, ev := range events {
		if ev.Sequence > lastSeq {
			lastSeq = ev.Sequence
		}
	}

	ev, err := threads.NewEvent(lastSeq+1, threads.EventEvalCheck, check)
	if err != nil {
		return
	}

	_ = store.AppendEvent(threadID, ev)
}

func foldThreadMetrics(store *threads.Store, threadID string, metrics *TrialMetrics) {
	_, events, err := store.Load(threadID)
	if err != nil {
		return
	}

	for _, ev := range events {
		switch ev.Type {
		case threads.EventLLMResponse:
			var payload struct {
				Message struct {
					ToolCalls []json.RawMessage `json:"tool_calls"`
				} `json:"message"`
				Usage struct {
					PromptTokens            int `json:"prompt_tokens"`
					CompletionTokens        int `json:"completion_tokens"`
					CompletionTokensDetails struct {
						ReasoningTokens int `json:"reasoning_tokens"`
					} `json:"completion_tokens_details"`
				} `json:"usage"`
			}

			if err := json.Unmarshal(ev.Data, &payload); err == nil {
				metrics.PromptTokens += payload.Usage.PromptTokens
				metrics.CompletionTokens += payload.Usage.CompletionTokens
				metrics.ReasoningTokens += payload.Usage.CompletionTokensDetails.ReasoningTokens
				metrics.ToolCalls += len(payload.Message.ToolCalls)
			}
		case threads.EventToolApproval:
			var payload struct {
				Approved bool `json:"approved"`
			}

			if err := json.Unmarshal(ev.Data, &payload); err == nil && !payload.Approved {
				metrics.ApprovalsDenied++
			}
		case threads.EventToolResult:
			var payload struct {
				Output string `json:"output"`
			}

			if err := json.Unmarshal(ev.Data, &payload); err == nil && strings.HasPrefix(payload.Output, "error: ") {
				metrics.ToolErrors++
			}
		}
	}
}

func finalizeTaskSummary(ts *TaskSummary) {
	if ts.Total == 0 {
		return
	}

	var (
		totalTokens int
		totalTools  int
		totalMS     int64
	)

	for _, tr := range ts.Trials {
		totalTokens += tr.PromptTokens + tr.CompletionTokens
		totalTools += tr.ToolCalls
		totalMS += tr.DurationMS
	}

	n := float64(ts.Total)

	ts.PassRate = float64(ts.Passed) / n
	ts.AvgTokens = float64(totalTokens) / n
	ts.AvgToolCalls = float64(totalTools) / n
	ts.AvgDurationMS = float64(totalMS) / n
}

// LoadOrBuildSummary reads summary.csv from runDir, or lazily reconstructs it from events.jsonl if missing.
func LoadOrBuildSummary(runDir string) (*RunSummary, error) {
	summaryPath := filepath.Join(runDir, summaryFileName)

	summary, err := readSummaryCSV(summaryPath)
	if err == nil {
		return summary, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	return buildSummaryFromEvents(runDir)
}

// buildSummaryFromEvents reconstructs a RunSummary by folding each trial's events.jsonl.
func buildSummaryFromEvents(runDir string) (*RunSummary, error) {
	store := &threads.Store{BaseDir: filepath.Join(runDir, "threads")}

	list, err := store.List("")
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no summary.json or threads found in %s", runDir)
	}

	byTask := make(map[string]*TaskSummary)

	for _, meta := range list {
		metrics := TrialMetrics{ThreadID: meta.ID, Task: meta.Name}
		foldThreadMetrics(store, meta.ID, &metrics)
		applyEvalCheckMetrics(store, meta.ID, &metrics)

		ts, ok := byTask[meta.Name]
		if !ok {
			ts = &TaskSummary{Task: meta.Name}
			byTask[meta.Name] = ts
		}

		ts.Trials = append(ts.Trials, metrics)
		ts.Total++

		if metrics.Passed {
			ts.Passed++
		}
	}

	summary := &RunSummary{
		RunID: filepath.Base(runDir),
		Model: list[0].Model,
	}

	for _, ts := range byTask {
		finalizeTaskSummary(ts)
		summary.Tasks = append(summary.Tasks, *ts)
		summary.Passed += ts.Passed
		summary.Total += ts.Total
	}

	slices.SortFunc(summary.Tasks, func(a, b TaskSummary) int {
		return cmp.Compare(a.Task, b.Task)
	})

	if summary.Total > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.Total)
	}

	return summary, nil
}

// applyEvalCheckMetrics reads the final eval.check event to populate pass/fail and duration.
func applyEvalCheckMetrics(store *threads.Store, threadID string, metrics *TrialMetrics) {
	_, events, err := store.Load(threadID)
	if err != nil {
		return
	}

	for _, ev := range events {
		if ev.Type != threads.EventEvalCheck {
			continue
		}

		var chk CheckData
		if json.Unmarshal(ev.Data, &chk) == nil {
			metrics.RunError = chk.RunError
			metrics.Passed = chk.RunError == "" && chk.Passed
			metrics.CheckExitCode = chk.ExitCode
			metrics.DurationMS = cmp.Or(chk.TrialDurationMS, chk.DurationMS)
		}
	}
}

// FormatTable renders a human-readable summary table for an eval run.
func FormatTable(s *RunSummary) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Eval run %s (model: %s)\n", s.RunID, s.Model)
	fmt.Fprintf(&b, "%-26s %-9s %-10s %-8s %-9s\n", "TASK", "PASS", "AVG TOKENS", "TOOLS", "AVG TIME")

	for _, t := range s.Tasks {
		fmt.Fprintf(&b, "%-26s %2d/%-6d %-10.0f %-8.1f %-9s\n",
			t.Task, t.Passed, t.Total, t.AvgTokens, t.AvgToolCalls,
			(time.Duration(t.AvgDurationMS) * time.Millisecond).Round(time.Millisecond))
	}

	fmt.Fprintf(&b, "Total: %d/%d (%.0f%%) in %s\n",
		s.Passed, s.Total, s.PassRate*100,
		(time.Duration(s.DurationMS) * time.Millisecond).Round(time.Millisecond))

	return b.String()
}

// FormatDiff renders a side-by-side comparison between two RunSummary reports.
func FormatDiff(a, b *RunSummary) string {
	var out strings.Builder

	fmt.Fprintf(&out, "Diff %s -> %s\n", a.RunID, b.RunID)
	fmt.Fprintf(&out, "Total: %d/%d (%.0f%%) -> %d/%d (%.0f%%) (%+.0f%%)\n",
		a.Passed, a.Total, a.PassRate*100,
		b.Passed, b.Total, b.PassRate*100,
		(b.PassRate-a.PassRate)*100)

	byNameA := make(map[string]TaskSummary, len(a.Tasks))

	for _, t := range a.Tasks {
		byNameA[t.Task] = t
	}

	for _, tb := range b.Tasks {
		ta, ok := byNameA[tb.Task]
		if !ok {
			fmt.Fprintf(&out, "  %-26s new: %d/%d\n", tb.Task, tb.Passed, tb.Total)

			continue
		}

		fmt.Fprintf(&out, "  %-26s %d/%d -> %d/%d (tokens %+.0f, tools %+.1f)\n",
			tb.Task, ta.Passed, ta.Total, tb.Passed, tb.Total,
			tb.AvgTokens-ta.AvgTokens, tb.AvgToolCalls-ta.AvgToolCalls)
	}

	return out.String()
}

// writeSummary writes per-trial eval results to summary.csv.
func writeSummary(path string, summary *RunSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	w := csv.NewWriter(f)
	header := []string{
		"run_id", "model", "started_at", "task", "trial", "passed",
		"check_exit_code", "prompt_tokens", "completion_tokens",
		"reasoning_tokens", "tool_calls", "tool_errors",
		"approvals_denied", "duration_ms", "thread_id", "run_error",
	}

	if err := w.Write(header); err != nil {
		_ = f.Close()

		return err
	}

	startedAt := summary.StartedAt.UTC().Format(time.RFC3339)

	for _, ts := range summary.Tasks {
		for _, tr := range ts.Trials {
			row := []string{
				summary.RunID,
				summary.Model,
				startedAt,
				tr.Task,
				strconv.Itoa(tr.Trial),
				strconv.FormatBool(tr.Passed),
				strconv.Itoa(tr.CheckExitCode),
				strconv.Itoa(tr.PromptTokens),
				strconv.Itoa(tr.CompletionTokens),
				strconv.Itoa(tr.ReasoningTokens),
				strconv.Itoa(tr.ToolCalls),
				strconv.Itoa(tr.ToolErrors),
				strconv.Itoa(tr.ApprovalsDenied),
				strconv.FormatInt(tr.DurationMS, 10),
				tr.ThreadID,
				tr.RunError,
			}

			if err := w.Write(row); err != nil {
				_ = f.Close()

				return err
			}
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		_ = f.Close()

		return err
	}

	return f.Close()
}

// readSummaryCSV parses summary.csv back into an aggregated RunSummary.
func readSummaryCSV(path string) (*RunSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", summaryFileName, err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("%s has no trial rows", summaryFileName)
	}

	summary := &RunSummary{
		RunID: rows[1][0],
		Model: rows[1][1],
	}
	if t, err := time.Parse(time.RFC3339, rows[1][2]); err == nil {
		summary.StartedAt = t
	}

	byTask := make(map[string]*TaskSummary)

	for _, row := range rows[1:] {
		if len(row) < 16 {
			return nil, fmt.Errorf("invalid row in %s: expected 16 columns, got %d", summaryFileName, len(row))
		}

		tr := TrialMetrics{
			Task:     row[3],
			ThreadID: row[14],
			RunError: row[15],
		}
		tr.Trial, _ = strconv.Atoi(row[4])
		tr.Passed, _ = strconv.ParseBool(row[5])
		tr.CheckExitCode, _ = strconv.Atoi(row[6])
		tr.PromptTokens, _ = strconv.Atoi(row[7])
		tr.CompletionTokens, _ = strconv.Atoi(row[8])
		tr.ReasoningTokens, _ = strconv.Atoi(row[9])
		tr.ToolCalls, _ = strconv.Atoi(row[10])
		tr.ToolErrors, _ = strconv.Atoi(row[11])
		tr.ApprovalsDenied, _ = strconv.Atoi(row[12])
		tr.DurationMS, _ = strconv.ParseInt(row[13], 10, 64)

		ts, ok := byTask[tr.Task]
		if !ok {
			ts = &TaskSummary{Task: tr.Task}
			byTask[tr.Task] = ts
		}

		ts.Trials = append(ts.Trials, tr)
		ts.Total++

		if tr.Passed {
			ts.Passed++
		}
	}

	for _, ts := range byTask {
		finalizeTaskSummary(ts)
		summary.Tasks = append(summary.Tasks, *ts)
		summary.Passed += ts.Passed
		summary.Total += ts.Total
	}

	slices.SortFunc(summary.Tasks, func(a, b TaskSummary) int {
		return cmp.Compare(a.Task, b.Task)
	})

	if summary.Total > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.Total)
	}

	return summary, nil
}

func newRunID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("2006-01-02_15-04-05"), hex.EncodeToString(b[:])), nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}

		defer in.Close()

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}

		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()

			return err
		}

		return out.Close()
	})
}

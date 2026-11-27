package harness

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/therealshek/enma-harness/config"
	"github.com/therealshek/enma-harness/llm"
	"github.com/therealshek/enma-harness/threads"
	"github.com/therealshek/enma-harness/tools"
)

type threadStartedData struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	ProjectRoot  string `json:"project_root"`
}

type userMessageData struct {
	Content string `json:"content"`
}

type llmResponseData struct {
	Turn       int         `json:"turn"`
	Message    llm.Message `json:"message"`
	Usage      llm.Usage   `json:"usage"`
	DurationMS int64       `json:"duration_ms"`
}

type llmErrorData struct {
	Turn       int    `json:"turn"`
	Error      string `json:"error"`
	DurationMS int64  `json:"duration_ms"`
}

type toolApprovalData struct {
	Turn       int    `json:"turn"`
	ToolCallID string `json:"tool_call_id"`
	Tool       string `json:"tool"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	Tainted    bool   `json:"tainted"`
	Approved   bool   `json:"approved"`
}

type toolResultData struct {
	Turn         int    `json:"turn"`
	ToolCallID   string `json:"tool_call_id"`
	Output       string `json:"output"`
	DurationMS   int64  `json:"duration_ms"`
	TaintedAfter bool   `json:"tainted_after"`
}

// TurnHooks receives live callbacks during Runner.RunPrompt without coupling the loop to a terminal.
type TurnHooks struct {
	OnToken      func(token string, isThinking bool)
	OnReply      func(reply llm.Message, usage llm.Usage, conversation []llm.Message)
	OnToolStart  func(name, rawArgs string)
	OnToolResult func(output string)
	OnError      func(err error)
}

// ChatFunc sends a conversation to a model backend and streams tokens to onToken.
type ChatFunc func(ctx context.Context, messages []llm.Message, onToken func(token string, isThinking bool)) (llm.Message, llm.Usage, error)

// Runner executes the prompt-model-tool loop and records delta events to threads.Store.
type Runner struct {
	Store             *threads.Store
	ProjectRoot       string
	Model             string
	APIURL            string
	ActiveThread      *threads.Metadata
	PendingThreadName string
	MaxTurns          int
	OnWarning         func(msg string)

	eventSeq     int64
	conversation []llm.Message
	toolSession  *tools.Session
	chatFn       ChatFunc
}

// NewRunner creates a Runner initialized with the default system prompt.
func NewRunner(store *threads.Store, projectRoot string, toolSession *tools.Session) *Runner {
	if toolSession == nil {
		toolSession = &tools.Session{ProjectRoot: projectRoot}
	}

	return &Runner{
		Store:        store,
		ProjectRoot:  projectRoot,
		Model:        config.DefaultModel,
		APIURL:       config.ChatCompletionsURL,
		conversation: []llm.Message{{Role: "system", Content: config.DefaultSystemPrompt}},
		toolSession:  toolSession,
	}
}

// Conversation returns the current conversation messages.
func (r *Runner) Conversation() []llm.Message {
	return r.conversation
}

// ToolSession returns the runner's tool session.
func (r *Runner) ToolSession() *tools.Session {
	return r.toolSession
}

// RunPrompt appends a user prompt and runs model/tool turns until the model stops calling tools or MaxTurns is reached.
func (r *Runner) RunPrompt(ctx context.Context, prompt string, hooks TurnHooks) error {
	r.CreateThreadIfNeeded(prompt)
	r.AppendUserMessage(prompt)

	turn := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		turn++
		if r.MaxTurns > 0 && turn > r.MaxTurns {
			return fmt.Errorf("exceeded max turns (%d)", r.MaxTurns)
		}

		chatFn := r.chatFn
		if chatFn == nil {
			chatFn = func(ctx context.Context, messages []llm.Message, onToken func(string, bool)) (llm.Message, llm.Usage, error) {
				return llm.ChatWithEndpoint(ctx, r.APIURL, r.Model, messages, onToken)
			}
		}

		start := time.Now()
		reply, usage, err := chatFn(ctx, r.conversation, hooks.OnToken)
		durationMS := time.Since(start).Milliseconds()

		if err != nil {
			r.recordEvent(threads.EventLLMError, llmErrorData{
				Turn:       turn,
				Error:      err.Error(),
				DurationMS: durationMS,
			})

			if hooks.OnError != nil {
				hooks.OnError(err)
			}

			return err
		}

		r.conversation = append(r.conversation, reply)

		r.recordEvent(threads.EventLLMResponse, llmResponseData{
			Turn:       turn,
			Message:    reply,
			Usage:      usage,
			DurationMS: durationMS,
		})

		if hooks.OnReply != nil {
			hooks.OnReply(reply, usage, r.conversation)
		}

		if len(reply.ToolCalls) == 0 {
			return nil
		}

		for _, tc := range reply.ToolCalls {
			if hooks.OnToolStart != nil {
				hooks.OnToolStart(tc.Function.Name, tc.Function.Arguments)
			}

			toolOutput := r.executeToolCall(turn, tc)

			r.PersistTaintState()

			if hooks.OnToolResult != nil {
				hooks.OnToolResult(toolOutput)
			}

			r.conversation = append(r.conversation, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    toolOutput,
			})
		}
	}
}

func (r *Runner) executeToolCall(turn int, tc llm.ToolCall) string {
	prevOnApproval := r.toolSession.OnApproval
	r.toolSession.OnApproval = func(action, target string, tainted, approved bool) {
		r.recordEvent(threads.EventToolApproval, toolApprovalData{
			Turn:       turn,
			ToolCallID: tc.ID,
			Tool:       tc.Function.Name,
			Action:     action,
			Target:     target,
			Tainted:    tainted,
			Approved:   approved,
		})

		if prevOnApproval != nil {
			prevOnApproval(action, target, tainted, approved)
		}
	}

	defer func() {
		r.toolSession.OnApproval = prevOnApproval
	}()

	start := time.Now()
	output := tools.ExecuteWithSession(r.toolSession, tc.Function.Name, tc.Function.Arguments)
	durationMS := time.Since(start).Milliseconds()

	r.recordEvent(threads.EventToolResult, toolResultData{
		Turn:         turn,
		ToolCallID:   tc.ID,
		Output:       output,
		DurationMS:   durationMS,
		TaintedAfter: r.toolSession.Tainted,
	})

	return output
}

// Reset clears the active conversation and taint state for a fresh thread.
func (r *Runner) Reset(name string) {
	r.ActiveThread = nil
	r.PendingThreadName = name
	r.eventSeq = 0
	r.conversation = []llm.Message{{Role: "system", Content: config.DefaultSystemPrompt}}
	r.toolSession.Tainted = false
}

// LoadThread loads a saved thread from store and folds its event log into conversation state.
func (r *Runner) LoadThread(id string) (*threads.Metadata, error) {
	if r.Store == nil {
		return nil, errors.New("thread store is not configured")
	}

	meta, events, err := r.Store.Load(id)
	if err != nil {
		return nil, err
	}

	loaded, tainted, lastSeq := foldEvents(events)

	r.ActiveThread = meta
	r.PendingThreadName = ""
	r.eventSeq = lastSeq
	r.conversation = loaded
	r.toolSession.Tainted = meta.Tainted || tainted

	return meta, nil
}

// CreateThreadIfNeeded initializes the thread directory on disk on the first real user prompt.
func (r *Runner) CreateThreadIfNeeded(firstPrompt string) {
	if r.Store == nil || r.ActiveThread != nil {
		return
	}

	name := cmp.Or(r.PendingThreadName, threads.NameFromPrompt(firstPrompt))
	model := cmp.Or(r.Model, config.DefaultModel)

	meta, err := r.Store.Create(r.ProjectRoot, model, name, r.toolSession.Tainted)
	if err != nil {
		r.warn("Warning: failed to create thread: " + err.Error())

		return
	}

	r.ActiveThread = meta
	r.PendingThreadName = ""
	r.eventSeq = 0

	systemPrompt := config.DefaultSystemPrompt
	if len(r.conversation) > 0 && r.conversation[0].Role == "system" {
		systemPrompt = r.conversation[0].Content
	}

	r.recordEvent(threads.EventThreadStarted, threadStartedData{
		Model:        meta.Model,
		SystemPrompt: systemPrompt,
		ProjectRoot:  meta.ProjectRoot,
	})
}

// AppendUserMessage appends a user prompt to the conversation and records a user.message event.
func (r *Runner) AppendUserMessage(content string) {
	r.conversation = append(r.conversation, llm.Message{Role: "user", Content: content})
	r.recordEvent(threads.EventUserMessage, userMessageData{Content: content})
}

func (r *Runner) recordEvent(eventType string, payload any) {
	if r.Store == nil || r.ActiveThread == nil {
		return
	}

	r.eventSeq++

	ev, err := threads.NewEvent(r.eventSeq, eventType, payload)
	if err != nil {
		r.warn("Warning: failed to encode event: " + err.Error())

		return
	}

	if err := r.Store.AppendEvent(r.ActiveThread.ID, ev); err != nil {
		r.warn("Warning: failed to save event: " + err.Error())
	}
}

// PersistTaintState writes an updated Tainted flag to meta.json when a session becomes tainted.
func (r *Runner) PersistTaintState() {
	if r.Store == nil || r.ActiveThread == nil {
		return
	}

	if r.toolSession.Tainted && !r.ActiveThread.Tainted {
		r.ActiveThread.Tainted = true

		if err := r.Store.SaveMetadata(r.ActiveThread); err != nil {
			r.warn("Warning: failed to save taint state: " + err.Error())
		}
	}
}

func (r *Runner) warn(msg string) {
	if r.OnWarning != nil {
		r.OnWarning(msg)
	}
}

// foldEvents reconstructs conversation messages, taint state, and the highest sequence number from an event log.
func foldEvents(events []threads.Event) ([]llm.Message, bool, int64) {
	systemPrompt := config.DefaultSystemPrompt
	msgs := make([]llm.Message, 0, len(events)+1)
	tainted := false

	var lastSeq int64

	for _, ev := range events {
		if ev.Sequence > lastSeq {
			lastSeq = ev.Sequence
		}

		switch ev.Type {
		case threads.EventThreadStarted:
			var data threadStartedData
			if err := json.Unmarshal(ev.Data, &data); err == nil && data.SystemPrompt != "" {
				systemPrompt = data.SystemPrompt
			}
		case threads.EventUserMessage:
			var data userMessageData
			if err := json.Unmarshal(ev.Data, &data); err == nil {
				msgs = append(msgs, llm.Message{Role: "user", Content: data.Content})
			}
		case threads.EventLLMResponse:
			var data llmResponseData
			if err := json.Unmarshal(ev.Data, &data); err == nil {
				msg := data.Message
				msg.Role = "assistant"
				msgs = append(msgs, msg)
			}
		case threads.EventToolResult:
			var data toolResultData
			if err := json.Unmarshal(ev.Data, &data); err == nil {
				msgs = append(msgs, llm.Message{
					Role:       "tool",
					ToolCallID: data.ToolCallID,
					Content:    data.Output,
				})
				if data.TaintedAfter {
					tainted = true
				}
			}
		}
	}

	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, llm.Message{Role: "system", Content: systemPrompt})
	out = append(out, msgs...)

	return out, tainted, lastSeq
}

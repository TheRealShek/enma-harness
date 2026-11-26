package config

// Model and API defaults.
const (
	ChatCompletionsURL  = "http://localhost:11434/v1/chat/completions"
	DefaultModel        = "qwen3:4b"
	DefaultSystemPrompt = "Answer directly. Avoid meta commentary, self-evaluation, and unnecessary explanations. Keep internal reasoning concise and minimal. Explain only when asked or when needed for clarity. Keep responses concise but complete."
)

// Context, streaming, and display settings.
const (
	MaxContextTokens         = 8192
	StreamInitialBufferBytes = 64 * 1024
	StreamMaxBufferBytes     = 1024 * 1024
	ToolArgPreviewMaxChars   = 60
	ShowThinkingTraces       = false
	InMemoryOnly             = false
)

// Thread storage settings.
const (
	HarnessDirName = ".enma"
	ThreadsDirName = "threads"
	ThreadsPerPage = 10
)

// Tool limits and untrusted path marker.
const (
	ReadFileMaxLines    = 500
	ReadFileMaxBytes    = 10 * 1024
	ListFilesMaxEntries = 100
	UntrustedDirName    = "untrusted"
)

// Security policy path rules.
// Context, streaming, and display settings.
const (
	MaxContextTokens         = 8192
	StreamInitialBufferBytes = 64 * 1024
	StreamMaxBufferBytes     = 1024 * 1024
	ToolArgPreviewMaxChars   = 60
	ShowThinkingTraces       = false
	InMemoryOnly             = false
)

var (
	// AlwaysSensitive requires approval before writing, even in an untainted session.
	AlwaysSensitive = []string{
		".git", ".vscode", ".zed", ".mcp.json", HarnessDirName,
	}

	// TaintedSensitive requires approval before writing only when the session is tainted.
	TaintedSensitive = []string{"AGENTS.md"}
)

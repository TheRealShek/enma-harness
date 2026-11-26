package config

// Model and API defaults.
const (
	// ChatCompletionsURL is the OpenAI-compatible endpoint for chat requests.
	ChatCompletionsURL = "http://localhost:11434/v1/chat/completions"

	// DefaultModel is the model name sent to the API.
	DefaultModel = "qwen3:4b"

	// DefaultSystemPrompt sets the base instructions for every new conversation.
	DefaultSystemPrompt = "Answer directly. Avoid meta commentary, self-evaluation, and unnecessary explanations. Keep internal reasoning concise and minimal. Explain only when asked or when needed for clarity. Keep responses concise but complete."
)

// Context, streaming, and display limits.
const (
	// MaxContextTokens is the model context window used for context usage tracking.
	MaxContextTokens = 8192

	// StreamInitialBufferBytes is the starting buffer size for reading SSE stream lines.
	StreamInitialBufferBytes = 64 * 1024

	// StreamMaxBufferBytes is the maximum buffer size allowed for a single SSE stream line.
	StreamMaxBufferBytes = 1024 * 1024

	// ToolArgPreviewMaxChars caps how many characters of a tool argument are printed in the CLI.
	ToolArgPreviewMaxChars = 60
)

// Tool output limits.
const (
	// ReadFileMaxLines caps how many lines read_file returns in one call.
	ReadFileMaxLines = 500

	// ReadFileMaxBytes caps how many bytes read_file returns in one call.
	ReadFileMaxBytes = 10 * 1024

	// ListFilesMaxEntries caps how many directory entries list_files returns in one call.
	ListFilesMaxEntries = 100
)

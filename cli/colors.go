package cli

const (
	reset = "\033[0m"

	colorAssistant = "\033[38;5;141m"
	colorUser      = "\033[38;5;75m"
	colorTool      = "\033[38;5;214m"
	colorMuted     = "\033[38;5;242m"
	colorError     = "\033[38;5;203m"
	colorSuccess   = "\033[38;5;114m"
)

func styleAssistant(text string) string {
	return colorAssistant + text + reset
}

func styleUser(text string) string {
	return colorUser + text + reset
}

func styleTool(text string) string {
	return colorTool + text + reset
}

func styleMuted(text string) string {
	return colorMuted + text + reset
}

func styleError(text string) string {
	return colorError + text + reset
}

func styleSuccess(text string) string {
	return colorSuccess + text + reset
}

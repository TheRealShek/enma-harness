package main

const (
	colorReset     = "\033[0m"
	colorUser      = "\033[1;94m" // bold bright blue
	colorAssistant = "\033[1;92m" // bold bright green
	colorTool      = "\033[33m"   // yellow
	colorError     = "\033[31m"   // red
	colorMuted     = "\033[90m"   // dim gray
)

func styleUser(text string) string {
	return colorUser + text + colorReset
}

func styleAssistant(text string) string {
	return colorAssistant + text + colorReset
}

func styleTool(text string) string {
	return colorTool + text + colorReset
}

func styleError(text string) string {
	return colorError + text + colorReset
}

func styleMuted(text string) string {
	return colorMuted + text + colorReset
}

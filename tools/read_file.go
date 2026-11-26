package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/therealshek/enma-harness/config"
)

type ReadFileTool struct{}

type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (r *ReadFileTool) Name() string {
	return "read_file"
}

func (r *ReadFileTool) Description() string {
	return "Read a file."
}

func (r *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path to read.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Start line number. First line is 1.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Limit on how many lines to read (max %d lines).", config.ReadFileMaxLines),
			},
		},
		"required": []string{"path"},
	}
}

// Run reads UTF-8 text from offset up to limit, capped by maxReadLines and maxReadBytes.
func (r *ReadFileTool) Run(rawArgs string) (string, error) {
	var args readFileArgs

	args.Offset = 1 // default offset

	// Parse arguments
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", fmt.Errorf("bad arguments: %w", err)
	}

	rawPath := args.Path
	if strings.TrimSpace(rawPath) == "" {
		return "", fmt.Errorf("missing or invalid path")
	}

	// Resolve path and validate file
	resolvedPath, err := resolvePath(rawPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", rawPath)
		}

		return "", fmt.Errorf("failed to access file: %w", err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", rawPath)
	}

	// Read and validate UTF-8 content
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !utf8.Valid(data) {
		return "", fmt.Errorf("file is not valid UTF-8: %s", rawPath)
	}

	content := string(data)

	var lines []string

	if len(content) > 0 {
		lines = strings.Split(content, "\n")
	}

	totalLines := len(lines)

	// Validate offset and inspect first line
	if args.Offset < 1 {
		return "", fmt.Errorf("invalid offset %d: must be an integer >= 1", args.Offset)
	}

	if args.Offset > totalLines {
		return "", fmt.Errorf("offset %d is past end of file (file has %d lines)", args.Offset, totalLines)
	}

	firstLineIdx := args.Offset - 1
	if len(lines[firstLineIdx]) > config.ReadFileMaxBytes {
		return fmt.Sprintf("Line %d is over %d bytes cap; cannot display whole line.", args.Offset, config.ReadFileMaxBytes), nil
	}

	selected, stopReason := sliceLines(lines[args.Offset-1:], args.Limit)

	out := strings.Join(selected, "\n")
	lineA := args.Offset
	lineB := args.Offset + len(selected) - 1

	switch stopReason {
	case stopCap:
		out += fmt.Sprintf("\n[Showing lines %d-%d of %d]", lineA, lineB, totalLines)
	case stopLimit:
		remaining := totalLines - lineB
		out += fmt.Sprintf("\n[%d more lines in file]", remaining)
	}

	return out, nil
}

type stopReason int

const (
	stopNone stopReason = iota
	stopCap
	stopLimit
)

func sliceLines(lines []string, limit int) ([]string, stopReason) {
	var selected []string

	totalBytes := 0

	for _, line := range lines {
		if limit > 0 && len(selected) >= limit {
			return selected, stopLimit
		}

		if len(selected) >= config.ReadFileMaxLines {
			return selected, stopCap
		}

		newlineBytes := 0
		if len(selected) > 0 {
			newlineBytes = 1
		}

		if totalBytes+len(line)+newlineBytes > config.ReadFileMaxBytes {
			return selected, stopCap
		}

		selected = append(selected, line)
		totalBytes += len(line) + newlineBytes
	}

	return selected, stopNone
}

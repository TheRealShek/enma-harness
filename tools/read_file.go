package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxReadLines = 2000
	maxReadBytes = 50 * 1024
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
	return "Read a file. If the result ends halfway, check the note at the end for the next offset."
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
				"description": "Start line number. First line is 1 (e.g. if you read lines 1 to 5, use offset 6 next).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Limit on how many lines to read (max 2000 lines).",
			},
		},
		"required": []string{"path"},
	}
}

// Run reads UTF-8 text from offset up to limit, capped by maxReadLines and maxReadBytes
// If truncated by either cap or limit, it appends a continuation note with offset.
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
	cleanPath := strings.TrimPrefix(rawPath, "@")
	resolvedPath := filepath.Clean(cleanPath)
	if !filepath.IsAbs(resolvedPath) {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("unable to determine working directory: %w", err)
		}
		resolvedPath = filepath.Join(wd, resolvedPath)
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
	if len(lines[firstLineIdx]) > maxReadBytes {
		return fmt.Sprintf("Line %d is over %d bytes cap; cannot display whole line.", args.Offset, maxReadBytes), nil
	}

	selected, stopReason := sliceLines(lines[args.Offset-1:], args.Limit)

	out := strings.Join(selected, "\n")
	lineA := args.Offset
	lineB := args.Offset + len(selected) - 1

	switch stopReason {
	case stopCap:
		out += fmt.Sprintf("\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", lineA, lineB, totalLines, lineB+1)
	case stopLimit:
		remaining := totalLines - lineB
		out += fmt.Sprintf("\n[%d more lines in file. Use offset=%d to continue.]", remaining, lineB+1)
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
		if len(selected) >= maxReadLines {
			return selected, stopCap
		}

		newlineBytes := 0
		if len(selected) > 0 {
			newlineBytes = 1
		}

		if totalBytes+len(line)+newlineBytes > maxReadBytes {
			return selected, stopCap
		}

		selected = append(selected, line)
		totalBytes += len(line) + newlineBytes
	}

	return selected, stopNone
}

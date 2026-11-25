package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type WriteFileTool struct{}

type writeFileArgs struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite"`
}

func (w *WriteFileTool) Name() string {
	return "write_file"
}

func (w *WriteFileTool) Description() string {
	return "Create a new file with UTF-8 content. Also can Create missing parent folders. Set overwrite: true to replace an existing file."
}

func (w *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "UTF-8 text to write.",
			},
			"overwrite": map[string]any{
				"type":        "boolean",
				"description": "Replaces file if it already exists. Default is false.",
			},
		},
		"required": []string{"path", "content"},
	}
}

// Run creates a new UTF-8 file (and any missing parent directories), or overwrites an existing one when overwrite is true.
func (w *WriteFileTool) Run(rawArgs string) (string, error) {
	var args writeFileArgs

	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", fmt.Errorf("bad arguments: %w", err)
	}

	rawPath := strings.TrimSpace(args.Path)
	if rawPath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	resolvedPath, err := resolvePath(rawPath)
	if err != nil {
		return "", err
	}

	if !utf8.ValidString(args.Content) {
		return "", fmt.Errorf("content is not valid UTF-8")
	}

	perm := os.FileMode(0o644)

	info, err := os.Lstat(resolvedPath)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("path %q is not a regular file", rawPath)
		}

		if !args.Overwrite {
			return "", fmt.Errorf("file %q already exists", rawPath)
		}

		perm = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("failed to access file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := atomicWriteFile(resolvedPath, []byte(args.Content), perm); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %q (%d bytes).", rawPath, len(args.Content)), nil
}

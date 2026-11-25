package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type EditFileTool struct{}

type editFileArgs struct {
	Path        string `json:"path"`
	Target      string `json:"target"`
	Replacement string `json:"replacement"`
}

func (e *EditFileTool) Name() string {
	return "edit_file"
}

func (e *EditFileTool) Description() string {
	return `Replace exact text in an existing file. target must appear once. For empty files, pass target: "". Use empty replacement to delete.`
}

func (e *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path to edit.",
			},
			"target": map[string]any{
				"type":        "string",
				"description": `Exact text to find and replace. For empty files, use "".`,
			},
			"replacement": map[string]any{
				"type":        "string",
				"description": "New text to insert.",
			},
		},
		"required": []string{"path", "target", "replacement"},
	}
}

func (e *EditFileTool) Run(rawArgs string) (string, error) {
	var args editFileArgs

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

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("file %q does not exist", rawPath)
		}

		return "", fmt.Errorf("failed to access file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path %q is not a regular file", rawPath)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !utf8.Valid(data) {
		return "", fmt.Errorf("file %q is not valid UTF-8", rawPath)
	}

	content := string(data)

	if len(content) == 0 {
		if args.Target != "" {
			return "", fmt.Errorf("file %q is empty", rawPath)
		}

		if err := atomicWriteFile(resolvedPath, []byte(args.Replacement), info.Mode().Perm()); err != nil {
			return "", fmt.Errorf("failed to write file: %w", err)
		}

		return fmt.Sprintf("Successfully wrote to empty file %q.", rawPath), nil
	}

	if args.Target == "" {
		return "", fmt.Errorf("file %q is not empty; target must match existing text", rawPath)
	}

	count := strings.Count(content, args.Target)

	if count == 0 {
		if matchesIgnoringWhitespace(content, args.Target) {
			return "", fmt.Errorf("target text exists in %q but indentation or whitespace differed", rawPath)
		}

		return "", fmt.Errorf("target text not found in %q", rawPath)
	}

	if count > 1 {
		return "", fmt.Errorf("target text appears %d times in %q (must appear exactly once)", count, rawPath)
	}

	updated := strings.Replace(content, args.Target, args.Replacement, 1)

	if err := atomicWriteFile(resolvedPath, []byte(updated), info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully replaced 1 occurrence in %q.", rawPath), nil
}

// atomicWriteFile writes data to a temporary file in the same folder, syncs it, and renames it into place.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tempFile, err := os.CreateTemp(dir, ".edit_file-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tempPath := tempFile.Name()

	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()

		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()

		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Chmod(tempPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func normalizeWhitespace(s string) string {
	var b strings.Builder

	inSpace := false

	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')

				inSpace = true
			}
		} else {
			b.WriteRune(r)

			inSpace = false
		}
	}

	return strings.TrimSpace(b.String())
}

func matchesIgnoringWhitespace(content, target string) bool {
	normTarget := normalizeWhitespace(target)

	if len(normTarget) < 5 {
		return false
	}

	return strings.Contains(normalizeWhitespace(content), normTarget)
}

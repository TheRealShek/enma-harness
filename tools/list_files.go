package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ListFilesTool struct{}

type listFilesArgs struct {
	Path string `json:"path"`
}

func (l *ListFilesTool) Name() string {
	return "list_files"
}

func (l *ListFilesTool) Description() string {
	return "List files in a directory. Directories end with /. Use . for the current directory."
}

func (l *ListFilesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list files from.",
			},
		},
		"required": []string{"path"},
	}
}

// Run lists directory contents, adding a trailing slash to subdirectories.
func (l *ListFilesTool) Run(rawArgs string) (string, error) {
	var args listFilesArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", fmt.Errorf("bad arguments: %w", err)
	}

	if strings.TrimSpace(args.Path) == "" {
		return "", fmt.Errorf("missing or invalid path")
	}

	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, e := range entries {
		out.WriteString(e.Name())
		if e.IsDir() {
			out.WriteString("/")
		}
		out.WriteString("\n")
	}
	return out.String(), nil
}

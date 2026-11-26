package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/therealshek/enma-harness/config"
)

type ListFilesTool struct{}

type listFilesArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (l *ListFilesTool) Name() string {
	return "list_files"
}

func (l *ListFilesTool) Description() string {
	return "List files in a folder. Folders end with /."
}

func (l *ListFilesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Folder path. Default is '.' for current folder.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Start item number. First item is 1.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": fmt.Sprintf("Limit on how many items to list (max %d items).", config.ListFilesMaxEntries),
			},
		},
	}
}

// Run lists directory contents, adding a trailing slash to subdirectories.
func (l *ListFilesTool) Run(rawArgs string) (string, error) {
	var args listFilesArgs

	args.Offset = 1

	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return "", fmt.Errorf("bad arguments: %w", err)
	}

	rawPath := strings.TrimSpace(args.Path)
	if rawPath == "" {
		rawPath = "."
	}

	resolvedPath, err := resolvePath(rawPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("directory does not exist: %s", rawPath)
		}

		return "", fmt.Errorf("failed to access directory: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("path is a file, not a directory: %s", rawPath)
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	total := len(entries)

	if args.Offset < 1 {
		return "", fmt.Errorf("invalid offset %d: must be an integer >= 1", args.Offset)
	}

	if total == 0 {
		return "", nil
	}

	if args.Offset > total {
		return "", fmt.Errorf("offset %d is past end of directory (has %d entries)", args.Offset, total)
	}

	firstIdx := args.Offset - 1
	hasLimit := args.Limit > 0
	stoppedByLimit := false
	stoppedBySizeCap := false

	var selected []string

	for i := firstIdx; i < total; i++ {
		if hasLimit && len(selected) >= args.Limit {
			stoppedByLimit = true

			break
		}

		if len(selected) >= config.ListFilesMaxEntries {
			stoppedBySizeCap = true

			break
		}

		e := entries[i]
		name := e.Name()

		if e.IsDir() {
			name += "/"
		}

		selected = append(selected, name)
	}

	out := strings.Join(selected, "\n")
	entryA := args.Offset
	entryB := args.Offset + len(selected) - 1

	if stoppedBySizeCap {
		out += fmt.Sprintf("\n[Showing entries %d-%d of %d]", entryA, entryB, total)
	} else if stoppedByLimit {
		remaining := total - entryB
		out += fmt.Sprintf("\n[%d more entries in directory]", remaining)
	}

	return out, nil
}

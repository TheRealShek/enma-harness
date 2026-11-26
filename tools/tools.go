package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/therealshek/enma-harness/config"
)

// Tool defines the contract for any executable tool exposed to the model.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Run(rawArgs string) (string, error)
}

// Session tracks the project root, taint state, and user approval callback for tool calls.
type Session struct {
	ProjectRoot string
	Tainted     bool
	Approve     func(action, target string, tainted bool) bool
}

// All lists every active tool available in the harness.
var All = []Tool{
	&ReadFileTool{},
	&ListFilesTool{},
	&EditFileTool{},
	&WriteFileTool{},
}

// Specs formats registered tools into the OpenAI function schema expected by the API.
func Specs() []any {
	var out []any

	for _, t := range All {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}

	return out
}

// Execute runs a tool with a default session rooted at the current working directory.
func Execute(name, rawArgs string) string {
	wd, _ := os.Getwd()

	return ExecuteWithSession(&Session{ProjectRoot: wd}, name, rawArgs)
}

// ExecuteWithSession enforces path, symlink, and taint approval gates before running a tool.
func ExecuteWithSession(s *Session, name, rawArgs string) string {
	t := findTool(name)
	if t == nil {
		return "error: unknown tool " + name
	}

	if s == nil {
		wd, _ := os.Getwd()
		s = &Session{ProjectRoot: wd}
	}

	realRoot, err := resolveRealPath(s.ProjectRoot)
	if err != nil {
		realRoot = filepath.Clean(s.ProjectRoot)
	}

	realTarget, hasPath := extractRealPath(rawArgs)

	if hasPath {
		if action, needsApproval := requiredApproval(s.Tainted, name, realRoot, realTarget); needsApproval {
			if !s.requestApproval(action, realTarget) {
				return "error: action denied by user"
			}
		}
	}

	out, err := t.Run(rawArgs)
	if err != nil {
		return "error: " + err.Error()
	}

	if hasPath && isUntrustedPath(realRoot, realTarget) {
		s.Tainted = true

		if name == "read_file" {
			out = fmt.Sprintf("<untrusted_data path=%q>\n%s\n</untrusted_data>", realTarget, out)
		}
	}

	return out
}

func findTool(name string) Tool {
	for _, t := range All {
		if t.Name() == name {
			return t
		}
	}

	return nil
}

func (s *Session) requestApproval(action, target string) bool {
	if s.Approve == nil {
		return false
	}

	return s.Approve(action, target, s.Tainted)
}

func requiredApproval(tainted bool, toolName, realRoot, realTarget string) (string, bool) {
	if isWriteTool(toolName) && isSensitiveMatch(realRoot, realTarget, config.AlwaysSensitive) {
		return "write sensitive repo file", true
	}

	if !tainted {
		return "", false
	}

	if isWriteTool(toolName) {
		if isOutsideRoot(realRoot, realTarget) {
			return "write outside project", true
		}

		if isSensitiveMatch(realRoot, realTarget, config.TaintedSensitive) {
			return "write sensitive file while tainted", true
		}
	}

	if toolName == "read_file" && isOutsideRoot(realRoot, realTarget) {
		return "read file outside project", true
	}

	return "", false
}

func isWriteTool(name string) bool {
	return name == "write_file" || name == "edit_file"
}

func extractRealPath(rawArgs string) (string, bool) {
	var payload struct {
		Path string `json:"path"`
	}

	if err := json.Unmarshal([]byte(rawArgs), &payload); err != nil {
		return "", false
	}

	if strings.TrimSpace(payload.Path) == "" {
		return "", false
	}

	resolved, err := resolvePath(payload.Path)
	if err != nil {
		return "", false
	}

	realPath, err := resolveRealPath(resolved)
	if err != nil {
		return resolved, true
	}

	return realPath, true
}

// resolveRealPath resolves symlinks via the nearest existing ancestor so new or nested paths cannot hide their target.
func resolveRealPath(target string) (string, error) {
	if target == "" {
		return "", nil
	}

	clean := filepath.Clean(target)

	var suffix []string

	curr := clean
	for {
		_, err := os.Lstat(curr)
		if err == nil {
			realBase, err := filepath.EvalSymlinks(curr)
			if err != nil {
				return "", err
			}

			for _, part := range slices.Backward(suffix) {
				realBase = filepath.Join(realBase, part)
			}

			return realBase, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			return clean, nil
		}

		suffix = append(suffix, filepath.Base(curr))
		curr = parent
	}
}

func isOutsideRoot(root, target string) bool {
	if root == "" {
		return false
	}

	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return true
	}

	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isSensitiveMatch(root, target string, names []string) bool {
	if root == "" || isOutsideRoot(root, target) {
		return false
	}

	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}

	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		if slices.Contains(names, part) || (slices.Contains(names, ".env") && strings.HasPrefix(part, ".env.")) {
			return true
		}
	}

	return false
}

func isUntrustedPath(root, target string) bool {
	untrustedDir := config.UntrustedDirName
	cleanTarget := filepath.Clean(target)

	if root != "" {
		untrustedRoot := filepath.Join(filepath.Clean(root), untrustedDir)
		if cleanTarget == untrustedRoot || strings.HasPrefix(cleanTarget, untrustedRoot+string(filepath.Separator)) {
			return true
		}
	}

	for part := range strings.SplitSeq(cleanTarget, string(filepath.Separator)) {
		if part == untrustedDir {
			return true
		}
	}

	return false
}

// resolvePath strips any @ prefix and turns relative paths into absolute paths from the working directory.
func resolvePath(rawPath string) (string, error) {
	cleanPath := strings.TrimPrefix(strings.TrimSpace(rawPath), "@")
	resolvedPath := filepath.Clean(cleanPath)

	if !filepath.IsAbs(resolvedPath) {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("unable to determine working directory: %w", err)
		}

		resolvedPath = filepath.Join(wd, resolvedPath)
	}

	return resolvedPath, nil
}

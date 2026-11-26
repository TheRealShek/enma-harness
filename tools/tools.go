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

// Session tracks the project root, taint state, and approval callbacks for tool calls.
type Session struct {
	ProjectRoot string
	Tainted     bool
	Approve     func(action, target string, tainted bool) bool
	OnApproval  func(action, target string, tainted, approved bool)
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

	realTarget, normalizedArgs, hasPath := prepareToolPath(s.ProjectRoot, name, rawArgs)

	if hasPath {
		if action, needsApproval := requiredApproval(s.Tainted, name, realRoot, realTarget); needsApproval {
			approved := s.requestApproval(action, realTarget)
			if s.OnApproval != nil {
				s.OnApproval(action, realTarget, s.Tainted, approved)
			}

			if !approved {
				return "error: action denied by user"
			}
		}
	}

	out, err := t.Run(normalizedArgs)
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
	if isWriteTool(toolName) {
		if isHarnessStatePath(realTarget) {
			return "write harness state directory", true
		}

		if matchesSensitivePath(realRoot, realTarget, config.AlwaysSensitive) {
			return "write sensitive repo file", true
		}
	}

	if !tainted {
		return "", false
	}

	if isWriteTool(toolName) {
		if isOutsideProjectRoot(realRoot, realTarget) {
			return "write outside project", true
		}

		if matchesSensitivePath(realRoot, realTarget, config.TaintedSensitive) {
			return "write sensitive file while tainted", true
		}
	}

	if toolName == "read_file" && isOutsideProjectRoot(realRoot, realTarget) {
		return "read file outside project", true
	}

	return "", false
}

func isWriteTool(name string) bool {
	return name == "write_file" || name == "edit_file"
}

// prepareToolPath resolves any path argument against projectRoot and rewrites relative paths in rawArgs to absolute paths.
func prepareToolPath(projectRoot, toolName, rawArgs string) (string, string, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawArgs), &payload); err != nil {
		return "", rawArgs, false
	}

	if payload == nil {
		payload = make(map[string]any)
	}

	rawPath, _ := payload["path"].(string)
	trimmed := strings.TrimSpace(rawPath)

	if trimmed == "" {
		if toolName != "list_files" {
			return "", rawArgs, false
		}

		trimmed = "."
	}

	resolved, err := resolvePathInRoot(projectRoot, trimmed)
	if err != nil {
		return "", rawArgs, false
	}

	realPath, err := resolveRealPath(resolved)
	if err != nil {
		realPath = resolved
	}

	if projectRoot != "" {
		payload["path"] = resolved
		if encoded, err := json.Marshal(payload); err == nil {
			rawArgs = string(encoded)
		}
	}

	return realPath, rawArgs, true
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

func isOutsideProjectRoot(root, target string) bool {
	if root == "" {
		return false
	}

	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return true
	}

	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func matchesSensitivePath(root, target string, names []string) bool {
	if root == "" || isOutsideProjectRoot(root, target) {
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

func isHarnessStatePath(realTarget string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}

	stateRoot := filepath.Join(filepath.Clean(home), config.HarnessDirName)
	if realState, err := resolveRealPath(stateRoot); err == nil {
		stateRoot = realState
	}

	cleanTarget := filepath.Clean(realTarget)

	return cleanTarget == stateRoot || strings.HasPrefix(cleanTarget, stateRoot+string(filepath.Separator))
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
	return resolvePathInRoot("", rawPath)
}

func resolvePathInRoot(root, rawPath string) (string, error) {
	cleanPath := strings.TrimPrefix(strings.TrimSpace(rawPath), "@")
	resolvedPath := filepath.Clean(cleanPath)

	if !filepath.IsAbs(resolvedPath) {
		base := strings.TrimSpace(root)
		if base == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("unable to determine working directory: %w", err)
			}

			base = wd
		}

		resolvedPath = filepath.Join(base, resolvedPath)
	}

	return resolvedPath, nil
}

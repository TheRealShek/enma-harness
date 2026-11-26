package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteWithSessionSecurityGates(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	untrustedDir := filepath.Join(root, "untrusted")

	if err := os.MkdirAll(untrustedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	untrustedFile := filepath.Join(untrustedDir, "page.txt")
	if err := os.WriteFile(untrustedFile, []byte("line1\nline2"), 0o644); err != nil {
		t.Fatal(err)
	}

	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("reading from untrusted taints session and wraps slices", func(t *testing.T) {
		s := &Session{ProjectRoot: root}

		out := ExecuteWithSession(s, "read_file", mustJSON(t, map[string]any{
			"path":   untrustedFile,
			"offset": 2,
			"limit":  1,
		}))

		if !s.Tainted {
			t.Fatal("expected session to be tainted")
		}

		if !strings.HasPrefix(out, "<untrusted_data ") || !strings.Contains(out, "line2") {
			t.Errorf("expected wrapped untrusted output, got %q", out)
		}
	})

	t.Run("always gates sensitive repo paths even when untainted", func(t *testing.T) {
		mcpPath := filepath.Join(root, ".mcp.json")
		prompted := false

		s := &Session{
			ProjectRoot: root,
			Tainted:     false,
			Approve: func(action, target string, tainted bool) bool {
				prompted = true

				return false
			},
		}

		out := ExecuteWithSession(s, "write_file", mustJSON(t, map[string]any{
			"path":    mcpPath,
			"content": `{"mcpServers":{}}`,
		}))
		if !prompted || !strings.HasPrefix(out, "error: action denied") {
			t.Fatalf("expected approval prompt and denial, prompted=%v out=%q", prompted, out)
		}
	})

	t.Run("tainted read outside project requires approval", func(t *testing.T) {
		cleanSession := &Session{ProjectRoot: root, Tainted: false}
		if out := ExecuteWithSession(cleanSession, "read_file", mustJSON(t, map[string]any{"path": outsideFile})); out != "secret" {
			t.Fatalf("untainted read should succeed, got %q", out)
		}

		var (
			gotTainted bool
			approved   bool
		)

		taintedSession := &Session{
			ProjectRoot: root,
			Tainted:     true,
			Approve: func(action, target string, tainted bool) bool {
				gotTainted = tainted

				return approved
			},
		}

		denied := ExecuteWithSession(taintedSession, "read_file", mustJSON(t, map[string]any{"path": outsideFile}))
		if !gotTainted || !strings.HasPrefix(denied, "error: action denied") {
			t.Fatalf("expected tainted denial, gotTainted=%v out=%q", gotTainted, denied)
		}

		approved = true

		if okOut := ExecuteWithSession(taintedSession, "read_file", mustJSON(t, map[string]any{"path": outsideFile})); okOut != "secret" {
			t.Fatalf("expected approved read to succeed, got %q", okOut)
		}
	})

	t.Run("symlink escaping project root is caught by approval gate", func(t *testing.T) {
		linkDir := filepath.Join(root, "escape")
		if err := os.Symlink(outside, linkDir); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}

		prompted := false
		s := &Session{
			ProjectRoot: root,
			Tainted:     true,
			Approve: func(action, target string, tainted bool) bool {
				prompted = true

				return false
			},
		}

		out := ExecuteWithSession(s, "write_file", mustJSON(t, map[string]any{
			"path":    filepath.Join(linkDir, "pwn.txt"),
			"content": "bad",
		}))
		if !prompted || !strings.HasPrefix(out, "error: action denied") {
			t.Fatalf("expected symlink escape to be gated, prompted=%v out=%q", prompted, out)
		}
	})

	t.Run("tainted sensitive file write inside project requires approval", func(t *testing.T) {
		agentsFile := filepath.Join(root, "AGENTS.md")
		prompted := false

		s := &Session{
			ProjectRoot: root,
			Tainted:     true,
			Approve: func(action, target string, tainted bool) bool {
				prompted = true

				return true
			},
		}

		out := ExecuteWithSession(s, "write_file", mustJSON(t, map[string]any{
			"path":    agentsFile,
			"content": "# rules\n",
		}))
		if !prompted || !strings.Contains(out, "Successfully wrote") {
			t.Fatalf("expected approval prompt and write success, prompted=%v out=%q", prompted, out)
		}
	})
}

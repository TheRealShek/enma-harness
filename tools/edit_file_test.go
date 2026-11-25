package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	return string(data)
}

func TestEditFileRun(t *testing.T) {
	tool := &EditFileTool{}

	t.Run("replaces single occurrence and preserves permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.txt")

		if err := os.WriteFile(path, []byte("hello world\n"), 0o640); err != nil {
			t.Fatal(err)
		}

		args := mustJSON(t, editFileArgs{
			Path:        path,
			Target:      "world",
			Replacement: "gopher",
		})

		out, err := tool.Run(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "Successfully replaced 1 occurrence") {
			t.Errorf("unexpected output: %q", out)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "hello gopher\n" {
			t.Errorf("file content = %q, want %q", string(got), "hello gopher\n")
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o640 {
			t.Errorf("file perm = %o, want 0640", info.Mode().Perm())
		}
	})

	t.Run("deletes target with empty replacement", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "delete.txt")

		if err := os.WriteFile(path, []byte("keep remove keep"), 0o644); err != nil {
			t.Fatal(err)
		}

		args := mustJSON(t, editFileArgs{
			Path:        path,
			Target:      " remove",
			Replacement: "",
		})

		if _, err := tool.Run(args); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "keep keep" {
			t.Errorf("file content = %q, want %q", string(got), "keep keep")
		}
	})

	t.Run("writes to empty file when target is empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")

		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}

		args := mustJSON(t, editFileArgs{
			Path:        path,
			Target:      "",
			Replacement: "initial content",
		})

		out, err := tool.Run(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "Successfully wrote to empty file") {
			t.Errorf("unexpected output: %q", out)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "initial content" {
			t.Errorf("file content = %q, want %q", string(got), "initial content")
		}
	})

	t.Run("resolves @ prefix and relative path", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		if err := os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("alpha"), 0o644); err != nil {
			t.Fatal(err)
		}

		args := mustJSON(t, editFileArgs{
			Path:        "@rel.txt",
			Target:      "alpha",
			Replacement: "beta",
		})

		if _, err := tool.Run(args); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(dir, "rel.txt"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "beta" {
			t.Errorf("file content = %q, want %q", string(got), "beta")
		}
	})

	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		emptyPath := filepath.Join(dir, "empty.txt")
		multiPath := filepath.Join(dir, "multi.txt")
		indentPath := filepath.Join(dir, "indent.txt")
		binPath := filepath.Join(dir, "bin.dat")

		for _, f := range []struct {
			path string
			data []byte
		}{
			{emptyPath, nil},
			{multiPath, []byte("foo bar foo")},
			{indentPath, []byte("func main() {\n\treturn\n}")},
			{binPath, []byte{0xff, 0xfe, 0x00}},
		} {
			if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
				t.Fatal(err)
			}
		}

		cases := []struct {
			name    string
			rawArgs string
			wantErr string
		}{
			{
				name:    "bad json",
				rawArgs: "{",
				wantErr: "bad arguments",
			},
			{
				name:    "empty path",
				rawArgs: mustJSON(t, editFileArgs{Path: "  "}),
				wantErr: "path cannot be empty",
			},
			{
				name:    "missing file",
				rawArgs: mustJSON(t, editFileArgs{Path: filepath.Join(dir, "missing.txt"), Target: "a"}),
				wantErr: "does not exist",
			},
			{
				name:    "directory path",
				rawArgs: mustJSON(t, editFileArgs{Path: dir, Target: "a"}),
				wantErr: "not a regular file",
			},
			{
				name:    "invalid utf8",
				rawArgs: mustJSON(t, editFileArgs{Path: binPath, Target: "a"}),
				wantErr: "not valid UTF-8",
			},
			{
				name:    "empty file with non-empty target",
				rawArgs: mustJSON(t, editFileArgs{Path: emptyPath, Target: "something"}),
				wantErr: "is empty",
			},
			{
				name:    "non-empty file with empty target",
				rawArgs: mustJSON(t, editFileArgs{Path: multiPath, Target: ""}),
				wantErr: "is not empty",
			},
			{
				name:    "target not found",
				rawArgs: mustJSON(t, editFileArgs{Path: multiPath, Target: "missing"}),
				wantErr: "not found",
			},
			{
				name:    "whitespace mismatch",
				rawArgs: mustJSON(t, editFileArgs{Path: indentPath, Target: "func main() { return }"}),
				wantErr: "indentation or whitespace differed",
			},
			{
				name:    "duplicate target",
				rawArgs: mustJSON(t, editFileArgs{Path: multiPath, Target: "foo"}),
				wantErr: "appears 2 times",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := tool.Run(tc.rawArgs)
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
			})
		}
	})
}

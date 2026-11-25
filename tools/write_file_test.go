package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileRun(t *testing.T) {
	tool := &WriteFileTool{}

	t.Run("creates new file and missing parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "sub", "new.txt")

		out, err := tool.Run(mustJSON(t, writeFileArgs{
			Path:    path,
			Content: "hello\n",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "Successfully wrote") {
			t.Errorf("unexpected output: %q", out)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "hello\n" {
			t.Errorf("content = %q, want %q", string(got), "hello\n")
		}
	})

	t.Run("resolves @ prefix and relative path", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		if _, err := tool.Run(mustJSON(t, writeFileArgs{
			Path:    "@rel/file.txt",
			Content: "ok",
		})); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(dir, "rel", "file.txt"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "ok" {
			t.Errorf("content = %q, want %q", string(got), "ok")
		}
	})

	t.Run("overwrites existing file only when overwrite is true and preserves permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "existing.txt")

		if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
			t.Fatal(err)
		}

		if _, err := tool.Run(mustJSON(t, writeFileArgs{
			Path:    path,
			Content: "new",
		})); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("expected already exists error, got %v", err)
		}

		if _, err := tool.Run(mustJSON(t, writeFileArgs{
			Path:      path,
			Content:   "new",
			Overwrite: true,
		})); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "new" {
			t.Errorf("content = %q, want %q", string(got), "new")
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o640 {
			t.Errorf("perm = %o, want 0640", info.Mode().Perm())
		}
	})

	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")

		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
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
				rawArgs: mustJSON(t, writeFileArgs{Path: " ", Content: "x"}),
				wantErr: "path cannot be empty",
			},
			{
				name:    "directory target",
				rawArgs: mustJSON(t, writeFileArgs{Path: dir, Content: "x", Overwrite: true}),
				wantErr: "not a regular file",
			},
			{
				name:    "parent is a file",
				rawArgs: mustJSON(t, writeFileArgs{Path: filepath.Join(blocker, "child.txt"), Content: "x"}),
				wantErr: "not a directory",
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

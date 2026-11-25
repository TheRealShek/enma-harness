package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFilesRun(t *testing.T) {
	tool := &ListFilesTool{}

	t.Run("lists files and appends slash to directories", func(t *testing.T) {
		dir := t.TempDir()

		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "a.go"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "z.txt"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": dir}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "a.go\nsub/\nz.txt"
		if out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
	})

	t.Run("defaults to current directory and strips @ prefix", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		if err := os.WriteFile(filepath.Join(dir, "item.txt"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		for _, rawArgs := range []string{"{}", mustJSON(t, map[string]any{"path": "@."})} {
			out, err := tool.Run(rawArgs)
			if err != nil {
				t.Fatalf("args %q: unexpected error: %v", rawArgs, err)
			}

			if out != "item.txt" {
				t.Errorf("args %q: output = %q, want %q", rawArgs, out, "item.txt")
			}
		}
	})

	t.Run("returns empty string for empty directory", func(t *testing.T) {
		dir := t.TempDir()

		out, err := tool.Run(mustJSON(t, map[string]any{"path": dir}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if out != "" {
			t.Errorf("output = %q, want empty", out)
		}
	})

	t.Run("paginates with offset and limit", func(t *testing.T) {
		dir := t.TempDir()

		for _, name := range []string{"f1", "f2", "f3", "f4"} {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": dir, "offset": 2, "limit": 2}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "f2\nf3\n[1 more entries in directory]"
		if out != want {
			t.Errorf("output = %q, want %q", out, want)
		}
	})

	t.Run("caps output at maxListEntries", func(t *testing.T) {
		dir := t.TempDir()

		total := maxListEntries + 5
		for i := range total {
			name := fmt.Sprintf("file-%03d", i)
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": dir}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantFooter := fmt.Sprintf("[Showing entries 1-%d of %d]", maxListEntries, total)
		if !strings.HasSuffix(out, wantFooter) {
			t.Errorf("output should end with %q, got tail %q", wantFooter, out[len(out)-50:])
		}
	})

	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "file.txt")

		if err := os.WriteFile(filePath, nil, 0o644); err != nil {
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
				name:    "missing directory",
				rawArgs: mustJSON(t, listFilesArgs{Path: filepath.Join(dir, "missing")}),
				wantErr: "does not exist",
			},
			{
				name:    "file instead of directory",
				rawArgs: mustJSON(t, listFilesArgs{Path: filePath}),
				wantErr: "is a file, not a directory",
			},
			{
				name:    "offset below 1",
				rawArgs: mustJSON(t, listFilesArgs{Path: dir, Offset: -1}),
				wantErr: "invalid offset",
			},
			{
				name:    "offset past end",
				rawArgs: mustJSON(t, listFilesArgs{Path: dir, Offset: 2}),
				wantErr: "past end of directory",
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

package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/therealshek/enma-harness/config"
)

func TestReadFileRun(t *testing.T) {
	tool := &ReadFileTool{}

	t.Run("reads whole file and slices with offset and limit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "lines.txt")

		if err := os.WriteFile(path, []byte("l1\nl2\nl3\nl4\nl5"), 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": path}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if out != "l1\nl2\nl3\nl4\nl5" {
			t.Errorf("full read = %q, want %q", out, "l1\nl2\nl3\nl4\nl5")
		}

		sliced, err := tool.Run(mustJSON(t, map[string]any{"path": path, "offset": 2, "limit": 2}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "l2\nl3\n[2 more lines in file]"
		if sliced != want {
			t.Errorf("sliced = %q, want %q", sliced, want)
		}
	})

	t.Run("caps output at maxReadLines", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "many.txt")

		lines := make([]string, config.ReadFileMaxLines+5)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i+1)
		}

		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": path}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantFooter := fmt.Sprintf("[Showing lines 1-%d of %d]", config.ReadFileMaxLines, config.ReadFileMaxLines+5)
		if !strings.HasSuffix(out, wantFooter) {
			t.Errorf("output should end with %q, got tail %q", wantFooter, out[len(out)-60:])
		}
	})

	t.Run("caps output at maxReadBytes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bytes.txt")

		chunk := strings.Repeat("a", 4000)
		content := strings.Join([]string{chunk, chunk, chunk}, "\n")

		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": path}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasSuffix(out, "[Showing lines 1-2 of 3]") {
			t.Errorf("unexpected byte-cap output suffix: %q", out[len(out)-40:])
		}
	})

	t.Run("reports single line exceeding maxReadBytes", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "longline.txt")

		if err := os.WriteFile(path, []byte(strings.Repeat("x", config.ReadFileMaxBytes+1)), 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": path}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(out, "over 10240 bytes cap") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("resolves @ prefix and relative path", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := tool.Run(mustJSON(t, map[string]any{"path": "@note.txt"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if out != "hello" {
			t.Errorf("output = %q, want %q", out, "hello")
		}
	})

	t.Run("errors", func(t *testing.T) {
		dir := t.TempDir()
		emptyPath := filepath.Join(dir, "empty.txt")
		smallPath := filepath.Join(dir, "small.txt")
		binPath := filepath.Join(dir, "bin.dat")

		if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(smallPath, []byte("one\ntwo"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(binPath, []byte{0xff, 0xfe}, 0o644); err != nil {
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
				rawArgs: mustJSON(t, readFileArgs{Path: " "}),
				wantErr: "missing or invalid path",
			},
			{
				name:    "missing file",
				rawArgs: mustJSON(t, readFileArgs{Path: filepath.Join(dir, "nope.txt")}),
				wantErr: "does not exist",
			},
			{
				name:    "directory",
				rawArgs: mustJSON(t, readFileArgs{Path: dir}),
				wantErr: "is a directory",
			},
			{
				name:    "invalid utf8",
				rawArgs: mustJSON(t, readFileArgs{Path: binPath}),
				wantErr: "not valid UTF-8",
			},
			{
				name:    "offset below 1",
				rawArgs: mustJSON(t, readFileArgs{Path: smallPath, Offset: -1}),
				wantErr: "invalid offset",
			},
			{
				name:    "offset past end",
				rawArgs: mustJSON(t, readFileArgs{Path: smallPath, Offset: 3}),
				wantErr: "past end of file",
			},
			{
				name:    "empty file",
				rawArgs: mustJSON(t, map[string]any{"path": emptyPath}),
				wantErr: "past end of file (file has 0 lines)",
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

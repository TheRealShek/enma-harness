package main

import (
	"encoding/json"
	"os"
	"strings"
)

// tools registers local executable functions available to the model
var tools = []Tool{
	{
		Name:  "read_file",
		Desc:  "Read the contents of a file at a relative path.",
		Props: []string{"path"},
		Run:   readFile,
	},
	{
		Name:  "list_files",
		Desc:  "List files in a directory. Directories end with /. Use . for the current directory.",
		Props: []string{"path"},
		Run:   listFiles,
	},
}

// specs transforms local tool definitions into OpenAI-compatible func
func specs() []any {
	out := []any{}
	for _, t := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Desc,
				"parameters":  schema(t.Props),
			},
		})
	}
	return out
}

// schema converts parameter names into a JSON Schema object
func schema(props []string) map[string]any {
	p := map[string]any{}
	for _, k := range props {
		p[k] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": p, "required": props}
}

func execute(name, raw string) string {
	for _, t := range tools {
		if t.Name != name {
			continue
		}
		args := map[string]string{}
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return "error: bad arguments: " + err.Error()
		}
		out, err := t.Run(args)
		if err != nil {
			return "error: " + err.Error()
		}
		return out
	}
	return "error: unknown tool " + name
}

func readFile(a map[string]string) (string, error) {
	b, err := os.ReadFile(a["path"])
	return string(b), err
}

func listFiles(a map[string]string) (string, error) {
	entries, err := os.ReadDir(a["path"])
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

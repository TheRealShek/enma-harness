package main

import "os"

var tools = []Tool{
	{
		Name:  "read_file",
		Desc:  "Read the contents of a file at a relative path",
		Props: []string{"path"},
		Run: func(a map[string]string) (string, error) {
			b, err := os.ReadFile(a["path"])
			return string(b), err
		},
	},
}

func schema(props []string) map[string]any {
	p := map[string]any{}
	for _, k := range props {
		p[k] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": p, "required": props}
}

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

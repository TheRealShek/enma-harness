package tools

// Tool defines the contract for any executable tool exposed to the model.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Run(rawArgs string) (string, error)
}

// All lists every active tool available in the harness.
var All = []Tool{
	&ReadFileTool{},
	&ListFilesTool{},
	&EditFileTool{},
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

// Execute finds the matching tool by name and runs it with the model's raw JSON arguments.
func Execute(name, rawArgs string) string {
	for _, t := range All {
		if t.Name() == name {
			out, err := t.Run(rawArgs)
			if err != nil {
				return "error: " + err.Error()
			}

			return out
		}
	}

	return "error: unknown tool " + name
}

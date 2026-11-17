package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

const (
	url               = "http://localhost:11434/v1/chat/completions"
	model             = "qwen3:4b"
	sys_prompt string = "Answer directly. Avoid meta commentary, self-evaluation, and unnecessary explanations. Explain only when asked or when needed for clarity. Keep responses concise but complete."
)

func main() {
	conv := []Message{{Role: "system", Content: sys_prompt}}
	sc := bufio.NewScanner(os.Stdin)
	needInput := true

	for {
		if needInput {
			fmt.Print("\033[94mYou: ")
			if !sc.Scan() {
				break
			}
			conv = append(conv, Message{Role: "user", Content: sc.Text()})

		}

		reply, err := chat(conv)
		if err != nil {
			fmt.Println("Error:", err)
		}
		conv = append(conv, reply)

		if reply.Content != "" {
			fmt.Println("\033[92mQwen:", reply.Content, "\033[0m")
		}
		if len(reply.ToolCall) == 0 {
			break
		}
		fmt.Printf("DEBUG calls=%d content=%q\n", len(reply.ToolCall), reply.Content)
		needInput = len(reply.ToolCall) == 0
		for _, tc := range reply.ToolCall {
			fmt.Printf("\033[93mtool: %s(%s)\033[0m\n", tc.Function.Name, tc.Function.Arguments)
			args := map[string]string{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			out, err := tools[0].Run(args)
			if err != nil {
				out = "error: " + err.Error()
			}
			conv = append(conv, Message{Role: "tool", ToolCallID: tc.ID, Content: out})

		}
	}

	if err := sc.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

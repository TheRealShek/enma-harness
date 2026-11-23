package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/therealshek/enma-harness/tools"
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
			needInput = true
			if sc.Text() == "" {
				continue
			}
		}
		conv = append(conv, reply)

		if reply.Content != "" {
			fmt.Println("\033[92mQwen:", reply.Content, "\033[0m")
		}

		needInput = len(reply.ToolCalls) == 0
		for _, tc := range reply.ToolCalls {
			fmt.Printf("\033[93mtool: %s(%s)\033[0m\n", tc.Function.Name, tc.Function.Arguments)
			out := tools.Execute(tc.Function.Name, tc.Function.Arguments)
			conv = append(conv, Message{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}

	if err := sc.Err(); err != nil {
		fmt.Println("Input error:", err)
	}
}

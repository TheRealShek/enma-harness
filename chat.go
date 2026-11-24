package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/therealshek/enma-harness/tools"
)

func chat(messages []Message) (Message, Usage, error) {
	body, err := json.Marshal(Request{model, messages, tools.Specs()})
	if err != nil {
		return Message{}, Usage{}, err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return Message{}, Usage{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, Usage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Message{}, Usage{}, fmt.Errorf("status %d: %s", resp.StatusCode, data)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Message{}, Usage{}, err
	}
	if len(out.Choices) == 0 {
		return Message{}, Usage{}, fmt.Errorf("no choices: %s", data)
	}
	return out.Choices[0].Message, out.Usage, nil
}

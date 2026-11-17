package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func chat(messages []Message) (Message, error) {
	body, err := json.Marshal(Request{model, messages, specs()})
	if err != nil {
		return Message{}, err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("status %d: %s", resp.StatusCode, data)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return Message{}, err
	}
	if len(out.Choices) == 0 {
		return Message{}, fmt.Errorf("no choices: %s", data)
	}
	return out.Choices[0].Message, nil
}

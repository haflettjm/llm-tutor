package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const maxStreamLine = 8 << 20

// scanStreamJSON reads the Claude CLI's NDJSON output, emitting structured-output
// JSON fragments and returning the final result envelope for parseClaudeOutput.
func scanStreamJSON(r io.Reader, emit func(string) error) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)

	var final []byte
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var event struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type        string `json:"type"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if event.Type == "result" {
			final = append(final[:0], line...)
			continue
		}
		if event.Type != "stream_event" || event.Event.Type != "content_block_delta" || event.Event.Delta.Type != "input_json_delta" {
			continue
		}
		if err := emit(event.Event.Delta.PartialJSON); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream-json: %w", err)
	}
	return final, nil
}

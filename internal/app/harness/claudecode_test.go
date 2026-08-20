package harness

import "testing"

func TestParseClaudeOutputIgnoresLauncherNoise(t *testing.T) {
	raw := []byte("mise activated claude@2.1.234\n" +
		`{"type":"result","subtype":"success","result":"{\"message\":\"What do you predict?\",\"response_type\":\"question\",\"hint_level\":0}"}`)

	resp, err := parseClaudeOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message != "What do you predict?" {
		t.Fatalf("message = %q", resp.Message)
	}
}

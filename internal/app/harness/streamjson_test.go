package harness

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestScanStreamJSONCollectsPartialJSONAndFinalEnvelope(t *testing.T) {
	const input = `{"type":"system","subtype":"init"}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"message\":\"Hello "}}}
{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"world\"}"}}}
{"type":"result","subtype":"success","result":"{\"message\":\"Hello world\",\"response_type\":\"question\",\"hint_level\":0}"}
`
	var got strings.Builder
	final, err := scanStreamJSON(strings.NewReader(input), func(fragment string) error {
		got.WriteString(fragment)
		return nil
	})
	if err != nil {
		t.Fatalf("scanStreamJSON: %v", err)
	}
	if got.String() != `{"message":"Hello world"}` {
		t.Errorf("fragments = %q", got.String())
	}
	if !bytes.Contains(final, []byte(`"type":"result"`)) {
		t.Errorf("final envelope = %q, want result envelope", final)
	}
}

func TestScanStreamJSONHandlesVeryLongLines(t *testing.T) {
	long := strings.Repeat("x", 200_000)
	input := `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"` + long + `"}}}` + "\n" +
		`{"type":"result","subtype":"success","result":"{\"message\":\"ok\",\"response_type\":\"question\",\"hint_level\":0}"}` + "\n"

	var n int
	if _, err := scanStreamJSON(strings.NewReader(input), func(fragment string) error {
		n += len(fragment)
		return nil
	}); err != nil {
		t.Fatalf("scanStreamJSON: %v", err)
	}
	if n != len(long) {
		t.Errorf("got %d bytes, want %d", n, len(long))
	}
}

func TestScanStreamJSONPropagatesEmitError(t *testing.T) {
	const input = `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"message\":\"a"}}}` + "\n"
	want := errors.New("client gone")
	_, err := scanStreamJSON(strings.NewReader(input), func(string) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestScanStreamJSONIgnoresUnknownEvents(t *testing.T) {
	const input = `{"type":"some_future_event","payload":{"nested":true}}
{"type":"result","subtype":"success","result":"{\"message\":\"ok\",\"response_type\":\"question\",\"hint_level\":0}"}
`
	if _, err := scanStreamJSON(strings.NewReader(input), func(string) error { return nil }); err != nil {
		t.Fatalf("unknown event aborted the scan: %v", err)
	}
}

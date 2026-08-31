package tutor

import (
	"context"
	"strings"
	"testing"

	"github.com/haflettjm/llm-tutor/internal/app/harness"
	"github.com/haflettjm/llm-tutor/internal/types/request"
	"github.com/haflettjm/llm-tutor/internal/types/response"
)

type fakeStreamingHarness struct {
	fakeHarness
	chunks []string
}

func (f *fakeStreamingHarness) StreamQuery(_ context.Context, req request.Request, emit func(harness.StreamChunk) error) (response.Response, error) {
	f.lastReq = req
	for _, text := range f.chunks {
		if err := emit(harness.StreamChunk{Text: text}); err != nil {
			return response.Response{}, err
		}
	}
	return f.resp, nil
}

func TestHandleStreamFallsBackToQuery(t *testing.T) {
	tut, h := newFixture(t, allSouls(), demonstrated(3))
	h.resp = response.Response{Message: "whole reply"}
	var got strings.Builder
	resp, err := tut.HandleStream(context.Background(), request.Request{Message: "hi", SessionID: "s"}, func(ch harness.StreamChunk) error {
		got.WriteString(ch.Text)
		return nil
	})
	if err != nil || got.String() != "whole reply" || resp.Message != "whole reply" {
		t.Fatalf("stream=%q response=%+v error=%v", got.String(), resp, err)
	}
}

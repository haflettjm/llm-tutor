package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTutorStreamEmitsChunkThenDone(t *testing.T) {
	r, _, _ := newRouter(t, trackAt(0))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tutor/stream", strings.NewReader(`{"message":"hi","session_id":"s"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	for _, want := range []string{"event: chunk", `"text":"What do you predict?"`, "event: done", `"response_type":"question"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %q:\n%s", want, w.Body.String())
		}
	}
}

package tutor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleStub(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "MENTOR.md"), []byte("# test mentor"), 0644); err != nil {
		t.Fatal(err)
	}
	soulsDir := filepath.Join(dir, "souls")
	if err := os.MkdirAll(soulsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(soulsDir, "concepts-tutor.md"), []byte("# test soul"), 0644); err != nil {
		t.Fatal(err)
	}

	tutor, err := New("test-key", filepath.Join(dir, "MENTOR.md"), soulsDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := tutor.Handle(context.Background(), Request{
		Message:   "what is a variable?",
		Language:  "go",
		SessionID: "test",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.ResponseType == "" {
		t.Error("ResponseType should not be empty")
	}
}

package notify

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStdoutNotifierWritesMessages(t *testing.T) {
	var buf bytes.Buffer
	n := NewStdoutNotifier(&buf)

	if err := n.Info(context.Background(), "hello"); err != nil {
		t.Fatalf("info returned error: %v", err)
	}
	if err := n.Alert(context.Background(), "boom"); err != nil {
		t.Fatalf("alert returned error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "boom") {
		t.Fatalf("unexpected output: %q", out)
	}
}

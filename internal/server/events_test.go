package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEventBusFanOut(t *testing.T) {
	bus := NewEventBus(8)
	defer bus.Close()

	sub1, ch1 := bus.Subscribe()
	defer bus.Unsubscribe(sub1)
	sub2, ch2 := bus.Subscribe()
	defer bus.Unsubscribe(sub2)

	bus.Publish("ticket_done", map[string]any{"ticket": "sw-07"})

	select {
	case ev := <-ch1:
		if ev.Type != "ticket_done" {
			t.Fatalf("unexpected event type: %q", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event on subscriber 1")
	}

	select {
	case ev := <-ch2:
		if ev.Type != "ticket_done" {
			t.Fatalf("unexpected event type: %q", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event on subscriber 2")
	}
}

func TestEventsHandlerFormatsSSE(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		s.Router().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	s.Events().Publish("progress", map[string]any{"ticket": "sw-07", "progress": 42})
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: progress\n") {
		t.Fatalf("expected event line in SSE body, got %q", body)
	}
	if !strings.Contains(body, "data: {") {
		t.Fatalf("expected data line in SSE body, got %q", body)
	}
	if !strings.Contains(body, "\"ticket\":\"sw-07\"") {
		t.Fatalf("expected JSON payload in SSE body, got %q", body)
	}

	var payload map[string]any
	line := ""
	for _, part := range strings.Split(body, "\n") {
		if strings.HasPrefix(part, "data: ") {
			line = strings.TrimPrefix(part, "data: ")
			break
		}
	}
	if line == "" {
		t.Fatal("missing data line")
	}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("invalid data JSON: %v", err)
	}
}

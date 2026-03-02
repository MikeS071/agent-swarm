package tui

import (
	"strings"
	"testing"
)

func TestRenderTicketRowByStatus(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   string
	}{
		{name: "done", status: "done", want: "✅"},
		{name: "running", status: "running", want: "🔄"},
		{name: "queued", status: "todo", want: "⏳"},
		{name: "failed", status: "failed", want: "❌"},
		{name: "blocked", status: "blocked", want: "🔒"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := ticketRow{ID: "sw-01", Desc: "Ticket", Status: tc.status, Done: 1, Total: 2}
			line := renderTicketRow(row, false, false, 120)
			if !strings.Contains(line, tc.want) {
				t.Fatalf("expected %q in %q", tc.want, line)
			}
		})
	}
}

func TestRenderTicketRowIncludesProgressForRunningDetailed(t *testing.T) {
	row := ticketRow{ID: "sw-05", Desc: "Watchdog", Status: "running", Done: 4, Total: 6}
	line := renderTicketRow(row, true, false, 120)
	if !strings.Contains(line, "4/6") {
		t.Fatalf("expected progress ratio in row, got %q", line)
	}
	if !strings.Contains(line, "[") {
		t.Fatalf("expected progress bar in row, got %q", line)
	}
}

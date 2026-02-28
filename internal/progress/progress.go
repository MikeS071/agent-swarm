package progress

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
)

// Marker represents a parsed PROGRESS marker.
type Marker struct {
	Done  int
	Total int
}

// TicketProgress represents current progress for a ticket.
type TicketProgress struct {
	TicketID   string
	Status     string
	Progress   int
	Source     string
	TasksDone  int
	TasksTotal int
	LastOutput string
	RunningFor time.Duration
}

var markerPattern = regexp.MustCompile(`(?m)^\s*PROGRESS:\s*(\d+)\s*/\s*(\d+)\s*$`)

// ParseMarker returns the last valid PROGRESS: X/N marker in output.
func ParseMarker(output string) *Marker {
	matches := markerPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	for i := len(matches) - 1; i >= 0; i-- {
		done, err1 := strconv.Atoi(matches[i][1])
		total, err2 := strconv.Atoi(matches[i][2])
		if err1 != nil || err2 != nil || total <= 0 || done < 0 {
			continue
		}
		if done > total {
			done = total
		}
		return &Marker{Done: done, Total: total}
	}
	return nil
}

// InferHeuristic estimates progress from known output patterns.
func InferHeuristic(output string, runtime time.Duration) int {
	lower := strings.ToLower(output)

	score := 5
	if runtime > time.Minute {
		score = 10
	}
	if containsAny(lower, "created ", "write file", "updated ", "changed ") {
		score = max(score, 30)
	}
	if containsAny(lower, "thinking", "analyzing") && containsAny(lower, "created ", "write file", "updated ", "changed ") {
		score = max(score, 50)
	}
	if containsAny(lower, "go test", "npm run build", "cargo test", "pass", "build succeeded") {
		score = max(score, 70)
	}
	if containsAny(lower, "git commit", "[main", "[master", "[feat") {
		score = max(score, 90)
	}
	if containsAny(lower, "git push", "pushed") {
		score = max(score, 95)
	}
	if score > 100 {
		return 100
	}
	return score
}

// GetProgress computes ticket progress from output markers or heuristics.
func GetProgress(handle backend.AgentHandle, be backend.AgentBackend, promptTasks int) TicketProgress {
	ticketID := strings.TrimPrefix(handle.SessionName, "swarm-")
	status := "todo"
	if be.IsAlive(handle) {
		status = "running"
	}
	if be.HasExited(handle) {
		status = "done"
	}

	output, err := be.GetOutput(handle, 200)
	if err != nil {
		return TicketProgress{
			TicketID:   ticketID,
			Status:     "failed",
			Progress:   0,
			Source:     "heuristic",
			TasksTotal: promptTasks,
			RunningFor: time.Since(handle.StartedAt),
		}
	}

	lastLine := lastMeaningfulLine(output)
	if marker := ParseMarker(output); marker != nil {
		progress := marker.Done * 100 / marker.Total
		if status == "done" {
			progress = 100
		}
		return TicketProgress{
			TicketID:   ticketID,
			Status:     status,
			Progress:   progress,
			Source:     "marker",
			TasksDone:  marker.Done,
			TasksTotal: marker.Total,
			LastOutput: lastLine,
			RunningFor: time.Since(handle.StartedAt),
		}
	}

	progress := InferHeuristic(output, time.Since(handle.StartedAt))
	if status == "done" {
		progress = 100
	}
	return TicketProgress{
		TicketID:   ticketID,
		Status:     status,
		Progress:   progress,
		Source:     "heuristic",
		TasksTotal: promptTasks,
		LastOutput: lastLine,
		RunningFor: time.Since(handle.StartedAt),
	}
}

func containsAny(s string, items ...string) bool {
	for _, item := range items {
		if strings.Contains(s, item) {
			return true
		}
	}
	return false
}

func lastMeaningfulLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func max(a, b int) int {
	if b > a {
		return b
	}
	return a
}

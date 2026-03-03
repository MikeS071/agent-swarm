package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Tracker struct {
	Project string            `json:"project"`
	Tickets map[string]Ticket `json:"tickets"`
	UnlockedPhase int               `json:"unlocked_phase,omitempty"`
	filePath      string            `json:"-"`
}

type Ticket struct {
	Status     string   `json:"status"`
	Phase      int      `json:"phase"`
	Depends    []string `json:"depends"`
	Branch     string   `json:"branch,omitempty"`
	Desc       string   `json:"desc,omitempty"`
	Profile    string   `json:"profile,omitempty"`
	SHA        string   `json:"sha,omitempty"`
	StartedAt  string   `json:"startedAt,omitempty"`
	FinishedAt string   `json:"finishedAt,omitempty"`
}

type Stats struct {
	Done    int `json:"done"`
	Running int `json:"running"`
	Todo    int `json:"todo"`
	Failed  int `json:"failed"`
	Blocked int `json:"blocked"`
	Total   int `json:"total"`
}

var validStatuses = map[string]struct{}{
	"todo":    {},
	"running": {},
	"done":    {},
	"failed":  {},
	"blocked": {},
}

func Load(path string) (*Tracker, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tracker %s: %w", path, err)
	}
	var t Tracker
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse tracker %s: %w", path, err)
	}
	if t.Tickets == nil {
		t.Tickets = map[string]Ticket{}
	}
	t.filePath = path
	return &t, nil
}

func (t *Tracker) Save() error {
	if t.filePath == "" {
		return fmt.Errorf("tracker has no file path")
	}
	return t.SaveTo(t.filePath)
}

func (t *Tracker) SaveTo(path string) error {
	if t == nil {
		return fmt.Errorf("tracker is nil")
	}
	if t.Tickets == nil {
		t.Tickets = map[string]Ticket{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir tracker dir: %w", err)
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracker: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write tracker %s: %w", path, err)
	}
	return nil
}

func (t *Tracker) SetStatus(id, status string) error {
	ticket, ok := t.Tickets[id]
	if !ok {
		return fmt.Errorf("ticket %q not found", id)
	}
	status = strings.TrimSpace(status)
	if _, ok := validStatuses[status]; !ok {
		return fmt.Errorf("invalid status %q", status)
	}
	ticket.Status = status
	t.Tickets[id] = ticket
	return nil
}

func (t *Tracker) GetSpawnable() []string {
	active := t.ActivePhase()
	if active == 0 {
		return nil
	}
	ids := sortedTicketIDs(t.Tickets)
	out := make([]string, 0)
	for _, id := range ids {
		ticket := t.Tickets[id]
		if ticket.Status != "todo" {
			continue
		}
		if ticket.Phase > active {
			continue
		}
		ready := true
		for _, dep := range ticket.Depends {
			depTicket, ok := t.Tickets[dep]
			if !ok || depTicket.Status != "done" {
				ready = false
				break
			}
		}
		if ready {
			out = append(out, id)
		}
	}
	return out
}

func (t *Tracker) GetByPhase(phase int) []Ticket {
	ids := sortedTicketIDs(t.Tickets)
	out := make([]Ticket, 0)
	for _, id := range ids {
		ticket := t.Tickets[id]
		if ticket.Phase == phase {
			out = append(out, ticket)
		}
	}
	return out
}

func (t *Tracker) ActivePhase() int {
	minPhase := 0
	for _, ticket := range t.Tickets {
		if ticket.Status == "done" {
			continue
		}
		if minPhase == 0 || ticket.Phase < minPhase {
			minPhase = ticket.Phase
		}
	}
	return minPhase
}

func (t *Tracker) Stats() Stats {
	var s Stats
	for _, ticket := range t.Tickets {
		s.Total++
		switch ticket.Status {
		case "done":
			s.Done++
		case "running":
			s.Running++
		case "todo":
			s.Todo++
		case "failed":
			s.Failed++
		case "blocked":
			s.Blocked++
		}
	}
	return s
}

func (t *Tracker) DependencyOrder() []string {
	ids := sortedTicketIDs(t.Tickets)
	inDegree := make(map[string]int, len(ids))
	adj := make(map[string][]string, len(ids))
	for _, id := range ids {
		inDegree[id] = 0
	}
	for id, ticket := range t.Tickets {
		for _, dep := range ticket.Depends {
			if _, ok := t.Tickets[dep]; !ok {
				continue
			}
			adj[dep] = append(adj[dep], id)
			inDegree[id]++
		}
	}
	for key := range adj {
		sort.Strings(adj[key])
	}

	queue := make([]string, 0)
	for _, id := range ids {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]string, 0, len(ids))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, nxt := range adj[id] {
			inDegree[nxt]--
			if inDegree[nxt] == 0 {
				queue = append(queue, nxt)
				sort.Strings(queue)
			}
		}
	}

	if len(order) != len(ids) {
		seen := map[string]struct{}{}
		for _, id := range order {
			seen[id] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := seen[id]; !ok {
				order = append(order, id)
			}
		}
	}
	return order
}

func sortedTicketIDs(tickets map[string]Ticket) []string {
	ids := make([]string, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Status constants for typed usage by dispatcher
const (
	StatusTodo    = "todo"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Get returns a ticket by ID
func (t *Tracker) Get(id string) (Ticket, bool) {
	tk, ok := t.Tickets[id]
	return tk, ok
}

// AllDone returns true if every ticket is done
func (t *Tracker) AllDone() bool {
	for _, tk := range t.Tickets {
		if tk.Status != StatusDone {
			return false
		}
	}
	return true
}

// RunningCount returns the number of running tickets
func (t *Tracker) RunningCount() int {
	n := 0
	for _, tk := range t.Tickets {
		if tk.Status == StatusRunning {
			n++
		}
	}
	return n
}

// MarkDone sets a ticket to done with optional SHA
func (t *Tracker) MarkDone(id, sha string) error {
	tk, ok := t.Tickets[id]
	if !ok {
		return fmt.Errorf("ticket %q not found", id)
	}
	tk.Status = StatusDone
	tk.SHA = sha
	t.Tickets[id] = tk
	return nil
}

// MarkFailed sets a ticket to failed
func (t *Tracker) MarkFailed(id string) error {
	return t.SetStatus(id, StatusFailed)
}

// PhaseNumbers returns sorted unique phase numbers
func (t *Tracker) PhaseNumbers() []int {
	seen := map[int]bool{}
	for _, tk := range t.Tickets {
		seen[tk.Phase] = true
	}
	phases := make([]int, 0, len(seen))
	for p := range seen {
		phases = append(phases, p)
	}
	sort.Ints(phases)
	return phases
}

// TicketsByPhase returns all tickets in a given phase
func (t *Tracker) TicketsByPhase(phase int) map[string]Ticket {
	result := make(map[string]Ticket)
	for id, tk := range t.Tickets {
		if tk.Phase == phase {
			result[id] = tk
		}
	}
	return result
}

// New creates a Tracker from a map of tickets (for testing/programmatic use)
func New(project string, tickets map[string]Ticket) *Tracker {
	return &Tracker{
		Project: project,
		Tickets: tickets,
	}
}

// NewFromPtrs creates a Tracker from pointer map (convenience for tests)
func NewFromPtrs(project string, tickets map[string]*Ticket) *Tracker {
	m := make(map[string]Ticket, len(tickets))
	for k, v := range tickets {
		m[k] = *v
	}
	return &Tracker{Project: project, Tickets: m}
}

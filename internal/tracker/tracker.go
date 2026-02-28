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
}

type Ticket struct {
	Status     string   `json:"status"`
	Phase      int      `json:"phase"`
	Depends    []string `json:"depends"`
	Branch     string   `json:"branch,omitempty"`
	Desc       string   `json:"desc,omitempty"`
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
	return &t, nil
}

func (t *Tracker) Save(path string) error {
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

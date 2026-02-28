package tracker

import (
	"errors"
	"sort"
	"sync"
)

type Status string

const (
	StatusTodo    Status = "todo"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Ticket struct {
	Status  Status
	Phase   int
	Depends []string
	Branch  string
	Desc    string
	SHA     string
}

type Tracker struct {
	project string
	mu      sync.RWMutex
	tickets map[string]*Ticket
}

func New(project string, tickets map[string]*Ticket) *Tracker {
	cp := make(map[string]*Ticket, len(tickets))
	for id, t := range tickets {
		if t == nil {
			continue
		}
		tt := *t
		if t.Depends != nil {
			tt.Depends = append([]string(nil), t.Depends...)
		}
		cp[id] = &tt
	}
	return &Tracker{project: project, tickets: cp}
}

func (t *Tracker) IDs() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ids := make([]string, 0, len(t.tickets))
	for id := range t.tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (t *Tracker) Get(id string) (*Ticket, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tk, ok := t.tickets[id]
	if !ok {
		return nil, false
	}
	cp := *tk
	cp.Depends = append([]string(nil), tk.Depends...)
	return &cp, true
}

func (t *Tracker) SetStatus(id string, s Status) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tk, ok := t.tickets[id]
	if !ok {
		return errors.New("ticket not found")
	}
	tk.Status = s
	return nil
}

func (t *Tracker) MarkDone(id, sha string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tk, ok := t.tickets[id]
	if !ok {
		return errors.New("ticket not found")
	}
	tk.Status = StatusDone
	tk.SHA = sha
	return nil
}

func (t *Tracker) MarkFailed(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tk, ok := t.tickets[id]
	if !ok {
		return errors.New("ticket not found")
	}
	tk.Status = StatusFailed
	return nil
}

func (t *Tracker) RunningCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	n := 0
	for _, tk := range t.tickets {
		if tk.Status == StatusRunning {
			n++
		}
	}
	return n
}

func (t *Tracker) TicketsByPhase(phase int) map[string]*Ticket {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := map[string]*Ticket{}
	for id, tk := range t.tickets {
		if tk.Phase != phase {
			continue
		}
		cp := *tk
		cp.Depends = append([]string(nil), tk.Depends...)
		out[id] = &cp
	}
	return out
}

func (t *Tracker) PhaseNumbers() []int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	set := map[int]struct{}{}
	for _, tk := range t.tickets {
		set[tk.Phase] = struct{}{}
	}
	phases := make([]int, 0, len(set))
	for p := range set {
		phases = append(phases, p)
	}
	sort.Ints(phases)
	return phases
}

func (t *Tracker) AllDone() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, tk := range t.tickets {
		if tk.Status != StatusDone {
			return false
		}
	}
	return true
}

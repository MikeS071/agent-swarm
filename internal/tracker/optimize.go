package tracker

import "sort"

type OptimizeChange struct {
	TicketID     string `json:"ticket_id"`
	OldPriority  int    `json:"old_priority"`
	NewPriority  int    `json:"new_priority"`
	Descendants  int    `json:"descendants"`
	CriticalPath int    `json:"critical_path"`
}

type OptimizeOptions struct {
	OnlyTodo bool
}

// OptimizePriorities computes throughput-oriented ticket priorities based on
// graph centrality (descendants) and critical path depth, then writes
// priorities back to tracker tickets.
func (t *Tracker) OptimizePriorities(opts OptimizeOptions) []OptimizeChange {
	if t == nil || len(t.Tickets) == 0 {
		return nil
	}

	children := make(map[string][]string, len(t.Tickets))
	for id := range t.Tickets {
		children[id] = nil
	}
	for id, tk := range t.Tickets {
		for _, dep := range tk.Depends {
			if _, ok := t.Tickets[dep]; !ok {
				continue
			}
			children[dep] = append(children[dep], id)
		}
	}
	for id := range children {
		sort.Strings(children[id])
	}

	descMemo := map[string]int{}
	depthMemo := map[string]int{}

	var descendantsCount func(string) int
	descendantsCount = func(id string) int {
		if v, ok := descMemo[id]; ok {
			return v
		}
		seen := map[string]struct{}{}
		queue := append([]string{}, children[id]...)
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			queue = append(queue, children[n]...)
		}
		descMemo[id] = len(seen)
		return len(seen)
	}

	var criticalDepth func(string, map[string]bool) int
	criticalDepth = func(id string, stack map[string]bool) int {
		if v, ok := depthMemo[id]; ok {
			return v
		}
		if stack[id] {
			return 0
		}
		stack[id] = true
		maxDepth := 0
		for _, child := range children[id] {
			d := 1 + criticalDepth(child, stack)
			if d > maxDepth {
				maxDepth = d
			}
		}
		delete(stack, id)
		depthMemo[id] = maxDepth
		return maxDepth
	}

	changes := make([]OptimizeChange, 0)
	for id, tk := range t.Tickets {
		if opts.OnlyTodo && tk.Status != StatusTodo {
			continue
		}
		desc := descendantsCount(id)
		depth := criticalDepth(id, map[string]bool{})
		// Weight depth above fanout so critical path items get first slots.
		newPriority := depth*1000 + desc*10
		if tk.Status == StatusTodo && newPriority == 0 {
			newPriority = 1
		}
		if tk.Priority == newPriority {
			continue
		}
		changes = append(changes, OptimizeChange{
			TicketID:     id,
			OldPriority:  tk.Priority,
			NewPriority:  newPriority,
			Descendants:  desc,
			CriticalPath: depth,
		})
		tk.Priority = newPriority
		t.Tickets[id] = tk
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].NewPriority == changes[j].NewPriority {
			return changes[i].TicketID < changes[j].TicketID
		}
		return changes[i].NewPriority > changes[j].NewPriority
	})

	return changes
}

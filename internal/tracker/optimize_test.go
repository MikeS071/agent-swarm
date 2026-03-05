package tracker

import "testing"

func TestOptimizePrioritiesPrefersCriticalPathAndFanout(t *testing.T) {
	t.Parallel()
	tr := New("proj", map[string]Ticket{
		"a": {Status: StatusTodo, Phase: 1},
		"b": {Status: StatusTodo, Phase: 1, Depends: []string{"a"}},
		"c": {Status: StatusTodo, Phase: 1, Depends: []string{"a"}},
		"d": {Status: StatusTodo, Phase: 1, Depends: []string{"b"}},
		"e": {Status: StatusTodo, Phase: 1, Depends: []string{"c"}},
		"f": {Status: StatusTodo, Phase: 1, Depends: []string{"e"}},
	})

	changes := tr.OptimizePriorities(OptimizeOptions{OnlyTodo: true})
	if len(changes) == 0 {
		t.Fatal("expected priority changes")
	}

	if tr.Tickets["a"].Priority <= tr.Tickets["c"].Priority {
		t.Fatalf("expected a priority > c, got a=%d c=%d", tr.Tickets["a"].Priority, tr.Tickets["c"].Priority)
	}
	if tr.Tickets["c"].Priority <= tr.Tickets["b"].Priority {
		t.Fatalf("expected c priority > b (deeper critical path), got c=%d b=%d", tr.Tickets["c"].Priority, tr.Tickets["b"].Priority)
	}
}

func TestOptimizePrioritiesOnlyTodo(t *testing.T) {
	t.Parallel()
	tr := New("proj", map[string]Ticket{
		"a": {Status: StatusDone, Phase: 1, Priority: 77},
		"b": {Status: StatusTodo, Phase: 1},
	})
	_ = tr.OptimizePriorities(OptimizeOptions{OnlyTodo: true})
	if tr.Tickets["a"].Priority != 77 {
		t.Fatalf("expected done ticket priority unchanged, got %d", tr.Tickets["a"].Priority)
	}
	if tr.Tickets["b"].Priority == 0 {
		t.Fatalf("expected todo ticket priority to be set")
	}
}

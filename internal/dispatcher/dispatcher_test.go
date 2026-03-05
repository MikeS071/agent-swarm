package dispatcher

import (
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func testCfg(maxAgents int) *config.Config {
	return &config.Config{Project: config.ProjectConfig{MaxAgents: maxAgents, MinRAMMB: 0}}
}

func testTracker() *tracker.Tracker {
	return tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusTodo, Phase: 1},
		"b": {Status: tracker.StatusTodo, Phase: 1, Depends: []string{"a"}},
		"c": {Status: tracker.StatusTodo, Phase: 2, Depends: []string{"b"}},
	})
}

func TestEvaluateSpawnableInCurrentPhase(t *testing.T) {
	tr := testTracker()
	d := New(testCfg(3), tr)
	d.ramOK = func(int) bool { return true }

	sig, ids := d.Evaluate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn signal, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "a" {
		t.Fatalf("expected [a], got %#v", ids)
	}
}

func TestEvaluateBlockedWhenNoSpawnable(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusRunning, Phase: 1},
		"b": {Status: tracker.StatusTodo, Phase: 1, Depends: []string{"a"}},
	})
	d := New(testCfg(3), tr)
	d.ramOK = func(int) bool { return true }

	sig, ids := d.Evaluate()
	if sig != SignalBlocked {
		t.Fatalf("expected blocked signal, got %q", sig)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids, got %#v", ids)
	}
}

func TestPhaseGateDetection(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusDone, Phase: 1},
		"b": {Status: tracker.StatusDone, Phase: 1},
		"c": {Status: tracker.StatusTodo, Phase: 2},
	})
	d := New(testCfg(3), tr)

	sig, ids := d.Evaluate()
	if sig != SignalPhaseGate {
		t.Fatalf("expected phase gate, got %q", sig)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no spawnables before approval, got %#v", ids)
	}

	ps := d.PhaseStatus()
	if !ps.GateReached || ps.Phase != 1 || ps.Done != 2 || ps.Total != 2 {
		t.Fatalf("unexpected phase status: %#v", ps)
	}
}

func TestMarkDoneChainsAndUnblocks(t *testing.T) {
	tr := testTracker()
	d := New(testCfg(3), tr)

	sig, ids := d.MarkDone("a", "sha-a")
	if sig != SignalSpawn {
		t.Fatalf("expected spawn after done, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("expected b to unblock, got %#v", ids)
	}

	sig, ids = d.MarkDone("b", "sha-b")
	if sig != SignalPhaseGate {
		t.Fatalf("expected phase gate, got %q", sig)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids before phase approval, got %#v", ids)
	}
}

func TestApprovePhaseGateFlow(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusDone, Phase: 1},
		"b": {Status: tracker.StatusTodo, Phase: 2},
		"c": {Status: tracker.StatusTodo, Phase: 2, Depends: []string{"b"}},
	})
	d := New(testCfg(3), tr)

	if d.CurrentPhase() != 1 {
		t.Fatalf("expected phase 1, got %d", d.CurrentPhase())
	}

	sig, ids := d.ApprovePhaseGate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn after gate approval, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("expected b spawnable, got %#v", ids)
	}

	if d.CurrentPhase() != 2 {
		t.Fatalf("expected phase 2 after approval, got %d", d.CurrentPhase())
	}
}

func TestEvaluateAllDone(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusDone, Phase: 1},
		"b": {Status: tracker.StatusDone, Phase: 2},
	})
	d := New(testCfg(3), tr)

	sig, ids := d.Evaluate()
	if sig != SignalAllDone {
		t.Fatalf("expected all done, got %q", sig)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids, got %#v", ids)
	}
}

func TestCanSpawnMoreRespectsMaxAndRAM(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusRunning, Phase: 1},
		"b": {Status: tracker.StatusTodo, Phase: 1},
	})
	d := New(testCfg(1), tr)
	d.ramOK = func(int) bool { return true }

	if d.CanSpawnMore() {
		t.Fatal("expected cannot spawn when running == max")
	}

	d = New(testCfg(2), tr)
	d.ramOK = func(int) bool { return false }
	if d.CanSpawnMore() {
		t.Fatal("expected cannot spawn when RAM check fails")
	}
}

func TestNextSpawnableLimit(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusTodo, Phase: 1},
		"b": {Status: tracker.StatusTodo, Phase: 1},
		"c": {Status: tracker.StatusTodo, Phase: 1},
	})
	d := New(testCfg(5), tr)

	ids := d.NextSpawnable(2)
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %#v", ids)
	}
}

func TestEvaluateSpawnOrderUsesPriority(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"a": {Status: tracker.StatusTodo, Phase: 1, Priority: 10},
		"b": {Status: tracker.StatusTodo, Phase: 1, Priority: 90},
		"c": {Status: tracker.StatusTodo, Phase: 1, Priority: 50},
	})
	d := New(testCfg(3), tr)
	d.ramOK = func(int) bool { return true }

	sig, ids := d.Evaluate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn signal, got %q", sig)
	}
	want := []string{"b", "c", "a"}
	if len(ids) != len(want) {
		t.Fatalf("expected %d ids, got %#v", len(want), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, ids)
		}
	}
}

func TestEvaluateDispatchOrderByTicketType(t *testing.T) {
	tr := tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
		"feat-a":     {Status: tracker.StatusTodo, Phase: 1, Type: "feature"},
		"int-run-1":  {Status: tracker.StatusTodo, Phase: 1, Type: "integration"},
		"review-run": {Status: tracker.StatusTodo, Phase: 1, Type: "post_build"},
	})
	d := New(testCfg(3), tr)

	sig, ids := d.Evaluate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn signal, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "feat-a" {
		t.Fatalf("expected feature ticket first, got %v", ids)
	}

	if err := tr.MarkDone("feat-a", "sha-feat"); err != nil {
		t.Fatalf("mark feat done: %v", err)
	}
	sig, ids = d.Evaluate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn signal after feature done, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "int-run-1" {
		t.Fatalf("expected integration ticket second, got %v", ids)
	}

	if err := tr.MarkDone("int-run-1", "sha-int"); err != nil {
		t.Fatalf("mark integration done: %v", err)
	}
	sig, ids = d.Evaluate()
	if sig != SignalSpawn {
		t.Fatalf("expected spawn signal after integration done, got %q", sig)
	}
	if len(ids) != 1 || ids[0] != "review-run" {
		t.Fatalf("expected post_build ticket last, got %v", ids)
	}
}

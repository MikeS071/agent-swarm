package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type Signal string

const (
	SignalSpawn     Signal = ""
	SignalPhaseGate Signal = "PHASE_GATE"
	SignalAllDone   Signal = "ALL_DONE"
	SignalBlocked   Signal = "BLOCKED"
)

type PhaseStatus struct {
	Phase       int
	Total       int
	Done        int
	Running     int
	GateReached bool
}

type Dispatcher struct {
	config  *config.Config
	tracker *tracker.Tracker

	mu            sync.RWMutex
	unlockedPhase int
	ramOK         func(minMB int) bool
}

func New(cfg *config.Config, t *tracker.Tracker) *Dispatcher {
	if cfg == nil {
		cfg = &config.Config{}
	}
	d := &Dispatcher{config: cfg, tracker: t, ramOK: hasMinRAM}
	d.unlockedPhase = d.initialPhase()
	return d
}

func (d *Dispatcher) Evaluate() (Signal, []string) {
	if d.tracker == nil {
		return SignalBlocked, nil
	}
	if d.tracker.AllDone() {
		return SignalAllDone, nil
	}

	if d.phaseGateReached() {
		if d.config.Project.AutoApprove {
			// Auto-advance through the gate
			d.ApprovePhaseGate()
			// Re-evaluate after advancing
			return d.evaluateAfterApprove()
		}
		return SignalPhaseGate, nil
	}

	spawnable := d.spawnableInPhase(d.CurrentPhase())
	if len(spawnable) > 0 {
		return SignalSpawn, spawnable
	}

	// Current phase blocked (failed/running tickets) — look across all phases
	// for any ticket whose deps are satisfied
	crossPhase := d.spawnableAcrossPhases()
	if len(crossPhase) > 0 {
		return SignalSpawn, crossPhase
	}

	return SignalBlocked, nil
}

func (d *Dispatcher) evaluateAfterApprove() (Signal, []string) {
	if d.tracker.AllDone() {
		return SignalAllDone, nil
	}
	spawnable := d.spawnableInPhase(d.CurrentPhase())
	if len(spawnable) > 0 {
		return SignalSpawn, spawnable
	}
	return SignalBlocked, nil
}

func (d *Dispatcher) CanSpawnMore() bool {
	max := d.config.Project.MaxAgents
	if max <= 0 {
		max = 1
	}
	if d.tracker == nil {
		return false
	}
	if d.tracker.RunningCount() >= max {
		return false
	}
	return d.ramOK(d.config.Project.MinRAMMB)
}

func (d *Dispatcher) NextSpawnable(n int) []string {
	if n <= 0 {
		return nil
	}
	sig, ids := d.Evaluate()
	if sig != SignalSpawn || len(ids) == 0 {
		return nil
	}
	if len(ids) <= n {
		return ids
	}
	return ids[:n]
}

func (d *Dispatcher) MarkDone(ticketID, sha string) (Signal, []string) {
	if d.tracker != nil {
		_ = d.tracker.MarkDone(ticketID, sha)
	}
	return d.Evaluate()
}

func (d *Dispatcher) MarkFailed(ticketID string) error {
	if d.tracker == nil {
		return nil
	}
	return d.tracker.MarkFailed(ticketID)
}

func (d *Dispatcher) ApprovePhaseGate() (Signal, []string) {
	if !d.phaseGateReached() {
		return d.Evaluate()
	}

	phases := d.tracker.PhaseNumbers()
	cur := d.CurrentPhase()
	for _, p := range phases {
		if p > cur {
			d.mu.Lock()
			d.unlockedPhase = p
			d.mu.Unlock()
			if d.tracker != nil {
				d.tracker.UnlockedPhase = p
				if saveErr := d.tracker.Save(); saveErr != nil { fmt.Fprintf(os.Stderr, "SAVE ERROR: %v\n", saveErr) }
			}
			break
		}
	}
	return d.Evaluate()
}

// SetUnlockedPhase updates the dispatcher's unlocked phase from external source (e.g. tracker file reload).
func (d *Dispatcher) SetUnlockedPhase(phase int) {
	d.mu.Lock()
	d.unlockedPhase = phase
	d.mu.Unlock()
}

func (d *Dispatcher) CurrentPhase() int {
	d.mu.RLock()
	cur := d.unlockedPhase
	d.mu.RUnlock()

	if cur != 0 {
		return cur
	}
	return d.initialPhase()
}

func (d *Dispatcher) PhaseStatus() PhaseStatus {
	phase := d.CurrentPhase()
	tickets := d.tracker.TicketsByPhase(phase)
	ps := PhaseStatus{Phase: phase, Total: len(tickets)}
	for _, tk := range tickets {
		switch tk.Status {
		case tracker.StatusDone:
			ps.Done++
		case tracker.StatusRunning:
			ps.Running++
		}
	}
	ps.GateReached = d.phaseGateReached()
	return ps
}

func (d *Dispatcher) initialPhase() int {
	if d.tracker == nil {
		return 0
	}
	if d.tracker.UnlockedPhase > 0 {
		return d.tracker.UnlockedPhase
	}
	phases := d.tracker.PhaseNumbers()
	if len(phases) == 0 {
		return 0
	}
	return phases[0]
}

func (d *Dispatcher) phaseGateReached() bool {
	if d.tracker == nil {
		return false
	}
	current := d.CurrentPhase()
	if current == 0 {
		return false
	}
	curTickets := d.tracker.TicketsByPhase(current)
	if len(curTickets) == 0 {
		return false
	}
	for _, tk := range curTickets {
		if tk.Status != tracker.StatusDone {
			return false
		}
	}

	phases := d.tracker.PhaseNumbers()
	for _, p := range phases {
		if p > current && len(d.tracker.TicketsByPhase(p)) > 0 {
			return true
		}
	}
	return false
}

func (d *Dispatcher) spawnableInPhase(phase int) []string {
	if d.tracker == nil || phase == 0 {
		return nil
	}

	tickets := d.tracker.TicketsByPhase(phase)
	ids := make([]string, 0, len(tickets))
	for id, tk := range tickets {
		if tk.Status != tracker.StatusTodo {
			continue
		}
		ready := true
		for _, dep := range tk.Depends {
			dt, ok := d.tracker.Get(dep)
			if !ok || dt.Status != tracker.StatusDone {
				ready = false
				break
			}
		}
		if ready {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}


func (d *Dispatcher) spawnableAcrossPhases() []string {
	if d.tracker == nil {
		return nil
	}
	maxPhase := d.CurrentPhase()
	if d.config.Project.AutoApprove {
		maxPhase = 9999 // no phase restriction when auto-approve is on
	}
	var ids []string
	for id, tk := range d.tracker.Tickets {
		if tk.Status != tracker.StatusTodo {
			continue
		}
		if tk.Phase > maxPhase {
			continue
		}
		ready := true
		for _, dep := range tk.Depends {
			dt, ok := d.tracker.Get(dep)
			if !ok || dt.Status != tracker.StatusDone {
				ready = false
				break
			}
		}
		if ready {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func hasMinRAM(minMB int) bool {
	if minMB <= 0 {
		return true
	}
	// Use MemAvailable from /proc/meminfo (not Freeram which excludes buffers/cache)
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return true // can't check, assume OK
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb := 0
				fmt.Sscanf(fields[1], "%d", &kb)
				return (kb / 1024) >= minMB
			}
		}
	}
	// Fallback to sysinfo if /proc/meminfo doesn't have MemAvailable
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return true
	}
	freeBytes := uint64(info.Freeram) * uint64(info.Unit)
	freeMB := freeBytes / (1024 * 1024)
	return int(freeMB) >= minMB
}

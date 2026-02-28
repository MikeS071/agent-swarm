package server

import (
	"encoding/json"
	"sync"
)

const (
	EventTicketDone    = "ticket_done"
	EventTicketSpawned = "ticket_spawned"
	EventProgress      = "progress"
	EventPhaseGate     = "phase_gate"
	EventFailure       = "failure"
	EventRAMWarning    = "ram_warning"
)

type Event struct {
	Type string
	Data []byte
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[int]chan Event
	nextID      int
	buffer      int
	closed      bool
}

func NewEventBus(buffer int) *EventBus {
	if buffer <= 0 {
		buffer = 16
	}
	return &EventBus{
		subscribers: make(map[int]chan Event),
		buffer:      buffer,
	}
}

func (b *EventBus) Subscribe() (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		ch := make(chan Event)
		close(ch)
		return 0, ch
	}
	b.nextID++
	id := b.nextID
	ch := make(chan Event, b.buffer)
	b.subscribers[id] = ch
	return id, ch
}

func (b *EventBus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

func (b *EventBus) Publish(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	ev := Event{Type: eventType, Data: data}

	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// Drop on slow subscribers to preserve fan-out.
		}
	}
}

func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.subscribers {
		delete(b.subscribers, id)
		close(ch)
	}
}

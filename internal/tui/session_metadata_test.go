package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

type ticketBackendStub struct {
	name       string
	outputByID map[string]string
	err        error
	outputSeen []backend.AgentHandle
	killSeen   []backend.AgentHandle
}

func (s *ticketBackendStub) Spawn(context.Context, backend.SpawnConfig) (backend.AgentHandle, error) {
	return backend.AgentHandle{}, errors.New("not implemented")
}
func (s *ticketBackendStub) IsAlive(backend.AgentHandle) bool   { return true }
func (s *ticketBackendStub) HasExited(backend.AgentHandle) bool { return false }
func (s *ticketBackendStub) GetOutput(h backend.AgentHandle, _ int) (string, error) {
	s.outputSeen = append(s.outputSeen, h)
	if s.err != nil {
		return "", s.err
	}
	if s.outputByID == nil {
		return "", nil
	}
	return s.outputByID[h.SessionName], nil
}
func (s *ticketBackendStub) Kill(h backend.AgentHandle) error {
	s.killSeen = append(s.killSeen, h)
	return s.err
}
func (s *ticketBackendStub) Name() string { return s.name }

func TestSessionHandleForTicket(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		projectName string
		ticketID    string
		ticket      tracker.Ticket
		wantSession string
	}{
		{
			name:        "uses stored session metadata",
			projectName: "proj",
			ticketID:    "sw-01",
			ticket:      tracker.Ticket{SessionName: "alt-session-sw-01"},
			wantSession: "alt-session-sw-01",
		},
		{
			name:        "falls back to namespaced session",
			projectName: "proj",
			ticketID:    "sw-01",
			ticket:      tracker.Ticket{},
			wantSession: "swarm-proj_sw-01",
		},
		{
			name:        "falls back to legacy session when project missing",
			projectName: "",
			ticketID:    "sw-01",
			ticket:      tracker.Ticket{},
			wantSession: "swarm-sw-01",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := model{config: &config.Config{Project: config.ProjectConfig{Name: tc.projectName}}}
			h := m.sessionHandleForTicket(tc.ticketID, tc.ticket)
			if h.SessionName != tc.wantSession {
				t.Fatalf("sessionHandleForTicket() session = %q, want %q", h.SessionName, tc.wantSession)
			}
		})
	}
}

func TestRefreshDetailOutputUsesSessionMetadata(t *testing.T) {
	defaultBackend := &ticketBackendStub{name: "codex-tmux", err: errors.New("wrong backend")}
	altBackend := &ticketBackendStub{name: "alt-backend", outputByID: map[string]string{"alt-session-sw-01": "line1\nline2\n"}}

	m := model{
		config: &config.Config{Project: config.ProjectConfig{Name: "proj"}},
		tracker: tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
			"sw-01": {Status: tracker.StatusRunning, SessionName: "alt-session-sw-01", SessionBackend: "alt-backend"},
		}),
		backend:  defaultBackend,
		detailID: "sw-01",
	}
	m.backendFactory = func(backendType string) (backend.AgentBackend, error) {
		switch backendType {
		case "alt-backend":
			return altBackend, nil
		default:
			return defaultBackend, nil
		}
	}

	m.refreshDetailOutput()

	if m.detailOutput != "line1\nline2" {
		t.Fatalf("detail output = %q, want %q", m.detailOutput, "line1\\nline2")
	}
	if len(altBackend.outputSeen) != 1 || altBackend.outputSeen[0].SessionName != "alt-session-sw-01" {
		t.Fatalf("alt backend GetOutput calls = %#v", altBackend.outputSeen)
	}
}

func TestKillSelectedUsesSessionMetadataBackend(t *testing.T) {
	defaultBackend := &ticketBackendStub{name: "codex-tmux"}
	altBackend := &ticketBackendStub{name: "alt-backend"}

	m := model{
		config: &config.Config{Project: config.ProjectConfig{Name: "proj"}},
		tracker: tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
			"sw-01": {Status: tracker.StatusRunning, SessionName: "alt-session-sw-01", SessionBackend: "alt-backend"},
		}),
		backend: defaultBackend,
		tickets: []ticketRow{{ID: "sw-01"}},
		cursor:  0,
	}
	m.backendFactory = func(backendType string) (backend.AgentBackend, error) {
		switch backendType {
		case "alt-backend":
			return altBackend, nil
		default:
			return defaultBackend, nil
		}
	}

	m.killSelected()

	if len(altBackend.killSeen) != 1 || altBackend.killSeen[0].SessionName != "alt-session-sw-01" {
		t.Fatalf("alt backend Kill calls = %#v", altBackend.killSeen)
	}
	if len(defaultBackend.killSeen) != 0 {
		t.Fatalf("default backend should not be used for kill, got %#v", defaultBackend.killSeen)
	}
}

func TestRebuildRowsUsesSessionMetadataBackendForProgress(t *testing.T) {
	defaultBackend := &ticketBackendStub{name: "codex-tmux", err: errors.New("wrong backend")}
	altBackend := &ticketBackendStub{name: "alt-backend", outputByID: map[string]string{"alt-session-sw-01": "PROGRESS: 2/4\n"}}

	m := model{
		config: &config.Config{Project: config.ProjectConfig{Name: "proj"}},
		tracker: tracker.NewFromPtrs("proj", map[string]*tracker.Ticket{
			"sw-01": {
				Status:         tracker.StatusRunning,
				SessionName:    "alt-session-sw-01",
				SessionBackend: "alt-backend",
				StartedAt:      time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339),
			},
		}),
		backend: defaultBackend,
	}
	m.backendFactory = func(backendType string) (backend.AgentBackend, error) {
		switch backendType {
		case "alt-backend":
			return altBackend, nil
		default:
			return defaultBackend, nil
		}
	}

	m.rebuildRows()

	if len(m.tickets) != 1 {
		t.Fatalf("tickets len = %d, want 1", len(m.tickets))
	}
	if m.tickets[0].Done != 2 || m.tickets[0].Total != 4 {
		t.Fatalf("progress = %d/%d, want 2/4", m.tickets[0].Done, m.tickets[0].Total)
	}
	if len(altBackend.outputSeen) == 0 {
		t.Fatal("expected progress to query alt backend")
	}
}

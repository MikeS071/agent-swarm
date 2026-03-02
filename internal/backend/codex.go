package backend

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const swarmSessionPrefix = "swarm-"

// CodexBackend implements AgentBackend using tmux sessions.
type CodexBackend struct {
	binary        string
	bypassSandbox bool
	runCmd        func(ctx context.Context, name string, args ...string) ([]byte, error)
	now           func() time.Time
}

// NewCodexBackend creates a codex tmux backend.
func NewCodexBackend(binary string, bypassSandbox bool) *CodexBackend {
	return newCodexBackendWithDeps(binary, bypassSandbox, exec.LookPath, os.UserHomeDir, defaultRunner, time.Now)
}

func newCodexBackendWithDeps(
	binary string,
	bypassSandbox bool,
	lookPath func(string) (string, error),
	homeDir func() (string, error),
	runner func(ctx context.Context, name string, args ...string) ([]byte, error),
	now func() time.Time,
) *CodexBackend {
	resolved := strings.TrimSpace(binary)
	if resolved == "" {
		if lookPath != nil {
			if p, err := lookPath("codex"); err == nil && p != "" {
				resolved = p
			}
		}
		if resolved == "" && homeDir != nil {
			if h, err := homeDir(); err == nil && h != "" {
				resolved = filepath.Join(h, ".local", "bin", "codex")
			}
		}
		if resolved == "" {
			resolved = "codex"
		}
	}
	if runner == nil {
		runner = defaultRunner
	}
	if now == nil {
		now = time.Now
	}

	return &CodexBackend{
		binary:        resolved,
		bypassSandbox: bypassSandbox,
		runCmd:        runner,
		now:           now,
	}
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (b *CodexBackend) Name() string {
	return "codex-tmux"
}

func (b *CodexBackend) Spawn(ctx context.Context, cfg SpawnConfig) (AgentHandle, error) {
	sessionName := swarmSessionPrefix + cfg.TicketID
	cmdStr := b.buildExecCommand(cfg)

	// Wrap with agent-wrapper to auto-commit + update tracker on exit
	if cfg.ProjectDir != "" {
		// Look for wrapper next to agent-swarm binary, then PATH
		candidates := []string{
			filepath.Join(filepath.Dir(b.binary), "agent-wrapper.sh"),
		}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "agent-wrapper.sh"))
		}
		for _, wp := range candidates {
			if _, serr := os.Stat(wp); serr == nil {
				cmdStr = fmt.Sprintf("bash %s %s %s %s %s",
					shQuote(wp),
					shQuote(cfg.ProjectDir),
					shQuote(cfg.WorkDir),
					shQuote(cfg.TicketID),
					cmdStr)
				break
			}
		}
	}

	// Capture stdout to log file for failure analysis
	logDir := filepath.Join(cfg.ProjectDir, "swarm", "logs")
	os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, cfg.TicketID+".log")
	cmdStr = fmt.Sprintf("(%s) 2>&1 | tee %s", cmdStr, shQuote(logFile))

	if _, err := b.runCmd(ctx, "tmux", "new-session", "-d", "-s", sessionName, cmdStr); err != nil {
		return AgentHandle{}, err
	}

	return AgentHandle{
		SessionName: sessionName,
		StartedAt:   b.now(),
	}, nil
}

func (b *CodexBackend) buildExecCommand(cfg SpawnConfig) string {
	parts := []string{shQuote(b.binary), "exec"}
	if strings.TrimSpace(cfg.Model) != "" {
		parts = append(parts, "-m", shQuote(cfg.Model))
	}
	if b.bypassSandbox {
		parts = append(parts, "--dangerously-bypass-approvals-and-sandbox")
	}
	if strings.TrimSpace(cfg.Effort) != "" {
		parts = append(parts, "--config", "model_reasoning_effort="+shQuote(cfg.Effort))
	}
	if strings.TrimSpace(cfg.WorkDir) != "" {
		parts = append(parts, "-C", shQuote(cfg.WorkDir))
	}
	for _, flag := range cfg.ExtraFlags {
		if strings.TrimSpace(flag) == "" {
			continue
		}
		parts = append(parts, shQuote(flag))
	}
	parts = append(parts, fmt.Sprintf("\"$(cat %s)\"", shQuote(cfg.PromptFile)))
	return strings.Join(parts, " ")
}

func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (b *CodexBackend) IsAlive(handle AgentHandle) bool {
	if strings.TrimSpace(handle.SessionName) == "" {
		return false
	}
	_, err := b.runCmd(context.Background(), "tmux", "has-session", "-t", handle.SessionName)
	return err == nil
}

func (b *CodexBackend) HasExited(handle AgentHandle) bool {
	if !b.IsAlive(handle) {
		// Session gone entirely — the agent has exited
		return true
	}
	out, err := b.runCmd(context.Background(), "tmux", "list-panes", "-t", handle.SessionName, "-F", "#{pane_pid}")
	if err != nil {
		return false
	}
	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		return false
	}
	pid, err := strconv.Atoi(strings.Split(pidStr, "\n")[0])
	if err != nil || pid <= 0 {
		return false
	}
	_, err = b.runCmd(context.Background(), "ps", "-p", strconv.Itoa(pid))
	return err != nil
}

func (b *CodexBackend) GetOutput(handle AgentHandle, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	out, err := b.runCmd(context.Background(), "tmux", "capture-pane", "-t", handle.SessionName, "-p", "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (b *CodexBackend) Kill(handle AgentHandle) error {
	_, err := b.runCmd(context.Background(), "tmux", "kill-session", "-t", handle.SessionName)
	return err
}

// ListSessions returns currently running swarm-managed tmux sessions.
func (b *CodexBackend) ListSessions(ctx context.Context) ([]string, error) {
	out, err := b.runCmd(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// tmux returns exit 1 when no server is running (zero sessions) — not an error
		outStr := string(out)
		errStr := err.Error()
		if strings.Contains(outStr, "no server running") || strings.Contains(errStr, "exit status 1") {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, swarmSessionPrefix) && !strings.Contains(line, "watchdog") {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

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
	if cfg.ProjectName != "" {
		sessionName = swarmSessionPrefix + cfg.ProjectName + "_" + cfg.TicketID
	}
	baseCmd := b.buildExecCommand(cfg)

	// Capture stdout to log file for failure analysis
	logDir := filepath.Join(cfg.ProjectDir, "swarm", "logs")
	os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, cfg.TicketID+".log")
	cmdStr := b.wrapWithExitArtifact(baseCmd, cfg, logFile)

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

func (b *CodexBackend) wrapWithExitArtifact(baseCmd string, cfg SpawnConfig, logFile string) string {
	lines := []string{
		"set -o pipefail",
		fmt.Sprintf("(%s) 2>&1 | tee %s", baseCmd, shQuote(logFile)),
		"ec=${PIPESTATUS[0]}",
		"ended=$(date -u +%FT%TZ)",
	}
	if strings.TrimSpace(cfg.WorkDir) != "" {
		lines = append(lines, fmt.Sprintf("head_sha=$(git -C %s rev-parse --short HEAD 2>/dev/null || true)", shQuote(cfg.WorkDir)))
	} else {
		lines = append(lines, "head_sha=")
	}
	if strings.TrimSpace(cfg.ExitFile) != "" {
		dir := filepath.Dir(cfg.ExitFile)
		if strings.TrimSpace(dir) != "" {
			lines = append(lines, fmt.Sprintf("mkdir -p %s", shQuote(dir)))
		}
		py := "python3 -c " + shQuote(`import json,os; print(json.dumps({"ticket_id":os.getenv("TICKET_ID",""),"ended_at":os.getenv("ENDED_AT",""),"process_exit_code":int(os.getenv("EXIT_CODE","0") or 0),"log_path":os.getenv("LOG_PATH",""),"work_dir":os.getenv("WORK_DIR",""),"head_sha":os.getenv("HEAD_SHA",""),"context_manifest_path":os.getenv("CONTEXT_MANIFEST_PATH","")}))`)
		lines = append(lines,
			fmt.Sprintf("TICKET_ID=%s ENDED_AT=\"$ended\" EXIT_CODE=\"$ec\" LOG_PATH=%s WORK_DIR=%s HEAD_SHA=\"$head_sha\" CONTEXT_MANIFEST_PATH=%s %s > %s", shQuote(cfg.TicketID), shQuote(logFile), shQuote(cfg.WorkDir), shQuote(cfg.ContextManifestPath), py, shQuote(cfg.ExitFile)),
		)
	}
	lines = append(lines, "exit $ec")
	script := strings.Join(lines, "; ")
	return fmt.Sprintf("bash -lc %s", shQuote(script))
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

// ListSessions returns all running swarm-managed tmux sessions (unfiltered).
func (b *CodexBackend) ListSessions(ctx context.Context) ([]string, error) {
	return b.ListSessionsForProject(ctx, "")
}

// ListSessionsForProject returns sessions filtered by project name.
func (b *CodexBackend) ListSessionsForProject(ctx context.Context, projectName string) ([]string, error) {
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
			if projectName != "" {
				expectedPrefix := swarmSessionPrefix + projectName + "_"
				if !strings.HasPrefix(line, expectedPrefix) {
					continue
				}
			}
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/progress"
	"github.com/MikeS071/agent-swarm/internal/sysinfo"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const refreshInterval = 3 * time.Second

type projectContext struct {
	name        string
	configPath  string
	trackerPath string
	configDir   string
}

type ticketRow struct {
	ID          string
	Desc        string
	Status      string
	SHA         string
	Depends     []string
	Progress    int
	Done        int
	Total       int
	LastOutput  string
	StatusLabel string
}

type tickMsg time.Time

type model struct {
	tracker  *tracker.Tracker
	config   *config.Config
	backend  backend.AgentBackend
	tickets  []ticketRow
	cursor   int
	viewMode string // "list" | "detail"
	detailID string
	compact  bool
	width    int
	height   int

	projects     []projectContext
	projectIndex int
	lastErr      error
	detailOutput string
}

func Run(configPath string, projectName string, compact bool) error {
	projects, err := discoverProjects(configPath)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return fmt.Errorf("no swarm.toml files found")
	}

	idx := 0
	if strings.TrimSpace(projectName) != "" {
		found := false
		for i, p := range projects {
			tr, err := tracker.Load(p.trackerPath)
			if err != nil {
				continue
			}
			if tr.Project == projectName {
				idx = i
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("project %q not found", projectName)
		}
	}

	cfg, tr, err := loadProject(projects[idx])
	if err != nil {
		return err
	}

	m := model{
		tracker:      tr,
		config:       cfg,
		backend:      newBackend(cfg),
		viewMode:     "list",
		compact:      compact,
		projects:     projects,
		projectIndex: idx,
	}
	m.rebuildRows()

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		m.refresh()
		return m, tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyUp:
			if m.viewMode != "detail" && m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.viewMode != "detail" && m.cursor < len(m.tickets)-1 {
				m.cursor++
			}
			return m, nil
		case tea.KeyEnter:
			if m.viewMode != "detail" && len(m.tickets) > 0 {
				m.viewMode = "detail"
				m.detailID = m.tickets[m.cursor].ID
				m.refreshDetailOutput()
			}
			return m, nil
		case tea.KeyEsc:
			if m.viewMode == "detail" {
				m.viewMode = "list"
				m.detailID = ""
			}
			return m, nil
		case tea.KeyTab:
			m.compact = !m.compact
			return m, nil
		}

		s := strings.ToLower(msg.String())
		switch s {
		case "q":
			return m, tea.Quit
		case "k":
			m.killSelected()
			m.refresh()
			return m, nil
		case "r":
			m.respawnSelected()
			m.refresh()
			return m, nil
		case "g":
			m.approveGate()
			m.refresh()
			return m, nil
		case "p":
			m.nextProject()
			return m, nil
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.viewMode == "detail" {
		return m.renderDetail()
	}
	return m.renderList()
}

func (m *model) refresh() {
	if len(m.projects) == 0 {
		return
	}
	cfg, tr, err := loadProject(m.projects[m.projectIndex])
	if err != nil {
		m.lastErr = err
		return
	}
	m.config = cfg
	m.tracker = tr
	m.backend = newBackend(cfg)
	m.rebuildRows()
	if m.viewMode == "detail" {
		m.refreshDetailOutput()
	}
}

func (m *model) refreshDetailOutput() {
	if strings.TrimSpace(m.detailID) == "" {
		m.detailOutput = ""
		return
	}
	if m.backend == nil {
		m.detailOutput = "backend unavailable"
		return
	}
	out, err := m.backend.GetOutput(backend.AgentHandle{SessionName: "swarm-" + m.detailID}, 50)
	if err != nil {
		m.detailOutput = fmt.Sprintf("unable to load output: %v", err)
		return
	}
	m.detailOutput = strings.TrimRight(out, "\n")
}

func (m *model) rebuildRows() {
	if m.tracker == nil {
		m.tickets = nil
		m.cursor = 0
		return
	}
	ids := make([]string, 0, len(m.tracker.Tickets))
	for id := range m.tracker.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	rows := make([]ticketRow, 0, len(ids))
	for _, id := range ids {
		tk := m.tracker.Tickets[id]
		row := ticketRow{
			ID:      id,
			Desc:    tk.Desc,
			Status:  tk.Status,
			SHA:     tk.SHA,
			Depends: append([]string(nil), tk.Depends...),
		}

		if tk.Status == "running" {
			h := backend.AgentHandle{SessionName: "swarm-" + id, StartedAt: time.Now().Add(-30 * time.Second)}
			pg := progress.GetProgress(h, m.backend, 0)
			row.Progress = pg.Progress
			row.Done = pg.TasksDone
			row.Total = pg.TasksTotal
			row.LastOutput = pg.LastOutput
		}
		if tk.Status == "done" {
			row.Progress = 100
			row.Done = 1
			row.Total = 1
		}
		if tk.Status == "todo" {
			row.StatusLabel = "queued"
		}
		rows = append(rows, row)
	}

	m.tickets = rows
	if m.cursor >= len(m.tickets) {
		m.cursor = maxInt(0, len(m.tickets)-1)
	}
}

func (m model) renderList() string {
	if m.tracker == nil || m.config == nil {
		return "loading..."
	}
	stats := m.tracker.Stats()
	done := stats.Done
	total := stats.Total
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	phase := m.tracker.ActivePhase()
	ramMB, _ := sysinfo.AvailableRAM()
	ramText := fmt.Sprintf("%.1f GB free", float64(ramMB)/1024.0)

	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render("agent-swarm — " + m.config.Project.Name)
	b.WriteString(title + "\n")
	b.WriteString(fmt.Sprintf("Progress: %s %d/%d (%d%%)\n", renderBarOnly(done, total, 24), done, total, pct))
	b.WriteString(fmt.Sprintf("Phase: %d | Agents: %d/%d | RAM: %s\n", phase, stats.Running, m.config.Project.MaxAgents, ramText))
	b.WriteString(strings.Repeat("-", maxInt(20, m.width-2)) + "\n")
	for i, row := range m.tickets {
		line := renderTicketRow(row, i == m.cursor, m.compact, m.width)
		b.WriteString(line + "\n")
	}
	b.WriteString("q: quit | Enter: view output | k: kill | r: respawn | g: approve gate | p: project | Tab: compact")
	if m.lastErr != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.lastErr.Error()))
	}
	return b.String()
}

func (m model) renderDetail() string {
	header := lipgloss.NewStyle().Bold(true).Render("ticket " + m.detailID)
	if strings.TrimSpace(m.detailOutput) == "" {
		return header + "\n\n(no output)\n\nEsc: back"
	}
	return header + "\n\n" + m.detailOutput + "\n\nEsc: back"
}

func renderTicketRow(row ticketRow, selected bool, compact bool, width int) string {
	status := normalizeStatus(row.Status)
	icon := statusIcon(status)
	label := status
	if row.StatusLabel != "" {
		label = row.StatusLabel
	}

	desc := strings.TrimSpace(row.Desc)
	if desc == "" {
		desc = row.ID
	}
	line := fmt.Sprintf("%s %s %s", icon, row.ID, desc)
	if compact {
		line = fmt.Sprintf("%s %s", line, styleForStatus(status).Render(label))
	} else {
		right := styleForStatus(status).Render(label)
		if status == "running" {
			right = styleForStatus(status).Render("running") + " " + renderProgressBar(row.Done, row.Total, 6)
		}
		if status == "done" && row.SHA != "" {
			right = right + fmt.Sprintf(" (%s)", shortSHA(row.SHA))
		}
		if status == "queued" && len(row.Depends) > 0 {
			right = right + " (needs " + strings.Join(row.Depends, ",") + ")"
		}
		line = fmt.Sprintf("%-52s %s", line, right)
	}
	if selected {
		return lipgloss.NewStyle().Bold(true).Render("› " + line)
	}
	return "  " + line
}

func renderProgressBar(done, total, width int) string {
	if width <= 0 {
		width = 6
	}
	if total <= 0 {
		total = 1
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %d/%d", bar, done, total)
}

func renderBarOnly(done, total, width int) string {
	if total <= 0 {
		total = 1
	}
	if done > total {
		done = total
	}
	filled := done * width / total
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func normalizeStatus(status string) string {
	switch status {
	case "done", "running", "failed", "blocked":
		return status
	case "todo":
		return "queued"
	default:
		return "queued"
	}
}

func statusIcon(status string) string {
	switch status {
	case "done":
		return "✅"
	case "running":
		return "🔄"
	case "failed":
		return "❌"
	case "blocked":
		return "🔒"
	default:
		return "⏳"
	}
}

func styleForStatus(status string) lipgloss.Style {
	s := lipgloss.NewStyle()
	switch status {
	case "done":
		return s.Foreground(lipgloss.Color("10"))
	case "running":
		return s.Foreground(lipgloss.Color("11"))
	case "failed":
		return s.Foreground(lipgloss.Color("9"))
	case "blocked":
		return s.Foreground(lipgloss.Color("8")).Faint(true)
	default:
		return s.Foreground(lipgloss.Color("8"))
	}
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func discoverProjects(preferredConfig string) ([]projectContext, error) {
	seen := map[string]bool{}
	result := make([]projectContext, 0)
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if seen[abs] {
			return
		}
		cfg, err := config.Load(abs)
		if err != nil {
			return
		}
		trackerPath := absOrJoin(filepath.Dir(abs), cfg.Project.Tracker)
		result = append(result, projectContext{
			name:        cfg.Project.Name,
			configPath:  abs,
			trackerPath: trackerPath,
			configDir:   filepath.Dir(abs),
		})
		seen[abs] = true
	}

	if strings.TrimSpace(preferredConfig) != "" {
		add(preferredConfig)
	}
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "swarm.toml" {
			add(path)
		}
		return nil
	})
	// Also scan workspace projects.json registry
	registryPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".openclaw", "workspace", "swarm", "projects.json"),
	}
	for _, rp := range registryPaths {
		data, err := os.ReadFile(rp)
		if err != nil {
			continue
		}
		var registry map[string]struct {
			Repo string `json:"repo"`
		}
		if json.Unmarshal(data, &registry) != nil {
			continue
		}
		for _, proj := range registry {
			if proj.Repo != "" {
				add(filepath.Join(proj.Repo, "swarm.toml"))
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].name < result[j].name
	})
	// Move preferred config (current dir) to front
	if abs, err := filepath.Abs(preferredConfig); err == nil {
		for i, p := range result {
			if p.configPath == abs && i > 0 {
				result = append([]projectContext{p}, append(result[:i], result[i+1:]...)...)
				break
			}
		}
	}
	return result, nil
}

func loadProject(p projectContext) (*config.Config, *tracker.Tracker, error) {
	cfg, err := config.Load(p.configPath)
	if err != nil {
		return nil, nil, err
	}
	tr, err := tracker.Load(p.trackerPath)
	if err != nil {
		return nil, nil, err
	}
	return cfg, tr, nil
}

func newBackend(cfg *config.Config) backend.AgentBackend {
	if cfg != nil && cfg.Backend.Type == "codex-tmux" {
		return backend.NewCodexBackend(cfg.Backend.Binary, cfg.Backend.BypassSandbox)
	}
	return &noopBackend{}
}

func (m *model) approveGate() {
	if m.tracker == nil || m.config == nil || len(m.projects) == 0 {
		return
	}
	d := dispatcher.New(m.config, m.tracker)
	sig, _ := d.Evaluate()
	if sig != dispatcher.SignalPhaseGate {
		m.lastErr = fmt.Errorf("no phase gate (signal: %s)", sig)
		return
	}
	d.ApprovePhaseGate()
	proj := m.projects[m.projectIndex]
	if err := m.tracker.SaveTo(proj.trackerPath); err != nil {
		m.lastErr = err
		return
	}
	m.lastErr = nil
}

func (m *model) nextProject() {
	if len(m.projects) <= 1 {
		return
	}
	m.projectIndex = (m.projectIndex + 1) % len(m.projects)
	m.cursor = 0
	m.viewMode = "list"
	m.detailID = ""
	m.lastErr = nil
	m.refresh()
}

func (m *model) killSelected() {
	if len(m.tickets) == 0 || m.backend == nil {
		return
	}
	id := m.tickets[m.cursor].ID
	m.lastErr = m.backend.Kill(backend.AgentHandle{SessionName: "swarm-" + id})
}

func (m *model) respawnSelected() {
	if len(m.tickets) == 0 || m.backend == nil || m.tracker == nil || m.config == nil || len(m.projects) == 0 {
		return
	}
	id := m.tickets[m.cursor].ID
	tk, ok := m.tracker.Get(id)
	if !ok {
		return
	}
	proj := m.projects[m.projectIndex]
	repoDir := absOrJoin(proj.configDir, m.config.Project.Repo)
	// Worktree dir: <repo>-worktrees/<ticketID>
	worktreeDir := repoDir + "-worktrees/" + id
	promptSrc := filepath.Join(absOrJoin(proj.configDir, m.config.Project.PromptDir), id+".md")

	// Ensure worktree exists (recreate if needed)
	if _, serr := os.Stat(worktreeDir); os.IsNotExist(serr) {
		// Remove stale branch and create fresh worktree
		exec.Command("git", "-C", repoDir, "branch", "-D", tk.Branch).Run()
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		out, werr := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", tk.Branch, worktreeDir, m.config.Project.BaseBranch).CombinedOutput()
		if werr != nil {
			m.lastErr = fmt.Errorf("worktree: %s: %w", strings.TrimSpace(string(out)), werr)
			return
		}
	}

	// Copy prompt to worktree
	if pdata, rerr := os.ReadFile(promptSrc); rerr == nil {
		_ = os.WriteFile(filepath.Join(worktreeDir, ".codex-prompt.md"), pdata, 0644)
	}

	_, err := m.backend.Spawn(context.Background(), backend.SpawnConfig{
		TicketID:   id,
		Branch:     tk.Branch,
		WorkDir:    worktreeDir,
		PromptFile: filepath.Join(worktreeDir, ".codex-prompt.md"),
		Model:      m.config.Backend.Model,
		Effort:     m.config.Backend.Effort,
	})
	if err != nil {
		m.lastErr = err
		return
	}
	if err := m.tracker.SetStatus(id, "running"); err != nil {
		m.lastErr = err
		return
	}
	if err := m.tracker.SaveTo(proj.trackerPath); err != nil {
		m.lastErr = err
	}
}

func absOrJoin(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type noopBackend struct{}

func (n *noopBackend) Spawn(context.Context, backend.SpawnConfig) (backend.AgentHandle, error) {
	return backend.AgentHandle{}, fmt.Errorf("backend not configured for spawn")
}
func (n *noopBackend) IsAlive(backend.AgentHandle) bool   { return false }
func (n *noopBackend) HasExited(backend.AgentHandle) bool { return false }
func (n *noopBackend) GetOutput(backend.AgentHandle, int) (string, error) {
	return "", fmt.Errorf("backend not configured")
}
func (n *noopBackend) Kill(backend.AgentHandle) error { return fmt.Errorf("backend not configured") }
func (n *noopBackend) Name() string                   { return "noop" }

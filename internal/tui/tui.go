package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/backend"
	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/progress"
	"github.com/MikeS071/agent-swarm/internal/sysinfo"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/MikeS071/agent-swarm/internal/version"
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
	Phase       int
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

type tuiStatusHints struct {
	Signal        string
	BlockedReason string
	NextAction    string
	TrackerPath   string
	Warning       string
}

type tickMsg time.Time

type model struct {
	tracker        *tracker.Tracker
	config         *config.Config
	backend        backend.AgentBackend
	backendFactory func(backendType string) (backend.AgentBackend, error)
	backendCache   map[string]backend.AgentBackend
	tickets        []ticketRow
	cursor         int
	viewMode       string // "list" | "detail"
	detailID       string
	compact        bool
	width          int
	height         int

	projects     []projectContext
	projectIndex int
	lastErr      error
	page         int
	pageSize     int
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
	projects[idx].trackerPath = cfg.Project.Tracker

	m := model{
		tracker:        tr,
		config:         cfg,
		backend:        newBackend(cfg),
		backendFactory: newBackendFactory(cfg),
		viewMode:       "list",
		compact:        compact,
		pageSize:       20,
		projects:       projects,
		projectIndex:   idx,
	}
	m.resetBackendCache()
	m.rebuildRows()

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.pageSize == 0 {
		m.pageSize = 20
	}
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
				m.page = m.cursor / m.pageSize
			}
			return m, nil
		case tea.KeyDown:
			if m.viewMode != "detail" && m.cursor < len(m.tickets)-1 {
				m.cursor++
				m.page = m.cursor / m.pageSize
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

		raw := msg.String()
		s := strings.ToLower(raw)
		switch {
		case s == "q":
			return m, tea.Quit
		case s == "k":
			m.killSelected()
			m.refresh()
			return m, nil
		case s == "r":
			m.respawnSelected()
			m.refresh()
			return m, nil
		case raw == "A":
			m.approveGate()
			m.refresh()
			return m, nil
		case s == "m":
			m.toggleAutoApprove()
			m.refresh()
			return m, nil
		case s == "p":
			m.nextProject()
			return m, nil
		case s == "[":
			if m.page > 0 {
				m.page--
				m.cursor = m.page * m.pageSize
			}
			return m, nil
		case s == "]":
			maxPage := (len(m.tickets) - 1) / m.pageSize
			if m.page < maxPage {
				m.page++
				m.cursor = m.page * m.pageSize
			}
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
	m.lastErr = nil
	m.projects[m.projectIndex].trackerPath = cfg.Project.Tracker
	m.config = cfg
	m.tracker = tr
	m.backend = newBackend(cfg)
	m.backendFactory = newBackendFactory(cfg)
	m.resetBackendCache()
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
	if m.tracker == nil {
		m.detailOutput = "tracker unavailable"
		return
	}
	tk, ok := m.tracker.Get(m.detailID)
	if !ok {
		m.detailOutput = "ticket not found"
		return
	}
	be := m.backendForTicket(m.detailID, tk)
	if be == nil {
		m.detailOutput = "backend unavailable"
		return
	}
	out, err := be.GetOutput(m.sessionHandleForTicket(m.detailID, tk), 50)
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
			Phase:   tk.Phase,
			Desc:    tk.Desc,
			Status:  tk.Status,
			SHA:     tk.SHA,
			Depends: append([]string(nil), tk.Depends...),
		}

		if tk.Status == "running" {
			be := m.backendForTicket(id, tk)
			h := m.sessionHandleForTicket(id, tk)
			pg := progress.GetProgress(h, be, 0)
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
	modeTag := "manual"
	if m.config.Project.AutoApprove {
		modeTag = "auto"
	}
	title := lipgloss.NewStyle().Bold(true).Render("agent-swarm " + appVersion() + " — " + m.config.Project.Name + " [" + modeTag + "]")
	b.WriteString(title + "\n")
	b.WriteString(fmt.Sprintf("Progress: %s %d/%d (%d%%)\n", renderBarOnly(done, total, 24), done, total, pct))
	b.WriteString(fmt.Sprintf("Phase: %d | Agents: %d/%d | RAM: %s\n", phase, stats.Running, m.config.Project.MaxAgents, ramText))
	hints := m.deriveStatusHints()
	signal := strings.TrimSpace(hints.Signal)
	if signal == "" {
		signal = "UNKNOWN"
	}
	blocked := strings.TrimSpace(hints.BlockedReason)
	if blocked == "" {
		blocked = "NONE"
	}
	next := strings.TrimSpace(hints.NextAction)
	if next == "" {
		next = "none"
	}
	b.WriteString(fmt.Sprintf("Signal: %s | blocked_reason=%s | next_step=%s\n", signal, blocked, next))
	if strings.TrimSpace(hints.TrackerPath) != "" {
		b.WriteString("Tracker: " + hints.TrackerPath + "\n")
	}
	if strings.TrimSpace(hints.Warning) != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Warning: "+hints.Warning) + "\n")
	}
	b.WriteString(strings.Repeat("-", maxInt(20, m.width-2)) + "\n")
	startIdx := m.page * m.pageSize
	endIdx := startIdx + m.pageSize
	if endIdx > len(m.tickets) {
		endIdx = len(m.tickets)
	}
	for i := startIdx; i < endIdx; i++ {
		line := renderTicketRow(m.tickets[i], i == m.cursor, m.compact, m.width)
		b.WriteString(line + "\n")
	}
	totalPages := (len(m.tickets) + m.pageSize - 1) / m.pageSize
	if totalPages > 1 {
		b.WriteString(fmt.Sprintf("Page %d/%d  ", m.page+1, totalPages))
	}
	b.WriteString("q: quit | Enter: view | k: kill | r: respawn | A: approve | m: auto/manual | p: project | [/]: page")
	if m.lastErr != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.lastErr.Error()))
	}
	return b.String()
}

func (m model) deriveStatusHints() tuiStatusHints {
	h := tuiStatusHints{}
	if m.config == nil || m.tracker == nil {
		return h
	}
	if len(m.projects) > 0 {
		h.TrackerPath = m.projects[m.projectIndex].trackerPath
	}
	d := dispatcher.New(m.config, m.tracker)
	sig, _ := d.Evaluate()
	h.Signal = string(sig)
	if strings.TrimSpace(h.Signal) == "" {
		h.Signal = "SPAWN"
	}

	s := m.tracker.Stats()
	switch sig {
	case dispatcher.SignalPhaseGate:
		h.BlockedReason = "PHASE_GATE"
		h.NextAction = "run `agent-swarm go` to approve and continue"
	case dispatcher.SignalBlocked:
		switch {
		case s.Failed > 0:
			h.BlockedReason = "FAILED_TICKETS"
			h.NextAction = "fix/respawn failed tickets, then rerun watchdog"
		case s.Running > 0:
			h.BlockedReason = "WAITING_FOR_RUNNING_TICKETS"
			h.NextAction = "wait for running tickets to finish"
		default:
			h.BlockedReason = "WAITING_FOR_DEPENDENCIES"
			h.NextAction = "inspect dependency chain and prompt/prep gates"
		}
	case dispatcher.SignalSpawn:
		if !d.CanSpawnMore() {
			h.BlockedReason = "CAPACITY_OR_RESOURCE_LIMIT"
			h.NextAction = "reduce running agents or increase capacity"
		} else {
			h.NextAction = "spawnable tickets available"
		}
	case dispatcher.SignalAllDone:
		h.NextAction = "no action needed"
	}

	if div, err := detectTrackerDivergenceTUI(m.config, m.config.Project.Tracker, m.tracker); err != nil {
		h.Warning = err.Error()
	} else if div != nil {
		h.Warning = div.Error()
	}
	return h
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
	maxDesc := 30
	if width > 100 {
		maxDesc = 40
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-1] + "…"
	}
	line := fmt.Sprintf("%s %-12s P%d %-*s", icon, row.ID, row.Phase, maxDesc, desc)
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
		// Fixed left column: icon(2) + ID(8) + phase(3) + desc(maxDesc) + spaces(3) ≈ 46-56
		leftWidth := 8 + 3 + maxDesc + 5
		line = fmt.Sprintf("%-*s %s", leftWidth, line, right)
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
	seenNames := map[string]bool{}
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
		if seenNames[cfg.Project.Name] {
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
		seenNames[cfg.Project.Name] = true
	}

	if strings.TrimSpace(preferredConfig) != "" {
		add(preferredConfig)
	}
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, ".next": true,
		".-worktrees": true, ".decapod": true, "coverage": true,
	}
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasSuffix(d.Name(), "-worktrees") {
				return filepath.SkipDir
			}
			// Limit depth to 3
			if strings.Count(path, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
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
	trackerPath := absOrJoin(filepath.Dir(p.configPath), cfg.Project.Tracker)
	cfg.Project.Tracker = trackerPath

	tr, err := loadTrackerWithFallbackTUI(cfg, trackerPath)
	if err != nil {
		return nil, nil, err
	}
	if div, derr := detectTrackerDivergenceTUI(cfg, trackerPath, tr); derr != nil {
		return nil, nil, derr
	} else if div != nil {
		return nil, nil, fmt.Errorf("%s; use a single source of truth before using TUI", div.Error())
	}
	return cfg, tr, nil
}

type trackerDivergence struct {
	ActivePath    string
	LegacyPath    string
	ActiveTickets int
	LegacyTickets int
}

func (d *trackerDivergence) Error() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("tracker divergence detected: active=%s (%d tickets) vs legacy=%s (%d tickets)",
		d.ActivePath, d.ActiveTickets, d.LegacyPath, d.LegacyTickets)
}

func loadTrackerWithFallbackTUI(cfg *config.Config, trackerPath string) (*tracker.Tracker, error) {
	tr, err := tracker.Load(trackerPath)
	if err == nil {
		return tr, nil
	}

	legacy := filepath.Join(cfg.Project.Repo, "swarm", "tracker.json")
	if filepath.Clean(legacy) == filepath.Clean(trackerPath) {
		return nil, err
	}
	if _, statErr := os.Stat(legacy); statErr != nil {
		return nil, err
	}

	legacyTracker, lerr := tracker.Load(legacy)
	if lerr != nil {
		return nil, err
	}
	if mkErr := os.MkdirAll(filepath.Dir(trackerPath), 0o755); mkErr != nil {
		return nil, fmt.Errorf("mkdir tracker state dir: %w", mkErr)
	}
	if saveErr := legacyTracker.SaveTo(trackerPath); saveErr != nil {
		return nil, fmt.Errorf("import legacy tracker from %s to %s: %w", legacy, trackerPath, saveErr)
	}

	return tracker.Load(trackerPath)
}

func detectTrackerDivergenceTUI(cfg *config.Config, trackerPath string, active *tracker.Tracker) (*trackerDivergence, error) {
	if cfg == nil {
		return nil, nil
	}
	if strings.TrimSpace(cfg.Project.StateDir) == "" {
		return nil, nil
	}
	legacyPath := filepath.Join(cfg.Project.Repo, "swarm", "tracker.json")
	if filepath.Clean(legacyPath) == filepath.Clean(trackerPath) {
		return nil, nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return nil, nil
	}
	legacy, err := tracker.Load(legacyPath)
	if err != nil {
		return nil, nil
	}
	if len(legacy.Tickets) == 0 {
		return nil, nil
	}
	if active == nil {
		active, err = tracker.Load(trackerPath)
		if err != nil {
			return nil, err
		}
	}
	if len(active.Tickets) == 0 {
		return nil, nil
	}
	if trackersEquivalentTUI(active, legacy) {
		return nil, nil
	}
	return &trackerDivergence{
		ActivePath:    trackerPath,
		LegacyPath:    legacyPath,
		ActiveTickets: len(active.Tickets),
		LegacyTickets: len(legacy.Tickets),
	}, nil
}

func trackersEquivalentTUI(a, b *tracker.Tracker) bool {
	if a == nil || b == nil {
		return a == b
	}
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

func newBackend(cfg *config.Config) backend.AgentBackend {
	if cfg == nil {
		return &noopBackend{}
	}
	b, err := backend.Build(cfg.Backend.Type, backend.BuildOptions{
		Binary:        cfg.Backend.Binary,
		BypassSandbox: cfg.Backend.BypassSandbox,
	})
	if err != nil {
		return &noopBackend{}
	}
	return b
}

func newBackendFactory(cfg *config.Config) func(string) (backend.AgentBackend, error) {
	return func(backendType string) (backend.AgentBackend, error) {
		if cfg == nil {
			return backend.Build(backendType, backend.BuildOptions{})
		}
		return backend.Build(backendType, backend.BuildOptions{
			Binary:        cfg.Backend.Binary,
			BypassSandbox: cfg.Backend.BypassSandbox,
		})
	}
}

func (m *model) resetBackendCache() {
	m.backendCache = map[string]backend.AgentBackend{}
	if m.backend != nil {
		m.backendCache[m.defaultBackendType()] = m.backend
	}
}

func (m *model) toggleAutoApprove() {
	if m.config == nil || len(m.projects) == 0 {
		return
	}
	next := !m.config.Project.AutoApprove
	proj := m.projects[m.projectIndex]

	// Write back to swarm.toml (robust even when auto_approve key is missing)
	raw, err := os.ReadFile(proj.configPath)
	if err != nil {
		m.lastErr = fmt.Errorf("read config: %w", err)
		return
	}
	updated, err := setProjectAutoApprove(string(raw), next)
	if err != nil {
		m.lastErr = fmt.Errorf("toggle auto/manual: %w", err)
		return
	}
	if err := os.WriteFile(proj.configPath, []byte(updated), 0o644); err != nil {
		m.lastErr = fmt.Errorf("write config: %w", err)
		return
	}

	// Optimistically update local state so title bar flips immediately.
	m.config.Project.AutoApprove = next

	// Reload from disk so title bar and state stay in sync.
	m.refresh()
	if m.lastErr != nil {
		return
	}

	mode := "manual (phase gates)"
	if next {
		mode = "auto (no gates)"
	}
	m.lastErr = fmt.Errorf("🔄 Switched to %s", mode)
}

func setProjectAutoApprove(raw string, value bool) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("empty config")
	}
	lines := strings.Split(raw, "\n")
	projectStart := -1
	projectEnd := len(lines)
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "[project]" {
			projectStart = i
			continue
		}
		if projectStart >= 0 && strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			projectEnd = i
			break
		}
	}
	if projectStart < 0 {
		return "", fmt.Errorf("missing [project] section")
	}

	newLine := fmt.Sprintf("auto_approve = %v", value)
	for i := projectStart + 1; i < projectEnd; i++ {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "auto_approve") {
			indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			lines[i] = indent + newLine
			return strings.Join(lines, "\n"), nil
		}
	}

	updated := append([]string{}, lines[:projectEnd]...)
	updated = append(updated, newLine)
	updated = append(updated, lines[projectEnd:]...)
	return strings.Join(updated, "\n"), nil
}

func (m *model) approveGate() {
	if m.tracker == nil || m.config == nil || len(m.projects) == 0 {
		return
	}
	d := dispatcher.New(m.config, m.tracker)
	sig, _ := d.Evaluate()
	if sig != dispatcher.SignalPhaseGate {
		phase := m.tracker.ActivePhase()
		m.lastErr = fmt.Errorf("phase %d not at gate yet (signal: %s) — all tickets must complete first", phase, sig)
		return
	}
	phase := m.tracker.ActivePhase()
	d.ApprovePhaseGate()
	proj := m.projects[m.projectIndex]
	if err := m.tracker.SaveTo(proj.trackerPath); err != nil {
		m.lastErr = err
		return
	}
	// Immediately spawn available tickets in the new phase
	d2 := dispatcher.New(m.config, m.tracker)
	_, spawnable := d2.Evaluate()
	spawned := 0
	proj = m.projects[m.projectIndex]
	repoDir := absOrJoin(proj.configDir, m.config.Project.Repo)
	for _, tid := range spawnable {
		if !d2.CanSpawnMore() {
			break
		}
		tk, ok := m.tracker.Get(tid)
		if !ok {
			continue
		}
		worktreeDir := repoDir + "-worktrees/" + tid
		promptSrc := filepath.Join(absOrJoin(proj.configDir, m.config.Project.PromptDir), tid+".md")
		if _, serr := os.Stat(worktreeDir); os.IsNotExist(serr) {
			exec.Command("git", "-C", repoDir, "branch", "-D", tk.Branch).Run()
			exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
			out, werr := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", tk.Branch, worktreeDir, m.config.Project.BaseBranch).CombinedOutput()
			if werr != nil {
				m.lastErr = fmt.Errorf("worktree: %s: %w", strings.TrimSpace(string(out)), werr)
				continue
			}
		}
		if pdata, rerr := os.ReadFile(promptSrc); rerr == nil {
			_ = os.WriteFile(filepath.Join(worktreeDir, ".codex-prompt.md"), pdata, 0644)
		}
		be := m.backendForTicket(tid, tk)
		handle, err := be.Spawn(context.Background(), backend.SpawnConfig{
			TicketID:    tid,
			Branch:      tk.Branch,
			WorkDir:     worktreeDir,
			PromptFile:  filepath.Join(worktreeDir, ".codex-prompt.md"),
			Model:       m.modelForTicket(tk),
			Effort:      m.config.Backend.Effort,
			ProjectName: m.config.Project.Name,
		})
		if err != nil {
			continue
		}
		updated := tk
		updated.Status = tracker.StatusRunning
		updated.StartedAt = time.Now().UTC().Format(time.RFC3339)
		updated.SessionName = strings.TrimSpace(handle.SessionName)
		if updated.SessionName == "" {
			updated.SessionName = m.sessionName(tid)
		}
		updated.SessionBackend = m.backendTypeForTicket(tk)
		updated.SessionModel = m.modelForTicket(tk)
		m.tracker.Tickets[tid] = updated
		spawned++
	}
	_ = m.tracker.SaveTo(proj.trackerPath)
	m.lastErr = fmt.Errorf("✅ Phase %d approved — spawned %d tickets", phase, spawned)
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

func (m *model) sessionName(ticketID string) string {
	if m.config != nil && m.config.Project.Name != "" {
		return "swarm-" + m.config.Project.Name + "_" + ticketID
	}
	return "swarm-" + ticketID
}

func (m *model) defaultBackendType() string {
	if m == nil || m.config == nil {
		return backend.TypeCodexTmux
	}
	return normalizeBackendType(m.config.Backend.Type)
}

func normalizeBackendType(v string) string {
	n := strings.ToLower(strings.TrimSpace(v))
	if n == "" {
		return backend.TypeCodexTmux
	}
	return n
}

func (m *model) backendTypeForTicket(tk tracker.Ticket) string {
	if bt := strings.TrimSpace(tk.SessionBackend); bt != "" {
		return normalizeBackendType(bt)
	}
	return m.defaultBackendType()
}

func (m *model) modelForTicket(tk tracker.Ticket) string {
	if model := strings.TrimSpace(tk.SessionModel); model != "" {
		return model
	}
	if m.config == nil {
		return ""
	}
	return m.config.Backend.Model
}

func (m *model) backendForType(backendType string) backend.AgentBackend {
	bt := normalizeBackendType(backendType)
	if m.backendCache == nil {
		m.resetBackendCache()
	}
	if be, ok := m.backendCache[bt]; ok && be != nil {
		return be
	}
	if bt == m.defaultBackendType() && m.backend != nil {
		m.backendCache[bt] = m.backend
		return m.backend
	}
	if m.backendFactory == nil {
		return &noopBackend{}
	}
	be, err := m.backendFactory(bt)
	if err != nil {
		return &noopBackend{}
	}
	m.backendCache[bt] = be
	return be
}

func (m *model) backendForTicket(_ string, tk tracker.Ticket) backend.AgentBackend {
	return m.backendForType(m.backendTypeForTicket(tk))
}

func (m *model) sessionHandleForTicket(ticketID string, tk tracker.Ticket) backend.AgentHandle {
	session := strings.TrimSpace(tk.SessionName)
	if session == "" {
		session = m.sessionName(ticketID)
	}
	h := backend.AgentHandle{SessionName: session}
	if ts := strings.TrimSpace(tk.StartedAt); ts != "" {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			h.StartedAt = parsed
		}
	}
	return h
}

func (m *model) killSelected() {
	if len(m.tickets) == 0 || m.tracker == nil {
		return
	}
	id := m.tickets[m.cursor].ID
	tk, ok := m.tracker.Get(id)
	if !ok {
		return
	}
	be := m.backendForTicket(id, tk)
	m.lastErr = be.Kill(m.sessionHandleForTicket(id, tk))
}

func (m *model) respawnSelected() {
	if len(m.tickets) == 0 || m.tracker == nil || m.config == nil || len(m.projects) == 0 {
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

	be := m.backendForTicket(id, tk)
	handle, err := be.Spawn(context.Background(), backend.SpawnConfig{
		TicketID:    id,
		Branch:      tk.Branch,
		WorkDir:     worktreeDir,
		PromptFile:  filepath.Join(worktreeDir, ".codex-prompt.md"),
		Model:       m.modelForTicket(tk),
		Effort:      m.config.Backend.Effort,
		ProjectName: m.config.Project.Name,
	})
	if err != nil {
		m.lastErr = err
		return
	}
	tk.Status = tracker.StatusRunning
	tk.StartedAt = time.Now().UTC().Format(time.RFC3339)
	tk.SessionName = strings.TrimSpace(handle.SessionName)
	if tk.SessionName == "" {
		tk.SessionName = m.sessionName(id)
	}
	tk.SessionBackend = m.backendTypeForTicket(tk)
	tk.SessionModel = m.modelForTicket(tk)
	m.tracker.Tickets[id] = tk
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

func appVersion() string {
	return version.String()
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

func (m *model) archiveDone() {
	if m.tracker == nil || len(m.projects) == 0 {
		return
	}
	proj := m.projects[m.projectIndex]
	archivePath := tracker.DefaultArchivePath(proj.trackerPath)
	_, err := tracker.ArchiveDoneTickets(proj.trackerPath, archivePath, tracker.ArchiveOptions{})
	if err != nil {
		m.lastErr = err
	}
}

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

const integrateStateFilename = ".swarm-integrate-state.json"

type integrateState struct {
	Base      string   `json:"base"`
	Branch    string   `json:"branch"`
	Tickets   []string `json:"tickets"`
	Branches  []string `json:"branches"`
	NextIndex int      `json:"next_index"`
}

type mergeSummaryRow struct {
	Ticket string
	Branch string
	Result string
}

type integrateRunner struct {
	repoDir    string
	configPath string
	base       string
	branch     string
	dryRun     bool
	isContinue bool
	out        io.Writer
}

func newIntegrateRunner(repoDir, base, branch string, dryRun, isContinue bool, out io.Writer) *integrateRunner {
	if out == nil {
		out = os.Stdout
	}
	if base == "" {
		base = "main"
	}
	if branch == "" {
		branch = "integration/v1"
	}
	return &integrateRunner{
		repoDir:    repoDir,
		configPath: filepath.Join(repoDir, "swarm.toml"),
		base:       base,
		branch:     branch,
		dryRun:     dryRun,
		isContinue: isContinue,
		out:        out,
	}
}

func (r *integrateRunner) Run() error {
	cfg, err := config.Load(r.configPath)
	if err != nil {
		return err
	}
	tr, err := tracker.Load(resolveFromConfig(r.configPath, cfg.Project.Tracker))
	if err != nil {
		return err
	}

	state := &integrateState{}
	start := 0
	if r.isContinue {
		state, err = r.loadState()
		if err != nil {
			return err
		}
		r.base = state.Base
		r.branch = state.Branch
		start = state.NextIndex
		if len(state.Tickets) == 0 {
			return errors.New("resume state has no tickets")
		}
	} else {
		ids, branches := doneTicketsInOrder(tr)
		state = &integrateState{
			Base:      r.base,
			Branch:    r.branch,
			Tickets:   ids,
			Branches:  branches,
			NextIndex: 0,
		}
	}

	summary := make([]mergeSummaryRow, len(state.Tickets))
	for i := range state.Tickets {
		branchName := "feat/" + state.Tickets[i]
		if i < len(state.Branches) && strings.TrimSpace(state.Branches[i]) != "" {
			branchName = strings.TrimSpace(state.Branches[i])
		}
		summary[i] = mergeSummaryRow{Ticket: state.Tickets[i], Branch: branchName, Result: "skipped"}
	}

	if !r.dryRun {
		if r.isContinue {
			if err := r.git("checkout", r.branch); err != nil {
				return err
			}
		} else {
			if err := r.git("checkout", "-B", r.branch, r.base); err != nil {
				return err
			}
		}
	}

	for i := start; i < len(summary); i++ {
		if r.dryRun {
			summary[i].Result = "skipped"
			continue
		}

		if err := r.git("merge", summary[i].Branch, "--no-ff", "--no-edit"); err != nil {
			summary[i].Result = "conflict"
			state.NextIndex = i + 1
			if saveErr := r.saveState(state); saveErr != nil {
				return fmt.Errorf("merge failed (%w) and state save failed: %v", err, saveErr)
			}
			files, _ := r.conflictedFiles()
			r.printSummary(summary)
			r.printConflictHelp(summary[i], files)
			return fmt.Errorf("conflict merging %s", summary[i].Branch)
		}
		summary[i].Result = "ok"
	}

	r.printSummary(summary)

	if r.dryRun {
		return nil
	}
	if strings.TrimSpace(cfg.Integration.VerifyCmd) != "" {
		if err := r.runShell(cfg.Integration.VerifyCmd); err != nil {
			return fmt.Errorf("verify command failed: %w", err)
		}
	}
	if err := r.git("push", "-u", "origin", r.branch); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(r.repoDir, integrateStateFilename)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.TrimSpace(cfg.Integration.AuditTicket) != "" {
		fmt.Fprintf(r.out, "audit ticket is now spawnable: %s\n", cfg.Integration.AuditTicket)
	}
	return nil
}

func doneTicketsInOrder(tr *tracker.Tracker) ([]string, []string) {
	order := tr.DependencyOrder()
	ids := make([]string, 0, len(order))
	branches := make([]string, 0, len(order))
	for _, id := range order {
		tk, ok := tr.Tickets[id]
		if !ok || tk.Status != tracker.StatusDone {
			continue
		}
		ids = append(ids, id)
		branch := strings.TrimSpace(tk.Branch)
		if branch == "" {
			branch = "feat/" + id
		}
		branches = append(branches, branch)
	}
	return ids, branches
}

func (r *integrateRunner) loadState() (*integrateState, error) {
	path := filepath.Join(r.repoDir, integrateStateFilename)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", integrateStateFilename, err)
	}
	var st integrateState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", integrateStateFilename, err)
	}
	return &st, nil
}

func (r *integrateRunner) saveState(st *integrateState) error {
	if st == nil {
		return errors.New("state is nil")
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.repoDir, integrateStateFilename), b, 0o644)
}

func (r *integrateRunner) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w (%s)", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *integrateRunner) conflictedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = r.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git conflict files: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}

func (r *integrateRunner) runShell(command string) error {
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = r.repoDir
	cmd.Stdout = r.out
	cmd.Stderr = r.out
	return cmd.Run()
}

func (r *integrateRunner) printSummary(rows []mergeSummaryRow) {
	fmt.Fprintln(r.out, "")
	fmt.Fprintln(r.out, "Summary:")
	w := tabwriter.NewWriter(r.out, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "TICKET\tBRANCH\tRESULT")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\n", row.Ticket, row.Branch, row.Result)
	}
	_ = w.Flush()
}

func (r *integrateRunner) printConflictHelp(row mergeSummaryRow, files []string) {
	fmt.Fprintf(r.out, "\n❌ Conflict merging %s into %s\n\n", row.Branch, r.branch)
	fmt.Fprintln(r.out, "Conflicted files:")
	if len(files) == 0 {
		fmt.Fprintln(r.out, "  - (unable to detect files)")
	} else {
		for _, file := range files {
			fmt.Fprintf(r.out, "  - %s\n", file)
		}
	}
	fmt.Fprintln(r.out, "")
	fmt.Fprintln(r.out, "To resolve manually:")
	fmt.Fprintf(r.out, "  cd %s\n", r.repoDir)
	fmt.Fprintf(r.out, "  git merge %s\n", row.Branch)
	fmt.Fprintln(r.out, "  # fix conflicts")
	fmt.Fprintln(r.out, "  git add . && git commit")
	fmt.Fprintln(r.out, "  swarm integrate --continue")
}

var integrateBase string
var integrateBranch string
var integrateDryRun bool
var integrateContinue bool

var integrateCmd = &cobra.Command{
	Use:   "integrate",
	Short: "Merge done ticket branches into an integration branch",
	RunE: func(_ *cobra.Command, _ []string) error {
		absCfg, err := filepath.Abs(cfgFile)
		if err != nil {
			return err
		}
		runner := newIntegrateRunner(filepath.Dir(absCfg), integrateBase, integrateBranch, integrateDryRun, integrateContinue, os.Stdout)
		runner.configPath = absCfg
		return runner.Run()
	},
}

func init() {
	integrateCmd.Flags().StringVar(&integrateBase, "base", "main", "base branch for integration")
	integrateCmd.Flags().StringVar(&integrateBranch, "branch", "integration/v1", "integration branch name")
	integrateCmd.Flags().BoolVar(&integrateDryRun, "dry-run", false, "print merge order without executing merges")
	integrateCmd.Flags().BoolVar(&integrateContinue, "continue", false, "resume from saved integration state")
	rootCmd.AddCommand(integrateCmd)
}

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestRunPrepChecks_PostBuildMemEnforcesProfileAndPromptIntent(t *testing.T) {
	root := t.TempDir()
	promptDir := filepath.Join(root, "swarm", "prompts")
	writePath(t, filepath.Join(root, ".agents", "lifecycle-policy.toml"), `[profiles.by_ticket_type]
int = "code-agent"
gap = "code-reviewer"
tst = "e2e-runner"
review = "code-reviewer"
sec = "security-reviewer"
doc = "doc-updater"
clean = "refactor-cleaner"
mem = "doc-updater"
`)
	writePath(t, filepath.Join(promptDir, "mem-tp.md"), `# mem-tp
## Objective
Run post-build step "mem" for feature "tp".
## Dependencies
clean-tp, doc-tp
## Scope
- generic
## Verify
go build ./...
`)
	writePath(t, filepath.Join(root, ".agents", "profiles", "code-reviewer.md"), `---
name: code-reviewer
description: reviewer
mode: Review
---
`)

	cfg := config.Default()
	cfg.Project.Repo = root
	cfg.Project.RequireExplicitRole = true
	cfg.Project.RequireVerifyCmd = true

	tr := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"mem-tp": {
			Status:    tracker.StatusTodo,
			Type:      "mem",
			Profile:   "code-reviewer",
			VerifyCmd: "go build ./...",
		},
	})

	issues := runPrepChecks(cfg, tr, promptDir)
	if len(issues) == 0 {
		t.Fatalf("expected prep issues for mem intent mismatch")
	}

	hasProfileMismatch := false
	hasLessonsCheck := false
	for _, is := range issues {
		if is.Field == "profile" && containsAll(is.Reason, "should use profile", "doc-updater") {
			hasProfileMismatch = true
		}
		if is.Field == "prompt" && containsAll(is.Reason, "missing required snippet", "docs/lessons-learned.md") {
			hasLessonsCheck = true
		}
	}
	if !hasProfileMismatch {
		t.Fatalf("expected profile mismatch issue, got: %+v", issues)
	}
	if !hasLessonsCheck {
		t.Fatalf("expected lessons-learned prompt issue, got: %+v", issues)
	}
}

func TestRunPrepChecks_DocTicketValidIntentPasses(t *testing.T) {
	root := t.TempDir()
	promptDir := filepath.Join(root, "swarm", "prompts")
	writePath(t, filepath.Join(root, ".agents", "lifecycle-policy.toml"), `[profiles.by_ticket_type]
int = "code-agent"
gap = "code-reviewer"
tst = "e2e-runner"
review = "code-reviewer"
sec = "security-reviewer"
doc = "doc-updater"
clean = "refactor-cleaner"
mem = "doc-updater"
`)
	writePath(t, filepath.Join(promptDir, "doc-tp.md"), `# doc-tp
## Objective
Update docs
## Dependencies
review-tp, sec-tp
## Scope
- update docs/user-guide.md
- update docs/technical.md
- update docs/release-notes.md
- produce doc-report.md
## Verify
go build ./...
`)
	writePath(t, filepath.Join(root, ".agents", "profiles", "doc-updater.md"), `---
name: doc-updater
description: docs
tools: ["Read"]
mode: Development
---
`)

	cfg := config.Default()
	cfg.Project.Repo = root
	cfg.Project.RequireExplicitRole = true
	cfg.Project.RequireVerifyCmd = true

	tr := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"doc-tp": {
			Status:    tracker.StatusTodo,
			Type:      "doc",
			Profile:   "doc-updater",
			VerifyCmd: "go build ./...",
		},
	})

	issues := runPrepChecks(cfg, tr, promptDir)
	if len(issues) != 0 {
		t.Fatalf("expected no prep issues, got: %+v", issues)
	}
}

func TestRunPrepChecks_ProfileFrontmatterMustMatch(t *testing.T) {
	root := t.TempDir()
	promptDir := filepath.Join(root, "swarm", "prompts")
	writePath(t, filepath.Join(root, ".agents", "lifecycle-policy.toml"), `[profiles.by_ticket_type]
int = "code-agent"
gap = "code-reviewer"
tst = "e2e-runner"
review = "code-reviewer"
sec = "security-reviewer"
doc = "doc-updater"
clean = "refactor-cleaner"
mem = "doc-updater"
`)
	writePath(t, filepath.Join(promptDir, "tp-01.md"), `# tp-01
## Objective
Implement
## Dependencies
none
## Scope
- code
## Verify
go test ./...
`)
	writePath(t, filepath.Join(root, ".agents", "profiles", "code-agent.md"), `---
name: wrong-name
description: 
---
`)

	cfg := config.Default()
	cfg.Project.Repo = root
	cfg.Project.RequireExplicitRole = true
	cfg.Project.RequireVerifyCmd = true

	tr := tracker.NewFromPtrs("p", map[string]*tracker.Ticket{
		"tp-01": {
			Status:    tracker.StatusTodo,
			Profile:   "code-agent",
			VerifyCmd: "go test ./...",
		},
	})

	issues := runPrepChecks(cfg, tr, promptDir)
	if len(issues) == 0 {
		t.Fatalf("expected profile frontmatter issues")
	}

	hasNameMismatch := false
	hasMissingMode := false
	for _, is := range issues {
		if is.Field == "profile" && containsAll(is.Reason, "name mismatch") {
			hasNameMismatch = true
		}
		if is.Field == "profile" && containsAll(is.Reason, "missing mode") {
			hasMissingMode = true
		}
	}
	if !hasNameMismatch || !hasMissingMode {
		t.Fatalf("expected name mismatch + missing mode issues, got: %+v", issues)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !containsFold(s, sub) {
			return false
		}
	}
	return true
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexFold(s, sub) >= 0))
}

func indexFold(s, sub string) int {
	// Simple case-insensitive search without regexp.
	sl := []rune(s)
	subl := []rune(sub)
	for i := 0; i+len(subl) <= len(sl); i++ {
		ok := true
		for j := range subl {
			a := sl[i+j]
			b := subl[j]
			if 'A' <= a && a <= 'Z' {
				a = a - 'A' + 'a'
			}
			if 'A' <= b && b <= 'Z' {
				b = b - 'A' + 'a'
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

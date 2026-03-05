package prompts

import (
	"bytes"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestCompileDeterministicAndOrdered(t *testing.T) {
	t.Parallel()

	tk := tracker.Ticket{
		Status:    tracker.StatusTodo,
		Phase:     1,
		Depends:   []string{"tp-01"},
		Type:      "feature",
		Desc:      "Implement prompts build deterministic compiler",
		RunID:     "run-2026-03-04T10-32-00Z",
		Role:      "backend",
		Objective: "Compile deterministic execution prompts from structured tracker fields.",
		ScopeIn: []string{
			"cmd/prompts.go",
			"cmd/prompts_cmd.go",
			"internal/prompts/*.go",
		},
		ScopeOut: []string{
			"No watchdog runtime behavior changes",
		},
		FilesToTouch: []string{
			"cmd/prompts.go",
			"cmd/prompts_cmd.go",
			"internal/prompts/compiler.go",
		},
		ReferenceFiles: []string{
			"docs/SWARM-TICKET-PREP-V2-SPEC.md",
			"swarm/prompts/tp-03.md",
		},
		ImplementationSteps: []string{
			"Add prompts build command",
			"Compile deterministic section order",
			"Write prompt manifest",
		},
		TestsToAddOrUpdate: []string{
			"cmd/prompts_build_test.go",
			"internal/prompts/compiler_test.go",
		},
		VerifyCmd: "go test ./cmd/... ./internal/...",
		AcceptanceCriteria: []string{
			"prompts build writes prompt and manifest files",
			"compiled prompt includes explicit file scope and verify command",
		},
		Constraints: []string{
			"Fail closed in strict mode for missing required fields",
		},
	}

	policy := PolicyContext{
		ProjectName:          "agent-swarm",
		BaseBranch:           "main",
		SpecFile:             "SPEC.md",
		DefaultVerifyCommand: "go test ./...",
		AgentContextPointers: []string{
			"swarm.toml",
			".agents/AGENTS.md",
		},
	}

	first, err := Compile("tp-03", tk, policy, CompileOptions{Strict: true})
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := Compile("tp-03", tk, policy, CompileOptions{Strict: true})
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}

	if !bytes.Equal(first.Prompt, second.Prompt) {
		t.Fatal("prompt output is not deterministic")
	}
	if !bytes.Equal(first.Manifest, second.Manifest) {
		t.Fatal("manifest output is not deterministic")
	}

	prompt := string(first.Prompt)
	orderedSections := []string{
		"## Header",
		"## Objective",
		"## Scope In",
		"## Scope Out",
		"## Files to Touch",
		"## Reference Files",
		"## Implementation Steps",
		"## Tests to Add/Update",
		"## Verify Command",
		"## Acceptance Criteria",
		"## Constraints",
		"## Commit Contract",
		"## Forbidden Actions",
		"## Agent Context Pointers",
	}

	last := -1
	for _, section := range orderedSections {
		idx := strings.Index(prompt, section)
		if idx < 0 {
			t.Fatalf("missing section %q in prompt:\n%s", section, prompt)
		}
		if idx <= last {
			t.Fatalf("section %q is out of order", section)
		}
		last = idx
	}

	if !strings.Contains(prompt, "cmd/prompts.go") {
		t.Fatalf("prompt missing files_to_touch content: %s", prompt)
	}
	if !strings.Contains(prompt, "go test ./cmd/... ./internal/...") {
		t.Fatalf("prompt missing verify command: %s", prompt)
	}
	if !strings.Contains(prompt, "Fail closed in strict mode") {
		t.Fatalf("prompt missing constraints: %s", prompt)
	}
}

func TestCompileStrictFailsWhenRequiredDataMissing(t *testing.T) {
	t.Parallel()

	_, err := Compile("tp-03", tracker.Ticket{
		Status: tracker.StatusTodo,
		Desc:   "Incomplete ticket",
	}, PolicyContext{
		ProjectName: "agent-swarm",
	}, CompileOptions{Strict: true})
	if err == nil {
		t.Fatal("expected strict compile to fail")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Fatalf("unexpected strict error: %v", err)
	}
	if !strings.Contains(err.Error(), "objective") {
		t.Fatalf("strict error should include missing objective: %v", err)
	}
	if !strings.Contains(err.Error(), "files_to_touch") {
		t.Fatalf("strict error should include missing files_to_touch: %v", err)
	}
}

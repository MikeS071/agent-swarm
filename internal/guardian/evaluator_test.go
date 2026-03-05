package guardian

import (
	"context"
	"testing"
)

func TestBeforeSpawnBlocksMissingTicketScopeVerify(t *testing.T) {
	ev := NewStrictEvaluator()
	dec, err := ev.Evaluate(context.Background(), Request{
		Event: EventBeforeSpawn,
		Context: map[string]any{
			"profile":    "code-agent",
			"verify_cmd": "go test ./...",
			"desc":       "Implement guardian checks",
			"prompt":     "## Objective\n...\n## Dependencies\n...\n## Scope\n...\n## Verify\n...",
		},
	})
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if dec.Result != ResultBlock || dec.RuleID != "ticket_desc_has_scope_and_verify" {
		t.Fatalf("expected ticket scope/verify block, got %+v", dec)
	}
}

func TestBeforeSpawnBlocksMissingPromptSections(t *testing.T) {
	ev := NewStrictEvaluator()
	dec, err := ev.Evaluate(context.Background(), Request{
		Event: EventBeforeSpawn,
		Context: map[string]any{
			"profile":    "code-agent",
			"verify_cmd": "go test ./...",
			"desc":       "Scope: watchdog. Verify: go test",
			"prompt":     "## Objective\n...",
		},
	})
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if dec.Result != ResultBlock || dec.RuleID != "prompt_template_sections" {
		t.Fatalf("expected prompt section block, got %+v", dec)
	}
}

func TestBeforeSpawnAllowWithValidDescAndPrompt(t *testing.T) {
	ev := NewStrictEvaluator()
	dec, err := ev.Evaluate(context.Background(), Request{
		Event: EventBeforeSpawn,
		Context: map[string]any{
			"profile":    "code-agent",
			"verify_cmd": "go test ./...",
			"desc":       "Scope: watchdog spawn checks. Verify: go test ./internal/watchdog/...",
			"prompt":     "## Objective\n...\n## Dependencies\n...\n## Scope\n...\n## Verify\n...",
		},
	})
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if dec.Result != ResultAllow {
		t.Fatalf("expected allow, got %+v", dec)
	}
}

package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendGuardianEvent(t *testing.T) {
	base := t.TempDir()
	err := AppendGuardianEvent(base, GuardianEvent{
		EnforcementPoint: "transition",
		RuleID:           "phase_has_int_gap_tst_chain",
		Result:           "BLOCK",
		Reason:           "missing chain",
		Target:           "phase:2",
	})
	if err != nil {
		t.Fatalf("append guardian event: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(base, "guardian-events.jsonl"))
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	line := strings.TrimSpace(string(b))
	if !strings.Contains(line, "phase_has_int_gap_tst_chain") || !strings.Contains(line, "\"result\":\"BLOCK\"") {
		t.Fatalf("unexpected event line: %s", line)
	}
}

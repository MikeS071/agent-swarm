package tui

import "testing"

func TestSetProjectAutoApprove_ReplacesExisting(t *testing.T) {
	raw := `[project]
name = "x"
auto_approve = false
max_agents = 4

[backend]
type = "codex-tmux"
`
	out, err := setProjectAutoApprove(raw, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out; !contains(got, "auto_approve = true") {
		t.Fatalf("expected auto_approve=true in output:\n%s", got)
	}
}

func TestSetProjectAutoApprove_InsertsWhenMissing(t *testing.T) {
	raw := `[project]
name = "x"
max_agents = 4

[backend]
type = "codex-tmux"
`
	out, err := setProjectAutoApprove(raw, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(out, "auto_approve = false") {
		t.Fatalf("expected auto_approve line inserted:\n%s", out)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

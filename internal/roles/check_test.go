package roles

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func TestResolveTicketRole(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   tracker.Ticket
		want string
	}{
		{
			name: "returns trimmed profile",
			in:   tracker.Ticket{Profile: "  code-agent  "},
			want: "code-agent",
		},
		{
			name: "returns empty for blank profile",
			in:   tracker.Ticket{Profile: "   "},
			want: "",
		},
		{
			name: "returns empty for missing profile",
			in:   tracker.Ticket{},
			want: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveTicketRole(tc.in)
			if got != tc.want {
				t.Fatalf("ResolveTicketRole() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prepare func(t *testing.T, root string) *tracker.Tracker
		assert  func(t *testing.T, report Report)
	}{
		{
			name: "ok when base rules and role assets exist",
			prepare: func(t *testing.T, root string) *tracker.Tracker {
				writeBaseRules(t, root)
				writeRoleAssets(t, root, "code-agent")
				return &tracker.Tracker{
					Project: "test",
					Tickets: map[string]tracker.Ticket{
						"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
					},
				}
			},
			assert: func(t *testing.T, report Report) {
				if !report.OK() {
					t.Fatalf("expected OK report, got failures: %#v", report.Failures)
				}
				if !reflect.DeepEqual(report.Roles, []string{"code-agent"}) {
					t.Fatalf("roles = %#v, want %#v", report.Roles, []string{"code-agent"})
				}
			},
		},
		{
			name: "reports missing role assets for each used role",
			prepare: func(t *testing.T, root string) *tracker.Tracker {
				writeBaseRules(t, root)
				return &tracker.Tracker{
					Project: "test",
					Tickets: map[string]tracker.Ticket{
						"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
						"sw-02": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
					},
				}
			},
			assert: func(t *testing.T, report Report) {
				if report.OK() {
					t.Fatal("expected failures but report was OK")
				}
				if len(report.Failures) != 3 {
					t.Fatalf("failures = %d, want 3", len(report.Failures))
				}
				for _, failure := range report.Failures {
					if failure.Role != "code-agent" {
						t.Fatalf("role = %q, want code-agent", failure.Role)
					}
					if !reflect.DeepEqual(failure.Tickets, []string{"sw-01", "sw-02"}) {
						t.Fatalf("tickets = %#v, want %#v", failure.Tickets, []string{"sw-01", "sw-02"})
					}
				}
			},
		},
		{
			name: "reports missing base rules",
			prepare: func(t *testing.T, root string) *tracker.Tracker {
				writeRoleAssets(t, root, "code-agent")
				return &tracker.Tracker{
					Project: "test",
					Tickets: map[string]tracker.Ticket{
						"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: "code-agent"},
					},
				}
			},
			assert: func(t *testing.T, report Report) {
				if report.OK() {
					t.Fatal("expected failures but report was OK")
				}
				baseFailures := 0
				for _, failure := range report.Failures {
					if failure.Asset == AssetBaseRule {
						baseFailures++
					}
				}
				if baseFailures != len(RequiredBaseRuleFiles()) {
					t.Fatalf("base rule failures = %d, want %d", baseFailures, len(RequiredBaseRuleFiles()))
				}
			},
		},
		{
			name: "reports invalid role reference",
			prepare: func(t *testing.T, root string) *tracker.Tracker {
				writeBaseRules(t, root)
				return &tracker.Tracker{
					Project: "test",
					Tickets: map[string]tracker.Ticket{
						"sw-01": {Status: tracker.StatusTodo, Phase: 1, Profile: "../escape"},
					},
				}
			},
			assert: func(t *testing.T, report Report) {
				if report.OK() {
					t.Fatal("expected failures but report was OK")
				}
				found := false
				for _, failure := range report.Failures {
					if failure.Asset == AssetRoleRef && failure.Role == "../escape" && strings.Contains(failure.Reason, "invalid role name") {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected invalid role failure, got %#v", report.Failures)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tr := tc.prepare(t, root)
			report := Check(root, tr)
			tc.assert(t, report)
		})
	}
}

func writeBaseRules(t *testing.T, root string) {
	t.Helper()
	for _, name := range RequiredBaseRuleFiles() {
		p := filepath.Join(root, ".codex", "rules", "common", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatalf("write base rule %s: %v", p, err)
		}
	}
}

func writeRoleAssets(t *testing.T, root, role string) {
	t.Helper()
	profilePath := filepath.Join(root, ".agents", "profiles", role+".md")
	rolePath := filepath.Join(root, ".agents", "roles", role+".yaml")
	rulePath := filepath.Join(root, ".codex", "rules", role+".md")

	for _, path := range []string{profilePath, rolePath, rulePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

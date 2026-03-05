package schema

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePolicyFile(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write policy file: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantErr     bool
		errContains string
		assertErr   func(t *testing.T, err error)
	}{
		{
			name: "valid policy",
			body: `{
  "version": 2,
  "mode": "advisory",
  "settings": {
    "fail_closed": true,
    "cache_ttl_seconds": 30,
    "max_evidence_bytes": 4096
  },
  "enforcement_points": ["before_spawn", "transition"],
  "contexts": {
    "default": {"severity": "warn"}
  },
  "rules": [
    {
      "id": "rule-1",
      "enabled": true,
      "description": "require scope",
      "severity": "block",
      "enforcement_points": ["before_spawn"],
      "target": {
        "kind": "ticket",
        "source": "swarm/tracker.json",
        "fields": ["objective", "scope", "verify"]
      },
      "check": {
        "type": "ticket_fields",
        "params": {"required_fields": ["objective", "scope", "verify"]}
      },
      "pass_when": {
        "op": "all",
        "conditions": [
          {"metric": "required_fields_present", "equals": true}
        ]
      },
      "fail_reason": "missing required fields",
      "evidence": {
        "kind": "json",
        "path": "evidence/rule-1.json"
      }
    }
  ],
  "overrides": {
    "enabled": true,
    "require_reason": true,
    "require_expiry": true,
    "max_duration_hours": 24,
    "store": "approvals.json"
  },
  "events": {
    "file": "guardian-events.jsonl",
    "include": ["timestamp", "result", "reason"]
  }
}`,
		},
		{
			name:        "malformed payload",
			body:        `{"version":2,"rules":[}`,
			wantErr:     true,
			errContains: "invalid JSON",
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				var verrs ValidationErrors
				if !errors.As(err, &verrs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
			},
		},
		{
			name: "unknown nested rule field",
			body: `{
  "version": 2,
  "mode": "enforce",
  "enforcement_points": ["before_spawn"],
  "rules": [
    {
      "id": "rule-1",
      "enabled": true,
      "description": "x",
      "severity": "warn",
      "enforcement_points": ["before_spawn"],
      "target": {
        "kind": "file",
        "paths": ["**/*.go"]
      },
      "check": {
        "type": "regex",
        "params": {"pattern": "scope"}
      },
      "pass_when": {
        "op": "all",
        "conditions": [{"metric": "matches", "gte": 1}]
      },
      "fail_reason": "missing",
      "evidence": {
        "kind": "json",
        "path": "evidence/rule-1.json"
      },
      "unexpected_rule_key": true
    }
  ],
  "events": {
    "file": "guardian-events.jsonl",
    "include": ["timestamp"]
  }
}`,
			wantErr:     true,
			errContains: "rules[0].unexpected_rule_key",
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				var verrs ValidationErrors
				if !errors.As(err, &verrs) {
					t.Fatalf("expected ValidationErrors, got %T", err)
				}
				if len(verrs) == 0 {
					t.Fatal("expected at least one validation error")
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writePolicyFile(t, "flow.v2.json", tc.body)

			got, err := Load(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (policy=%+v)", got)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err, tc.errContains)
				}
				if tc.assertErr != nil {
					tc.assertErr(t, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got == nil {
				t.Fatal("Load() returned nil policy")
			}
			if got.Version != 2 {
				t.Fatalf("version=%d want=2", got.Version)
			}
		})
	}
}

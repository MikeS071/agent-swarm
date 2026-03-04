package watchdog

import (
	"strings"
	"testing"
)

func TestParseGuardianReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantErr      bool
		errContains  string
		wantFindings int
		wantVerdict  string
	}{
		{
			name: "valid schema block verdict",
			body: `{
  "findings": [
    {
      "severity": "CRITICAL",
      "category": "Security",
      "file": "api/routes.go",
      "line": 42,
      "title": "sql injection",
      "description": "unsafe query interpolation",
      "suggested_fix": "use parameterized query"
    }
  ],
  "verdict": "block",
  "summary": "1 critical"
}`,
			wantFindings: 1,
			wantVerdict:  "BLOCK",
		},
		{
			name: "invalid json",
			body: `{"findings":[}`,
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name: "unknown top level field rejected",
			body: `{
  "findings": [],
  "verdict": "PASS",
  "summary": "clean",
  "extra": true
}`,
			wantErr:     true,
			errContains: "unknown field",
		},
		{
			name: "invalid finding severity",
			body: `{
  "findings": [
    {
      "severity": "urgent",
      "category": "security",
      "file": "api/routes.go",
      "line": 12,
      "title": "bad",
      "description": "bad",
      "suggested_fix": "fix"
    }
  ],
  "verdict": "BLOCK",
  "summary": "bad"
}`,
			wantErr:     true,
			errContains: "findings[0].severity",
		},
		{
			name: "invalid verdict and findings combination",
			body: `{
  "findings": [
    {
      "severity": "low",
      "category": "style",
      "file": "main.go",
      "line": 5,
      "title": "style",
      "description": "minor",
      "suggested_fix": "optional"
    }
  ],
  "verdict": "PASS",
  "summary": "has findings"
}`,
			wantErr:     true,
			errContains: "verdict",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseGuardianReport([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err, tc.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseGuardianReport returned error: %v", err)
			}
			if len(got.Findings) != tc.wantFindings {
				t.Fatalf("findings count=%d want=%d", len(got.Findings), tc.wantFindings)
			}
			if got.Verdict != tc.wantVerdict {
				t.Fatalf("verdict=%q want=%q", got.Verdict, tc.wantVerdict)
			}
		})
	}
}

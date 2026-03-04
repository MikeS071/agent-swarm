package watchdog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func parseGuardianReport(body []byte) (reviewReport, error) {
	var report reviewReport

	if len(bytes.TrimSpace(body)) == 0 {
		return report, fmt.Errorf("report body is empty")
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&report); err != nil {
		return report, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return report, fmt.Errorf("unexpected trailing JSON content")
		}
		return report, err
	}

	if err := report.normalizeAndValidate(); err != nil {
		return reviewReport{}, err
	}
	return report, nil
}

func (r *reviewReport) normalizeAndValidate() error {
	if r == nil {
		return fmt.Errorf("report is nil")
	}

	r.Verdict = strings.ToUpper(strings.TrimSpace(r.Verdict))
	r.Summary = strings.TrimSpace(r.Summary)

	if r.Verdict != "BLOCK" && r.Verdict != "WARN" && r.Verdict != "PASS" {
		return fmt.Errorf("verdict must be BLOCK, WARN, or PASS")
	}
	if r.Summary == "" {
		return fmt.Errorf("summary is required")
	}

	hasBlockSeverity := false
	for i := range r.Findings {
		if err := r.Findings[i].normalizeAndValidate(i); err != nil {
			return err
		}
		if isActionableSeverity(r.Findings[i].Severity) {
			hasBlockSeverity = true
		}
	}

	switch {
	case len(r.Findings) == 0 && r.Verdict != "PASS":
		return fmt.Errorf("verdict must be PASS when findings are empty")
	case len(r.Findings) > 0 && hasBlockSeverity && r.Verdict != "BLOCK":
		return fmt.Errorf("verdict must be BLOCK when critical/high findings exist")
	case len(r.Findings) > 0 && !hasBlockSeverity && r.Verdict != "WARN":
		return fmt.Errorf("verdict must be WARN when only medium/low findings exist")
	}

	return nil
}

func (f *reviewFinding) normalizeAndValidate(index int) error {
	if f == nil {
		return fmt.Errorf("findings[%d] is nil", index)
	}

	fieldPrefix := fmt.Sprintf("findings[%d]", index)
	f.Severity = strings.ToLower(strings.TrimSpace(f.Severity))
	f.Category = strings.ToLower(strings.TrimSpace(f.Category))
	f.File = strings.TrimSpace(f.File)
	f.Title = strings.TrimSpace(f.Title)
	f.Description = strings.TrimSpace(f.Description)
	f.SuggestedFix = strings.TrimSpace(f.SuggestedFix)

	switch f.Severity {
	case "critical", "high", "medium", "low":
	default:
		return fmt.Errorf("%s.severity must be one of critical|high|medium|low", fieldPrefix)
	}

	switch f.Category {
	case "security", "correctness", "performance", "style", "documentation":
	default:
		return fmt.Errorf("%s.category must be one of security|correctness|performance|style|documentation", fieldPrefix)
	}

	if f.File == "" {
		return fmt.Errorf("%s.file is required", fieldPrefix)
	}
	if f.Line <= 0 {
		return fmt.Errorf("%s.line must be > 0", fieldPrefix)
	}
	if f.Title == "" {
		return fmt.Errorf("%s.title is required", fieldPrefix)
	}
	if f.Description == "" {
		return fmt.Errorf("%s.description is required", fieldPrefix)
	}
	if f.SuggestedFix == "" {
		return fmt.Errorf("%s.suggested_fix is required", fieldPrefix)
	}
	return nil
}

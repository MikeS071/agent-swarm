package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

var deterministicSections = []string{
	"Header",
	"Objective",
	"Scope In",
	"Scope Out",
	"Files to Touch",
	"Reference Files",
	"Implementation Steps",
	"Tests to Add/Update",
	"Verify Command",
	"Acceptance Criteria",
	"Constraints",
	"Commit Contract",
	"Forbidden Actions",
	"Agent Context Pointers",
}

type PolicyContext struct {
	ProjectName          string
	BaseBranch           string
	SpecFile             string
	DefaultVerifyCommand string
	AgentContextPointers []string
}

type CompileOptions struct {
	Strict bool
}

type CompiledArtifact struct {
	Prompt   []byte
	Manifest []byte
}

type compiledTicket struct {
	ID                 string
	Title              string
	Type               string
	Phase              int
	RunID              string
	Role               string
	Depends            []string
	Objective          string
	ScopeIn            []string
	ScopeOut           []string
	FilesToTouch       []string
	ReferenceFiles     []string
	Implementation     []string
	TestsToAddOrUpdate []string
	VerifyCommand      string
	AcceptanceCriteria []string
	Constraints        []string
}

type promptManifest struct {
	Version      int            `json:"version"`
	Ticket       string         `json:"ticket"`
	PromptFile   string         `json:"prompt_file"`
	Sections     []string       `json:"sections"`
	Strict       bool           `json:"strict"`
	PromptSHA256 string         `json:"prompt_sha256"`
	Policy       policyManifest `json:"policy"`
	Inputs       promptInputs   `json:"inputs"`
}

type policyManifest struct {
	ProjectName          string   `json:"project_name,omitempty"`
	BaseBranch           string   `json:"base_branch,omitempty"`
	SpecFile             string   `json:"spec_file,omitempty"`
	DefaultVerifyCommand string   `json:"default_verify_command,omitempty"`
	AgentContextPointers []string `json:"agent_context_pointers,omitempty"`
}

type promptInputs struct {
	Type               string   `json:"type,omitempty"`
	Phase              int      `json:"phase"`
	RunID              string   `json:"run_id,omitempty"`
	Role               string   `json:"role,omitempty"`
	Depends            []string `json:"depends,omitempty"`
	FilesToTouch       []string `json:"files_to_touch,omitempty"`
	ReferenceFiles     []string `json:"reference_files,omitempty"`
	VerifyCommand      string   `json:"verify_command,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	Constraints        []string `json:"constraints,omitempty"`
}

func Compile(ticketID string, tk tracker.Ticket, policy PolicyContext, opts CompileOptions) (CompiledArtifact, error) {
	id := strings.TrimSpace(ticketID)
	if id == "" {
		return CompiledArtifact{}, fmt.Errorf("ticket id is required")
	}

	compiled := normalizeTicket(id, tk, policy)
	if opts.Strict {
		if err := validateStrict(compiled); err != nil {
			return CompiledArtifact{}, err
		}
	}

	prompt := renderPrompt(compiled, policy)
	manifest, err := renderManifest(compiled, policy, opts.Strict, prompt)
	if err != nil {
		return CompiledArtifact{}, err
	}

	return CompiledArtifact{
		Prompt:   prompt,
		Manifest: manifest,
	}, nil
}

func normalizeTicket(ticketID string, tk tracker.Ticket, policy PolicyContext) compiledTicket {
	title := firstNonEmpty(tk.Desc, ticketID)
	ticketType := firstNonEmpty(tk.Type, "feature")
	role := firstNonEmpty(tk.Role, tk.Profile)
	verify := firstNonEmpty(tk.VerifyCmd, policy.DefaultVerifyCommand)

	return compiledTicket{
		ID:                 ticketID,
		Title:              strings.TrimSpace(title),
		Type:               strings.TrimSpace(ticketType),
		Phase:              tk.Phase,
		RunID:              strings.TrimSpace(tk.RunID),
		Role:               strings.TrimSpace(role),
		Depends:            cloneSlice(tk.Depends),
		Objective:          strings.TrimSpace(tk.Objective),
		ScopeIn:            fallbackList(tk.ScopeIn, "(not provided)"),
		ScopeOut:           fallbackList(tk.ScopeOut, "(not provided)"),
		FilesToTouch:       fallbackList(tk.FilesToTouch, "(not provided)"),
		ReferenceFiles:     fallbackList(tk.ReferenceFiles, "(none)"),
		Implementation:     fallbackList(tk.ImplementationSteps, "(not provided)"),
		TestsToAddOrUpdate: fallbackList(tk.TestsToAddOrUpdate, "(not provided)"),
		VerifyCommand:      strings.TrimSpace(verify),
		AcceptanceCriteria: fallbackList(tk.AcceptanceCriteria, "(not provided)"),
		Constraints:        fallbackList(tk.Constraints, "(not provided)"),
	}
}

func validateStrict(tk compiledTicket) error {
	missing := make([]string, 0, 12)
	if strings.TrimSpace(tk.Objective) == "" {
		missing = append(missing, "objective")
	}
	if len(filterNonEmpty(tk.ScopeIn)) == 0 || containsOnlyFallback(tk.ScopeIn, "(not provided)") {
		missing = append(missing, "scope_in")
	}
	if len(filterNonEmpty(tk.ScopeOut)) == 0 || containsOnlyFallback(tk.ScopeOut, "(not provided)") {
		missing = append(missing, "scope_out")
	}
	if len(filterNonEmpty(tk.FilesToTouch)) == 0 || containsOnlyFallback(tk.FilesToTouch, "(not provided)") {
		missing = append(missing, "files_to_touch")
	}
	if len(filterNonEmpty(tk.Implementation)) < 2 || containsOnlyFallback(tk.Implementation, "(not provided)") {
		missing = append(missing, "implementation_steps")
	}
	if len(filterNonEmpty(tk.TestsToAddOrUpdate)) == 0 || containsOnlyFallback(tk.TestsToAddOrUpdate, "(not provided)") {
		missing = append(missing, "tests_to_add_or_update")
	}
	if strings.TrimSpace(tk.VerifyCommand) == "" {
		missing = append(missing, "verify_cmd")
	}
	if len(filterNonEmpty(tk.AcceptanceCriteria)) == 0 || containsOnlyFallback(tk.AcceptanceCriteria, "(not provided)") {
		missing = append(missing, "acceptance_criteria")
	}
	if len(filterNonEmpty(tk.Constraints)) == 0 || containsOnlyFallback(tk.Constraints, "(not provided)") {
		missing = append(missing, "constraints")
	}
	if strings.TrimSpace(tk.Role) == "" {
		missing = append(missing, "role")
	}
	if strings.TrimSpace(tk.RunID) == "" {
		missing = append(missing, "runId")
	}

	placeholderFields := placeholderViolations(tk)
	if len(placeholderFields) > 0 {
		missing = append(missing, placeholderFields...)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(uniqueSorted(missing), ", "))
	}
	if strings.Contains(tk.VerifyCommand, "\n") || strings.Contains(tk.VerifyCommand, "\r") {
		return fmt.Errorf("invalid verify_cmd: must be a single shell command")
	}
	return nil
}

func placeholderViolations(tk compiledTicket) []string {
	violations := make([]string, 0)
	checkField := func(field, value string) {
		if hasPlaceholder(value) {
			violations = append(violations, field)
		}
	}
	checkList := func(field string, values []string) {
		for _, value := range values {
			if hasPlaceholder(value) {
				violations = append(violations, field)
				return
			}
		}
	}

	checkField("objective", tk.Objective)
	checkList("scope_in", tk.ScopeIn)
	checkList("scope_out", tk.ScopeOut)
	checkList("files_to_touch", tk.FilesToTouch)
	checkList("implementation_steps", tk.Implementation)
	checkList("tests_to_add_or_update", tk.TestsToAddOrUpdate)
	checkField("verify_cmd", tk.VerifyCommand)
	checkList("acceptance_criteria", tk.AcceptanceCriteria)
	checkList("constraints", tk.Constraints)

	return violations
}

func hasPlaceholder(value string) bool {
	s := strings.TrimSpace(strings.ToLower(value))
	if s == "" {
		return false
	}
	if strings.Contains(s, "todo") || strings.Contains(s, "tbd") || strings.Contains(s, "add details here") {
		return true
	}
	return strings.Contains(s, "<") && strings.Contains(s, ">")
}

func renderPrompt(tk compiledTicket, policy PolicyContext) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n", strings.ToUpper(tk.ID), tk.Title)

	writeHeaderSection(&b, tk, policy)
	writeTextSection(&b, "Objective", firstNonEmpty(tk.Objective, tk.Title, "(not provided)"))
	writeListSection(&b, "Scope In", tk.ScopeIn)
	writeListSection(&b, "Scope Out", tk.ScopeOut)
	writeListSection(&b, "Files to Touch", tk.FilesToTouch)
	writeListSection(&b, "Reference Files", tk.ReferenceFiles)
	writeNumberedSection(&b, "Implementation Steps", tk.Implementation)
	writeListSection(&b, "Tests to Add/Update", tk.TestsToAddOrUpdate)
	writeTextSection(&b, "Verify Command", "`"+tk.VerifyCommand+"`")
	writeListSection(&b, "Acceptance Criteria", tk.AcceptanceCriteria)
	writeListSection(&b, "Constraints", tk.Constraints)
	writeListSection(&b, "Commit Contract", []string{
		"Mark the ticket done only when the verify command passes.",
		"Ensure every acceptance criterion is satisfied with testable evidence.",
		"Include the ticket ID in the commit message.",
	})
	writeListSection(&b, "Forbidden Actions", []string{
		"Do not edit files outside the Files to Touch section unless acceptance criteria require it.",
		"Do not skip, weaken, or silently bypass tests listed for this ticket.",
		"Do not leave unresolved placeholders (TODO/TBD/<...>/Add details here) in final artifacts.",
	})
	writeListSection(&b, "Agent Context Pointers", normalizePointers(policy.AgentContextPointers))

	return []byte(b.String())
}

func writeHeaderSection(b *strings.Builder, tk compiledTicket, policy PolicyContext) {
	writeSectionHeader(b, "Header")
	fmt.Fprintf(b, "- Ticket ID: `%s`\n", tk.ID)
	fmt.Fprintf(b, "- Title: %s\n", tk.Title)
	fmt.Fprintf(b, "- Type: `%s`\n", tk.Type)
	fmt.Fprintf(b, "- Phase: `%d`\n", tk.Phase)
	fmt.Fprintf(b, "- Run ID: `%s`\n", firstNonEmpty(tk.RunID, "(not provided)"))
	fmt.Fprintf(b, "- Role: `%s`\n", firstNonEmpty(tk.Role, "(not provided)"))
	fmt.Fprintf(b, "- Project: `%s`\n", firstNonEmpty(strings.TrimSpace(policy.ProjectName), "(not provided)"))
	fmt.Fprintf(b, "- Base Branch: `%s`\n", firstNonEmpty(strings.TrimSpace(policy.BaseBranch), "(not provided)"))
	if len(tk.Depends) == 0 {
		fmt.Fprintf(b, "- Depends On: `none`\n\n")
		return
	}
	fmt.Fprintf(b, "- Depends On: `%s`\n\n", strings.Join(tk.Depends, ", "))
}

func writeSectionHeader(b *strings.Builder, title string) {
	fmt.Fprintf(b, "## %s\n", title)
}

func writeTextSection(b *strings.Builder, title, body string) {
	writeSectionHeader(b, title)
	fmt.Fprintf(b, "%s\n\n", strings.TrimSpace(body))
}

func writeListSection(b *strings.Builder, title string, items []string) {
	writeSectionHeader(b, title)
	for _, item := range filterNonEmpty(items) {
		fmt.Fprintf(b, "- %s\n", item)
	}
	fmt.Fprintln(b)
}

func writeNumberedSection(b *strings.Builder, title string, items []string) {
	writeSectionHeader(b, title)
	index := 1
	for _, item := range filterNonEmpty(items) {
		fmt.Fprintf(b, "%d. %s\n", index, item)
		index++
	}
	fmt.Fprintln(b)
}

func renderManifest(tk compiledTicket, policy PolicyContext, strict bool, prompt []byte) ([]byte, error) {
	sum := sha256.Sum256(prompt)
	manifest := promptManifest{
		Version:      1,
		Ticket:       tk.ID,
		PromptFile:   filepath.ToSlash(filepath.Join("swarm", "prompts", tk.ID+".md")),
		Sections:     cloneSlice(deterministicSections),
		Strict:       strict,
		PromptSHA256: hex.EncodeToString(sum[:]),
		Policy: policyManifest{
			ProjectName:          strings.TrimSpace(policy.ProjectName),
			BaseBranch:           strings.TrimSpace(policy.BaseBranch),
			SpecFile:             strings.TrimSpace(policy.SpecFile),
			DefaultVerifyCommand: strings.TrimSpace(policy.DefaultVerifyCommand),
			AgentContextPointers: normalizePointers(policy.AgentContextPointers),
		},
		Inputs: promptInputs{
			Type:               tk.Type,
			Phase:              tk.Phase,
			RunID:              tk.RunID,
			Role:               tk.Role,
			Depends:            cloneSlice(tk.Depends),
			FilesToTouch:       cloneSlice(tk.FilesToTouch),
			ReferenceFiles:     cloneSlice(tk.ReferenceFiles),
			VerifyCommand:      tk.VerifyCommand,
			AcceptanceCriteria: cloneSlice(tk.AcceptanceCriteria),
			Constraints:        cloneSlice(tk.Constraints),
		},
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal prompt manifest: %w", err)
	}
	return append(b, '\n'), nil
}

func fallbackList(values []string, fallback string) []string {
	filtered := filterNonEmpty(values)
	if len(filtered) == 0 {
		return []string{fallback}
	}
	return filtered
}

func filterNonEmpty(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func normalizePointers(values []string) []string {
	filtered := filterNonEmpty(values)
	if len(filtered) == 0 {
		return []string{"(none)"}
	}
	set := make(map[string]struct{}, len(filtered))
	for _, value := range filtered {
		set[filepath.ToSlash(filepath.Clean(value))] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func containsOnlyFallback(values []string, fallback string) bool {
	if len(values) != 1 {
		return false
	}
	return strings.TrimSpace(values[0]) == fallback
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

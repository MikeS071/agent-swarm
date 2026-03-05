package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

const (
	AssetBaseRule    = "base_rule"
	AssetRoleRule    = "role_rule"
	AssetRoleProfile = "role_profile"
	AssetRoleSpec    = "role_spec"
	AssetRoleRef     = "role_ref"
)

var requiredBaseRuleFiles = []string{
	"coding-style.md",
	"git-workflow.md",
	"testing.md",
	"performance.md",
	"patterns.md",
	"security.md",
}

var roleNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Failure struct {
	Tickets []string `json:"tickets,omitempty"`
	Role    string   `json:"role,omitempty"`
	Asset   string   `json:"asset"`
	Path    string   `json:"path,omitempty"`
	Reason  string   `json:"reason"`
}

type Report struct {
	Roles    []string  `json:"roles"`
	Failures []Failure `json:"failures"`
}

func (r Report) OK() bool {
	return len(r.Failures) == 0
}

func RequiredBaseRuleFiles() []string {
	out := make([]string, len(requiredBaseRuleFiles))
	copy(out, requiredBaseRuleFiles)
	return out
}

func ResolveTicketRole(tk tracker.Ticket) string {
	return strings.TrimSpace(tk.Profile)
}

func Check(projectRoot string, tr *tracker.Tracker) Report {
	root := strings.TrimSpace(projectRoot)
	roleTickets := collectRoleTickets(tr)
	roles := make([]string, 0, len(roleTickets))
	for role := range roleTickets {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	report := Report{
		Roles:    roles,
		Failures: make([]Failure, 0),
	}

	if hasBaseRuleFile(root) {
		// Newer layout: .codex/rules/base.md
	} else {
		for _, file := range requiredBaseRuleFiles {
			path := filepath.Join(root, ".codex", "rules", "common", file)
			ok, reason := validateRegularFile(path)
			if ok {
				continue
			}
			report.Failures = append(report.Failures, Failure{
				Asset:  AssetBaseRule,
				Path:   relPath(root, path),
				Reason: reason,
			})
		}
	}

	for _, role := range roles {
		tickets := append([]string(nil), roleTickets[role]...)
		if !isValidRoleName(role) {
			report.Failures = append(report.Failures, Failure{
				Tickets: tickets,
				Role:    role,
				Asset:   AssetRoleRef,
				Reason:  "invalid role name",
			})
			continue
		}

		profilePath := filepath.Join(root, ".agents", "profiles", role+".md")
		if ok, reason := validateRegularFile(profilePath); !ok {
			report.Failures = append(report.Failures, Failure{
				Tickets: tickets,
				Role:    role,
				Asset:   AssetRoleProfile,
				Path:    relPath(root, profilePath),
				Reason:  reason,
			})
		}

		roleRulePath := filepath.Join(root, ".codex", "rules", role+".md")
		if ok, reason := validateRegularFile(roleRulePath); !ok {
			report.Failures = append(report.Failures, Failure{
				Tickets: tickets,
				Role:    role,
				Asset:   AssetRoleRule,
				Path:    relPath(root, roleRulePath),
				Reason:  reason,
			})
		}

		roleSpecYAML := filepath.Join(root, ".agents", "roles", role+".yaml")
		roleSpecYML := filepath.Join(root, ".agents", "roles", role+".yml")
		yamlOK, yamlReason := validateRegularFile(roleSpecYAML)
		ymlOK, ymlReason := validateRegularFile(roleSpecYML)
		if !yamlOK && !ymlOK {
			reason := yamlReason
			if yamlReason != "missing file" && ymlReason != "missing file" {
				reason = fmt.Sprintf("%s; %s", yamlReason, ymlReason)
			}
			report.Failures = append(report.Failures, Failure{
				Tickets: tickets,
				Role:    role,
				Asset:   AssetRoleSpec,
				Path:    relPath(root, roleSpecYAML),
				Reason:  reason,
			})
		}
	}

	sort.Slice(report.Failures, func(i, j int) bool {
		a := report.Failures[i]
		b := report.Failures[j]
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		if a.Asset != b.Asset {
			return a.Asset < b.Asset
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return strings.Join(a.Tickets, ",") < strings.Join(b.Tickets, ",")
	})

	return report
}

func hasBaseRuleFile(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	basePath := filepath.Join(root, ".codex", "rules", "base.md")
	ok, _ := validateRegularFile(basePath)
	return ok
}

func collectRoleTickets(tr *tracker.Tracker) map[string][]string {
	roleTickets := map[string][]string{}
	if tr == nil {
		return roleTickets
	}

	ids := make([]string, 0, len(tr.Tickets))
	for id := range tr.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		role := ResolveTicketRole(tr.Tickets[id])
		if role == "" {
			continue
		}
		roleTickets[role] = append(roleTickets[role], id)
	}
	return roleTickets
}

func isValidRoleName(role string) bool {
	return roleNamePattern.MatchString(strings.TrimSpace(role))
}

func validateRegularFile(path string) (bool, string) {
	if strings.TrimSpace(path) == "" {
		return false, "missing file"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "missing file"
		}
		return false, err.Error()
	}
	if info.IsDir() {
		return false, "path is a directory"
	}
	return true, ""
}

func relPath(root, path string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(rel)
}

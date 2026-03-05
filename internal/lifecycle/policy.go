package lifecycle

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type policyFile struct {
	Profiles struct {
		ByTicketType map[string]string `toml:"by_ticket_type"`
	} `toml:"profiles"`
}

func DefaultProfileMap() map[string]string {
	return map[string]string{
		"int":    "code-agent",
		"gap":    "code-reviewer",
		"tst":    "e2e-runner",
		"review": "code-reviewer",
		"sec":    "security-reviewer",
		"doc":    "doc-updater",
		"clean":  "refactor-cleaner",
		"mem":    "doc-updater",
	}
}

func LoadProfileMap(path string) (map[string]string, error) {
	defaults := DefaultProfileMap()
	path = strings.TrimSpace(path)
	if path == "" {
		return defaults, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return defaults, fmt.Errorf("read lifecycle policy %s: %w", path, err)
	}
	var pf policyFile
	if err := toml.Unmarshal(b, &pf); err != nil {
		return defaults, fmt.Errorf("parse lifecycle policy %s: %w", path, err)
	}
	if len(pf.Profiles.ByTicketType) == 0 {
		return defaults, fmt.Errorf("lifecycle policy %s has empty profiles.by_ticket_type", path)
	}
	for k, v := range pf.Profiles.ByTicketType {
		k = strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		defaults[k] = v
	}
	return defaults, nil
}

func ProfileForTicketType(profileMap map[string]string, ticketType string) string {
	if len(profileMap) == 0 {
		profileMap = DefaultProfileMap()
	}
	return strings.TrimSpace(profileMap[strings.TrimSpace(strings.ToLower(ticketType))])
}

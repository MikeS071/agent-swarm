package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/tracker"
)

func CheckPrompts(trackerPath, promptDir string) ([]string, error) {
	t, err := tracker.Load(trackerPath)
	if err != nil {
		return nil, err
	}
	missing := make([]string, 0)
	for ticketID, ticket := range t.Tickets {
		if ticket.Status != "todo" {
			continue
		}
		promptPath := filepath.Join(promptDir, ticketID+".md")
		if _, err := os.Stat(promptPath); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, ticketID)
				continue
			}
			return nil, err
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func GeneratePrompt(trackerPath, promptDir, ticketID string) (string, error) {
	t, err := tracker.Load(trackerPath)
	if err != nil {
		return "", err
	}
	ticket, ok := t.Tickets[ticketID]
	if !ok {
		return "", errors.New("ticket not found")
	}
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return "", err
	}
	deps := "none"
	if len(ticket.Depends) > 0 {
		deps = strings.Join(ticket.Depends, ", ")
	}
	body := fmt.Sprintf(`# %s: %s

## Context
You are building a Go CLI called %q. Read SPEC.md for full spec.

## Dependencies
%s

## Your Scope
- Implement the ticket requirements
- Add or update tests first
- Ensure build and tests pass

## Notes
- Add implementation details here
- Add edge cases here
`, strings.ToUpper(ticketID), ticket.Desc, t.Project, deps)

	path := filepath.Join(promptDir, ticketID+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

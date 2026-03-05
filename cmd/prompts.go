package cmd

import (
	"os"
	"path/filepath"
	"sort"

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

package tracker

import (
	"encoding/json"
	"fmt"
	"os"
)

type Tracker struct {
	Project string            `json:"project"`
	Tickets map[string]Ticket `json:"tickets"`
}

type Ticket struct {
	Status  string   `json:"status"`
	Phase   int      `json:"phase"`
	Depends []string `json:"depends"`
	Branch  string   `json:"branch"`
	Desc    string   `json:"desc"`
	SHA     string   `json:"sha,omitempty"`
}

func Load(path string) (*Tracker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Tracker
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse tracker: %w", err)
	}
	if t.Tickets == nil {
		t.Tickets = map[string]Ticket{}
	}
	return &t, nil
}

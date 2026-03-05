package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ApprovalEntry struct {
	By   string `json:"by"`
	At   string `json:"at"`
	Note string `json:"note,omitempty"`
}

type Approvals map[string]ApprovalEntry

type ApprovalStore struct {
	path string
}

func NewApprovalStore(path string) *ApprovalStore {
	return &ApprovalStore{path: strings.TrimSpace(path)}
}

func (s *ApprovalStore) Path() string { return s.path }

func (s *ApprovalStore) Load() (Approvals, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return Approvals{}, nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Approvals{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return Approvals{}, nil
	}
	m := Approvals{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse approvals: %w", err)
	}
	return m, nil
}

func (s *ApprovalStore) Set(key, by, note string, at time.Time) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("approval key is required")
	}
	by = strings.TrimSpace(by)
	if by == "" {
		return fmt.Errorf("approval by is required")
	}
	m, err := s.Load()
	if err != nil {
		return err
	}
	if m == nil {
		m = Approvals{}
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	m[key] = ApprovalEntry{By: by, At: at.UTC().Format(time.RFC3339), Note: strings.TrimSpace(note)}
	return s.write(m)
}

func (s *ApprovalStore) Delete(key string) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	m, err := s.Load()
	if err != nil {
		return err
	}
	delete(m, key)
	return s.write(m)
}

func (s *ApprovalStore) write(m Approvals) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	// Stable output ordering for deterministic diffs.
	ordered := make(map[string]ApprovalEntry, len(m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ordered[k] = m[k]
	}
	b, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(b, '\n'), 0o644)
}

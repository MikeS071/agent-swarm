package feature

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MetadataFileName = "feature.json"

type State string

const (
	StateDraft      State = "draft"
	StatePRDReview  State = "prd_review"
	StateArchReview State = "arch_review"
	StateSpecReview State = "spec_review"
	StatePlanned    State = "planned"
	StateBuilding   State = "building"
	StatePostBuild  State = "post_build"
	StateComplete   State = "complete"
)

var stateOrder = []State{
	StateDraft,
	StatePRDReview,
	StateArchReview,
	StateSpecReview,
	StatePlanned,
	StateBuilding,
	StatePostBuild,
	StateComplete,
}

var featureNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Feature struct {
	Name             string   `json:"name"`
	State            State    `json:"state"`
	PRDApprovedAt    string   `json:"prd_approved_at,omitempty"`
	PRDApprovedBy    string   `json:"prd_approved_by,omitempty"`
	ArchReviewAt     string   `json:"arch_review_at,omitempty"`
	SpecApprovedAt   string   `json:"spec_approved_at,omitempty"`
	SpecApprovedBy   string   `json:"spec_approved_by,omitempty"`
	Tickets          []string `json:"tickets,omitempty"`
	PostBuildTickets []string `json:"post_build_tickets,omitempty"`
	FixTickets       []string `json:"fix_tickets,omitempty"`
}

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) Add(name string) (*Feature, error) {
	if err := validateFeatureName(name); err != nil {
		return nil, err
	}
	metadataPath := s.metadataPath(name)
	if _, err := os.Stat(metadataPath); err == nil {
		return nil, fmt.Errorf("feature %q already exists", name)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat feature metadata %s: %w", metadataPath, err)
	}

	f := &Feature{
		Name:  name,
		State: StateDraft,
	}
	if err := s.Save(f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Store) Get(name string) (*Feature, error) {
	if err := validateFeatureName(name); err != nil {
		return nil, err
	}
	metadataPath := s.metadataPath(name)
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("read feature metadata %s: %w", metadataPath, err)
	}
	var f Feature
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse feature metadata %s: %w", metadataPath, err)
	}
	if f.Name == "" {
		f.Name = name
	}
	if err := validateFeatureName(f.Name); err != nil {
		return nil, fmt.Errorf("invalid feature metadata name %q: %w", f.Name, err)
	}
	if !f.State.Valid() {
		return nil, fmt.Errorf("invalid state %q", f.State)
	}
	return &f, nil
}

func (s *Store) Save(f *Feature) error {
	if f == nil {
		return fmt.Errorf("feature is nil")
	}
	if err := validateFeatureName(f.Name); err != nil {
		return err
	}
	if !f.State.Valid() {
		return fmt.Errorf("invalid state %q", f.State)
	}

	dir := s.FeatureDir(f.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir feature dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal feature metadata: %w", err)
	}
	if err := os.WriteFile(s.metadataPath(f.Name), data, 0o644); err != nil {
		return fmt.Errorf("write feature metadata: %w", err)
	}
	return nil
}

func (s *Store) List() ([]Feature, error) {
	if _, err := os.Stat(s.root); os.IsNotExist(err) {
		return []Feature{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat features root %s: %w", s.root, err)
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read features root %s: %w", s.root, err)
	}

	out := make([]Feature, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(s.metadataPath(name)); os.IsNotExist(err) {
			continue
		}
		f, err := s.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *Store) Advance(name string, next State) (*Feature, error) {
	if !next.Valid() {
		return nil, fmt.Errorf("invalid state %q", next)
	}
	f, err := s.Get(name)
	if err != nil {
		return nil, err
	}
	if !isAdjacentTransition(f.State, next) {
		return nil, fmt.Errorf("invalid transition %q -> %q", f.State, next)
	}
	f.State = next
	if err := s.Save(f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Store) FeatureDir(name string) string {
	return filepath.Join(s.root, name)
}

func (s *Store) metadataPath(name string) string {
	return filepath.Join(s.FeatureDir(name), MetadataFileName)
}

func (s State) Valid() bool {
	for _, state := range stateOrder {
		if s == state {
			return true
		}
	}
	return false
}

func isAdjacentTransition(from, to State) bool {
	fromIdx := stateIndex(from)
	toIdx := stateIndex(to)
	return fromIdx >= 0 && toIdx == fromIdx+1
}

func stateIndex(state State) int {
	for idx, v := range stateOrder {
		if state == v {
			return idx
		}
	}
	return -1
}

func validateFeatureName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("feature name cannot be empty")
	}
	if !featureNamePattern.MatchString(name) {
		return fmt.Errorf("feature name %q is invalid", name)
	}
	return nil
}

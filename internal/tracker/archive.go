package tracker

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

const archiveFileName = "archive.json"

type ArchiveOptions struct {
	Phase  int
	DryRun bool
}

type ArchiveSummary struct {
	Total   int
	ByPhase map[int]int
}

func DefaultArchivePath(trackerPath string) string {
	return filepath.Join(filepath.Dir(trackerPath), archiveFileName)
}

func ArchiveDoneTickets(trackerPath, archivePath string, opts ArchiveOptions) (ArchiveSummary, error) {
	active, err := Load(trackerPath)
	if err != nil {
		return ArchiveSummary{}, err
	}
	archive, err := loadOrInitArchive(archivePath, active.Project)
	if err != nil {
		return ArchiveSummary{}, err
	}

	summary := ArchiveSummary{ByPhase: map[int]int{}}
	ids := sortedTicketIDs(active.Tickets)
	for _, id := range ids {
		tk := active.Tickets[id]
		if tk.Status != StatusDone {
			continue
		}
		if opts.Phase > 0 && tk.Phase != opts.Phase {
			continue
		}
		summary.Total++
		summary.ByPhase[tk.Phase]++
		if opts.DryRun {
			continue
		}
		if _, exists := archive.Tickets[id]; exists {
			return ArchiveSummary{}, fmt.Errorf("archive already contains ticket %q", id)
		}
		archive.Tickets[id] = tk
		delete(active.Tickets, id)
	}

	if opts.DryRun || summary.Total == 0 {
		return summary, nil
	}
	if err := archive.SaveTo(archivePath); err != nil {
		return ArchiveSummary{}, err
	}
	if err := active.SaveTo(trackerPath); err != nil {
		return ArchiveSummary{}, err
	}
	return summary, nil
}

func RestoreArchivedTickets(trackerPath, archivePath string) (int, error) {
	active, err := Load(trackerPath)
	if err != nil {
		return 0, err
	}
	archive, err := loadOrInitArchive(archivePath, active.Project)
	if err != nil {
		return 0, err
	}

	if len(archive.Tickets) == 0 {
		return 0, nil
	}
	ids := sortedTicketIDs(archive.Tickets)
	for _, id := range ids {
		if _, exists := active.Tickets[id]; exists {
			return 0, fmt.Errorf("tracker already contains ticket %q", id)
		}
		active.Tickets[id] = archive.Tickets[id]
	}

	if err := active.SaveTo(trackerPath); err != nil {
		return 0, err
	}
	archive.Tickets = map[string]Ticket{}
	if err := archive.SaveTo(archivePath); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func LoadArchiveIfExists(path string) (*Tracker, error) {
	tr, err := Load(path)
	if err == nil {
		return tr, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}

func (s ArchiveSummary) Phases() []int {
	phases := make([]int, 0, len(s.ByPhase))
	for phase := range s.ByPhase {
		phases = append(phases, phase)
	}
	sort.Ints(phases)
	return phases
}

func loadOrInitArchive(path, project string) (*Tracker, error) {
	tr, err := Load(path)
	if err == nil {
		return tr, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return &Tracker{
		Project: project,
		Tickets: map[string]Ticket{},
	}, nil
}

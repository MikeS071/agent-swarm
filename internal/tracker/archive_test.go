package tracker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveDoneTickets_MovesDoneTickets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	trackerPath := filepath.Join(dir, "tracker.json")
	archivePath := filepath.Join(dir, "archive.json")

	tr := New("proj", map[string]Ticket{
		"sw-01": {Status: "done", Phase: 1},
		"sw-02": {Status: "running", Phase: 1},
		"sw-03": {Status: "done", Phase: 2},
		"sw-04": {Status: "todo", Phase: 2},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	summary, err := ArchiveDoneTickets(trackerPath, archivePath, ArchiveOptions{})
	if err != nil {
		t.Fatalf("ArchiveDoneTickets: %v", err)
	}
	if summary.Total != 2 {
		t.Fatalf("summary total=%d want 2", summary.Total)
	}
	if summary.ByPhase[1] != 1 || summary.ByPhase[2] != 1 {
		t.Fatalf("summary phases=%v want {1:1,2:1}", summary.ByPhase)
	}

	updated, err := Load(trackerPath)
	if err != nil {
		t.Fatalf("load updated tracker: %v", err)
	}
	if _, ok := updated.Tickets["sw-01"]; ok {
		t.Fatalf("sw-01 should be removed from tracker")
	}
	if _, ok := updated.Tickets["sw-03"]; ok {
		t.Fatalf("sw-03 should be removed from tracker")
	}
	if len(updated.Tickets) != 2 {
		t.Fatalf("tracker ticket count=%d want 2", len(updated.Tickets))
	}

	archived, err := Load(archivePath)
	if err != nil {
		t.Fatalf("load archive: %v", err)
	}
	if len(archived.Tickets) != 2 {
		t.Fatalf("archive ticket count=%d want 2", len(archived.Tickets))
	}
}

func TestArchiveDoneTickets_PhaseFilterAndDryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	trackerPath := filepath.Join(dir, "tracker.json")
	archivePath := filepath.Join(dir, "archive.json")

	tr := New("proj", map[string]Ticket{
		"sw-01": {Status: "done", Phase: 1},
		"sw-02": {Status: "done", Phase: 2},
		"sw-03": {Status: "running", Phase: 2},
	})
	if err := tr.SaveTo(trackerPath); err != nil {
		t.Fatalf("save tracker: %v", err)
	}

	summary, err := ArchiveDoneTickets(trackerPath, archivePath, ArchiveOptions{Phase: 1, DryRun: true})
	if err != nil {
		t.Fatalf("ArchiveDoneTickets dry-run: %v", err)
	}
	if summary.Total != 1 {
		t.Fatalf("summary total=%d want 1", summary.Total)
	}
	if summary.ByPhase[1] != 1 {
		t.Fatalf("summary phase1=%d want 1", summary.ByPhase[1])
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("archive file should not be written in dry-run")
	}

	after, err := Load(trackerPath)
	if err != nil {
		t.Fatalf("load tracker after dry-run: %v", err)
	}
	if len(after.Tickets) != 3 {
		t.Fatalf("tracker changed in dry-run; count=%d want 3", len(after.Tickets))
	}

	summary, err = ArchiveDoneTickets(trackerPath, archivePath, ArchiveOptions{Phase: 1})
	if err != nil {
		t.Fatalf("ArchiveDoneTickets phase filter: %v", err)
	}
	if summary.Total != 1 {
		t.Fatalf("summary total=%d want 1", summary.Total)
	}

	after, err = Load(trackerPath)
	if err != nil {
		t.Fatalf("load tracker after archive: %v", err)
	}
	if _, ok := after.Tickets["sw-01"]; ok {
		t.Fatalf("sw-01 should be archived")
	}
	if _, ok := after.Tickets["sw-02"]; !ok {
		t.Fatalf("sw-02 should remain in tracker")
	}
}

func TestRestoreArchivedTickets_MovesBackAndClearsArchive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	trackerPath := filepath.Join(dir, "tracker.json")
	archivePath := filepath.Join(dir, "archive.json")

	active := New("proj", map[string]Ticket{
		"sw-02": {Status: "running", Phase: 2},
	})
	if err := active.SaveTo(trackerPath); err != nil {
		t.Fatalf("save active tracker: %v", err)
	}
	archived := New("proj", map[string]Ticket{
		"sw-01": {Status: "done", Phase: 1},
		"sw-03": {Status: "done", Phase: 2},
	})
	if err := archived.SaveTo(archivePath); err != nil {
		t.Fatalf("save archive tracker: %v", err)
	}

	restored, err := RestoreArchivedTickets(trackerPath, archivePath)
	if err != nil {
		t.Fatalf("RestoreArchivedTickets: %v", err)
	}
	if restored != 2 {
		t.Fatalf("restored=%d want 2", restored)
	}

	merged, err := Load(trackerPath)
	if err != nil {
		t.Fatalf("load merged tracker: %v", err)
	}
	if len(merged.Tickets) != 3 {
		t.Fatalf("merged tracker count=%d want 3", len(merged.Tickets))
	}
	if _, ok := merged.Tickets["sw-01"]; !ok {
		t.Fatalf("sw-01 should be restored")
	}

	emptyArchive, err := Load(archivePath)
	if err != nil {
		t.Fatalf("load empty archive: %v", err)
	}
	if len(emptyArchive.Tickets) != 0 {
		t.Fatalf("archive should be cleared, found %d tickets", len(emptyArchive.Tickets))
	}
}

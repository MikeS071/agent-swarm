package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailableRAMFromMeminfo(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "meminfo")
	contents := "MemTotal:       16384256 kB\nMemAvailable:    2097152 kB\n"
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	old := procMeminfoPath
	procMeminfoPath = p
	t.Cleanup(func() { procMeminfoPath = old })

	rm, err := AvailableRAM()
	if err != nil {
		t.Fatalf("AvailableRAM() error = %v", err)
	}
	if rm != 2048 {
		t.Fatalf("AvailableRAM() = %dMB, want 2048MB", rm)
	}
}

func TestCanSpawn(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "meminfo")
	contents := "MemAvailable:    1048576 kB\n"
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	old := procMeminfoPath
	procMeminfoPath = p
	t.Cleanup(func() { procMeminfoPath = old })

	if !CanSpawn(1000) {
		t.Fatal("CanSpawn(1000) should be true")
	}
	if CanSpawn(1100) {
		t.Fatal("CanSpawn(1100) should be false")
	}
}

func TestAvailableRAMMissingMeminfoFails(t *testing.T) {
	old := procMeminfoPath
	procMeminfoPath = "/no/such/file"
	t.Cleanup(func() { procMeminfoPath = old })

	_, err := AvailableRAM()
	if err == nil {
		t.Fatal("expected error when meminfo missing")
	}
}

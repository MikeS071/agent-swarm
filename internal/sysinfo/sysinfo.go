package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var procMeminfoPath = "/proc/meminfo"

func AvailableRAM() (int, error) {
	f, err := os.Open(procMeminfoPath)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", procMeminfoPath, err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("invalid MemAvailable line: %q", line)
		}
		kb, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("parse MemAvailable: %w", err)
		}
		return kb / 1024, nil
	}
	if err := s.Err(); err != nil {
		return 0, fmt.Errorf("scan meminfo: %w", err)
	}
	return 0, fmt.Errorf("MemAvailable not found in %s", procMeminfoPath)
}

func CanSpawn(minRAM int) bool {
	avail, err := AvailableRAM()
	if err != nil {
		return false
	}
	return avail >= minRAM
}

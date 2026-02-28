package tui

import (
	"strings"
	"testing"
)

func TestRenderProgressBar(t *testing.T) {
	bar := renderProgressBar(4, 8, 8)
	if !strings.Contains(bar, "█") {
		t.Fatalf("expected filled blocks in %q", bar)
	}
	if !strings.Contains(bar, "░") {
		t.Fatalf("expected empty blocks in %q", bar)
	}
	if !strings.Contains(bar, "4/8") {
		t.Fatalf("expected ratio in %q", bar)
	}
}

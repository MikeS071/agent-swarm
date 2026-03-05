package version

import "testing"

func TestGetReturnsDevWhenUnset(t *testing.T) {
	old := ver
	ver = ""
	defer func() { ver = old }()

	v := Get()
	// Without ldflags, Get() returns either the module version or "dev"
	if v == "" {
		t.Fatal("Get() returned empty string")
	}
}

func TestGetReturnsLdflagsValue(t *testing.T) {
	old := ver
	ver = "2026.03.05-1"
	defer func() { ver = old }()

	if got := Get(); got != "2026.03.05-1" {
		t.Fatalf("expected 2026.03.05-1, got %s", got)
	}
}

func TestStringFormatsWithPrefix(t *testing.T) {
	old := ver
	ver = "2026.03.05-1"
	defer func() { ver = old }()

	if got := String(); got != "v2026.03.05-1" {
		t.Fatalf("expected v2026.03.05-1, got %s", got)
	}
}

func TestStringDevBuild(t *testing.T) {
	old := ver
	ver = ""
	defer func() { ver = old }()

	s := String()
	// Should be either "vdev" or "v<module-version>"
	if s == "" {
		t.Fatal("String() returned empty")
	}
}

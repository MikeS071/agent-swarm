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
	if s == "" {
		t.Fatal("String() returned empty")
	}
}

func TestNormalizePseudoVersionToCanonical(t *testing.T) {
	in := "v0.0.0-20260305082248-0167d23c1333+dirty"
	got := normalize(in)
	want := "2026.03.05-82248"
	if got != want {
		t.Fatalf("normalize(%q)=%q want %q", in, got, want)
	}
}

func TestNormalizeCanonicalStaysCanonical(t *testing.T) {
	in := "v2026.03.05-9"
	got := normalize(in)
	if got != "2026.03.05-9" {
		t.Fatalf("normalize canonical got %q", got)
	}
}

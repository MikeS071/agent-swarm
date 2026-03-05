package version

import "testing"

func TestGetPrefersLdflagsValue(t *testing.T) {
	oldVer := ver
	oldEmbedded := embeddedVersion
	ver = "2026.03.05-11"
	embeddedVersion = "2026.03.05-9"
	defer func() {
		ver = oldVer
		embeddedVersion = oldEmbedded
	}()

	if got := Get(); got != "2026.03.05-11" {
		t.Fatalf("expected ldflags version, got %s", got)
	}
}

func TestGetUsesEmbeddedVersionWhenLdflagsUnset(t *testing.T) {
	oldVer := ver
	oldEmbedded := embeddedVersion
	ver = ""
	embeddedVersion = "2026.03.05-9"
	defer func() {
		ver = oldVer
		embeddedVersion = oldEmbedded
	}()

	if got := Get(); got != "2026.03.05-9" {
		t.Fatalf("expected embedded version, got %s", got)
	}
}

func TestNormalizeRejectsPseudoVersion(t *testing.T) {
	in := "v0.0.0-20260305082248-0167d23c1333+dirty"
	if got := normalize(in); got != "" {
		t.Fatalf("expected pseudo version to be rejected, got %q", got)
	}
}

func TestStringFormatsWithPrefix(t *testing.T) {
	oldVer := ver
	oldEmbedded := embeddedVersion
	ver = ""
	embeddedVersion = "2026.03.05-9"
	defer func() {
		ver = oldVer
		embeddedVersion = oldEmbedded
	}()

	if got := String(); got != "v2026.03.05-9" {
		t.Fatalf("expected v2026.03.05-9, got %s", got)
	}
}

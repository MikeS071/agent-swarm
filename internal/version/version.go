// Package version provides canonical date-based versioning (YYYY.MM.DD-patch).
//
// Set at build time via ldflags:
//
//	go build -ldflags "-X github.com/MikeS071/agent-swarm/internal/version.ver=2026.03.05-1"
//
// Falls back to embedded VERSION, then Go module version from debug.ReadBuildInfo.
package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

// ver is set via ldflags. Empty means use embedded VERSION/build info fallback.
var ver string

//go:embed VERSION
var embeddedVersion string

// Get returns the canonical version string (no "v" prefix).
func Get() string {
	if v := normalize(strings.TrimSpace(ver)); v != "" {
		return v
	}
	if v := normalize(strings.TrimSpace(embeddedVersion)); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := normalize(strings.TrimSpace(bi.Main.Version)); v != "" {
			return v
		}
	}
	return "dev"
}

func normalize(v string) string {
	if v == "" || v == "(devel)" {
		return ""
	}
	v = strings.TrimPrefix(v, "v")
	if isCanonical(v) {
		return v
	}
	return ""
}

func isCanonical(v string) bool {
	parts := strings.Split(v, "-")
	if len(parts) != 2 {
		return false
	}
	dateParts := strings.Split(parts[0], ".")
	if len(dateParts) != 3 {
		return false
	}
	if len(dateParts[0]) != 4 || len(dateParts[1]) != 2 || len(dateParts[2]) != 2 {
		return false
	}
	for _, p := range append(dateParts, parts[1]) {
		if p == "" {
			return false
		}
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

// String returns the display form with "v" prefix.
func String() string {
	v := Get()
	if v == "dev" {
		return "vdev"
	}
	return "v" + v
}

// Package version provides canonical date-based versioning (YYYY.MM.DD-patch).
//
// Set at build time via ldflags:
//
//	go build -ldflags "-X github.com/MikeS071/agent-swarm/internal/version.ver=2026.03.05-1"
//
// Falls back to Go module version from debug.ReadBuildInfo if ldflags not set.
package version

import (
	"runtime/debug"
	"strings"
)

// ver is set via ldflags. Empty means check build info or dev.
var ver string

// Get returns the canonical version string (no "v" prefix).
func Get() string {
	if ver != "" {
		return ver
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(bi.Main.Version); v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

// String returns the display form with "v" prefix.
func String() string {
	v := Get()
	if v == "dev" {
		return "vdev"
	}
	return "v" + v
}

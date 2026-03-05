// Package version provides canonical date-based versioning (YYYY.MM.DD-patch).
//
// Set at build time via ldflags:
//
//	go build -ldflags "-X github.com/MikeS071/agent-swarm/internal/version.ver=2026.03.05-1"
//
// Falls back to Go module version from debug.ReadBuildInfo if ldflags not set.
package version

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// ver is set via ldflags. Empty means check build info or dev.
var ver string

var pseudoVersionRE = regexp.MustCompile(`^v\d+\.\d+\.\d+-(\d{14})-[0-9a-f]+(?:\+dirty)?$`)

// Get returns the canonical version string (no "v" prefix).
func Get() string {
	if v := normalize(strings.TrimSpace(ver)); v != "" {
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
	if pv, ok := convertPseudoToCanonical(v); ok {
		return pv
	}
	if v == "" {
		return ""
	}
	return v
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

func convertPseudoToCanonical(v string) (string, bool) {
	m := pseudoVersionRE.FindStringSubmatch("v" + v)
	if len(m) != 2 {
		return "", false
	}
	ts := m[1] // YYYYMMDDhhmmss
	date := fmt.Sprintf("%s.%s.%s", ts[0:4], ts[4:6], ts[6:8])
	patch := strings.TrimLeft(ts[8:], "0")
	if patch == "" {
		patch = "0"
	}
	return date + "-" + patch, true
}

// String returns the display form with "v" prefix.
func String() string {
	v := Get()
	if v == "dev" {
		return "vdev"
	}
	return "v" + v
}

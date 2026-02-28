package cmd

import "testing"

func TestDetectInstallMethodSystemd(t *testing.T) {
	method := detectInstallMethod("linux", func(path string) bool {
		return path == "/run/systemd/system"
	})
	if method != installMethodSystemd {
		t.Fatalf("method = %q, want %q", method, installMethodSystemd)
	}
}

func TestDetectInstallMethodLaunchd(t *testing.T) {
	method := detectInstallMethod("darwin", func(path string) bool {
		return path == "/System/Library/LaunchDaemons"
	})
	if method != installMethodLaunchd {
		t.Fatalf("method = %q, want %q", method, installMethodLaunchd)
	}
}

func TestDetectInstallMethodCronFallback(t *testing.T) {
	method := detectInstallMethod("linux", func(string) bool { return false })
	if method != installMethodCron {
		t.Fatalf("method = %q, want %q", method, installMethodCron)
	}
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

const (
	installMethodSystemd = "systemd"
	installMethodLaunchd = "launchd"
	installMethodCron    = "cron"
)

var installUser bool
var installInterval string
var installUninstall bool

func ensureWatchdogInstalledForConfig(configPath string) error {
	absCfg, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(absCfg)
	if err != nil {
		return err
	}
	interval := strings.TrimSpace(cfg.Install.Interval)
	if interval == "" {
		interval = "5m"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	method := strings.TrimSpace(cfg.Install.Method)
	if method == "" {
		method = detectInstallMethod(runtime.GOOS, pathExists)
	}
	if err := installWatchdog(method, home, interval, true, resolveSwarmBinary()); err != nil {
		return err
	}
	return verifyInstall()
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install or uninstall automatic swarm watch scheduling",
	RunE: func(_ *cobra.Command, _ []string) error {
		absCfg, err := filepath.Abs(cfgFile)
		if err != nil {
			return err
		}
		cfg, err := config.Load(absCfg)
		if err != nil {
			return err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		method := strings.TrimSpace(cfg.Install.Method)
		if method == "" {
			method = detectInstallMethod(runtime.GOOS, pathExists)
		}
		if installUninstall {
			return uninstallWatchdog(method, home)
		}
		interval := strings.TrimSpace(installInterval)
		if interval != "" {
			cfg.Install.Interval = interval
			cfgBytes, merr := toml.Marshal(cfg)
			if merr == nil {
				_ = os.WriteFile(absCfg, cfgBytes, 0o644)
			}
		}
		return ensureWatchdogInstalledForConfig(absCfg)
	},
}

func init() {
	installCmd.Flags().BoolVar(&installUser, "user", true, "install user-level service")
	installCmd.Flags().StringVar(&installInterval, "interval", "5m", "watchdog interval")
	installCmd.Flags().BoolVar(&installUninstall, "uninstall", false, "remove existing installation")
	rootCmd.AddCommand(installCmd)
}

func resolveSwarmBinary() string {
	if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
		return exe
	}
	return "swarm"
}

func detectInstallMethod(goos string, exists func(string) bool) string {
	switch goos {
	case "linux":
		if exists("/run/systemd/system") {
			return installMethodSystemd
		}
	case "darwin":
		if exists("/System/Library/LaunchDaemons") || exists("/Library/LaunchDaemons") {
			return installMethodLaunchd
		}
	}
	return installMethodCron
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func installWatchdog(method, home, interval string, _ bool, swarmBin string) error {
	switch method {
	case installMethodSystemd:
		return installSystemd(home, interval, swarmBin)
	case installMethodLaunchd:
		return installLaunchd(home, interval, swarmBin)
	case installMethodCron:
		return installCron(interval, swarmBin)
	default:
		return fmt.Errorf("unsupported install method %q", method)
	}
}

func uninstallWatchdog(method, home string) error {
	switch method {
	case installMethodSystemd:
		_ = runCmdNoOutput("systemctl", "--user", "disable", "--now", "swarm-watchdog.timer")
		_ = os.Remove(filepath.Join(home, ".config/systemd/user/swarm-watchdog.service"))
		_ = os.Remove(filepath.Join(home, ".config/systemd/user/swarm-watchdog.timer"))
		_ = runCmdNoOutput("systemctl", "--user", "daemon-reload")
		return nil
	case installMethodLaunchd:
		plist := filepath.Join(home, "Library/LaunchAgents/com.agentswarm.watchdog.plist")
		_ = runCmdNoOutput("launchctl", "unload", plist)
		_ = os.Remove(plist)
		return nil
	case installMethodCron:
		out, err := exec.Command("crontab", "-l").CombinedOutput()
		if err != nil && len(strings.TrimSpace(string(out))) == 0 {
			return nil
		}
		lines := strings.Split(string(out), "\n")
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.Contains(line, "swarm watch --once") || strings.Contains(line, "swarm watchdog run-all-once") {
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			filtered = append(filtered, line)
		}
		return writeCrontab(filtered)
	default:
		return fmt.Errorf("unsupported install method %q", method)
	}
}

func installSystemd(home, interval, swarmBin string) error {
	dir := filepath.Join(home, ".config/systemd/user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	service := fmt.Sprintf(`[Unit]
Description=agent-swarm watchdog

[Service]
Type=oneshot
ExecStart=%s watchdog run-all-once
`, swarmBin)
	timer := `[Unit]
Description=agent-swarm watchdog timer

[Timer]
OnBootSec=1m
OnUnitActiveSec=` + interval + `
Unit=swarm-watchdog.service

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile(filepath.Join(dir, "swarm-watchdog.service"), []byte(service), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "swarm-watchdog.timer"), []byte(timer), 0o644); err != nil {
		return err
	}
	if err := runCmdNoOutput("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return runCmdNoOutput("systemctl", "--user", "enable", "--now", "swarm-watchdog.timer")
}

func installLaunchd(home, interval, swarmBin string) error {
	dur, err := time.ParseDuration(interval)
	if err != nil {
		return fmt.Errorf("parse interval: %w", err)
	}
	seconds := int(dur.Seconds())
	if seconds <= 0 {
		seconds = 300
	}
	dir := filepath.Join(home, "Library/LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.agentswarm.watchdog</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + swarmBin + `</string>
    <string>watchdog</string>
    <string>run-all-once</string>
  </array>
  <key>StartInterval</key><integer>` + strconv.Itoa(seconds) + `</integer>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`
	path := filepath.Join(dir, "com.agentswarm.watchdog.plist")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	_ = runCmdNoOutput("launchctl", "unload", path)
	return runCmdNoOutput("launchctl", "load", path)
}

func installCron(interval, swarmBin string) error {
	expr, err := durationToCron(interval)
	if err != nil {
		return err
	}
	line := expr + " " + shellEscapeForCron(swarmBin) + " watchdog run-all-once"
	out, _ := exec.Command("crontab", "-l").CombinedOutput()
	current := strings.TrimSpace(string(out))
	if strings.Contains(current, line) {
		return nil
	}
	lines := []string{}
	if current != "" {
		lines = append(lines, strings.Split(current, "\n")...)
	}
	lines = append(lines, line)
	return writeCrontab(lines)
}

func shellEscapeForCron(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "swarm"
	}
	if strings.ContainsAny(v, " 	") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}

func durationToCron(interval string) (string, error) {
	dur, err := time.ParseDuration(interval)
	if err != nil {
		return "", fmt.Errorf("parse interval: %w", err)
	}
	mins := int(dur.Minutes())
	if mins <= 0 {
		mins = 5
	}
	if mins > 59 {
		mins = 60
	}
	return fmt.Sprintf("*/%d * * * *", mins), nil
}

func writeCrontab(lines []string) error {
	payload := strings.Join(lines, "\n")
	if payload != "" {
		payload += "\n"
	}
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("crontab update: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCmdNoOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verifyInstall() error {
	bin := resolveSwarmBinary()
	cmd := exec.Command(bin, "watchdog", "run-all-once", "--dry-run")
	if out, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else {
		exe, e := os.Executable()
		if e != nil {
			return fmt.Errorf("verify install: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		fallback := exec.Command(exe, "watchdog", "run-all-once", "--dry-run")
		fallbackOut, fallbackErr := fallback.CombinedOutput()
		if fallbackErr != nil {
			return fmt.Errorf("verify install: %w (%s); fallback: %v (%s)", err, strings.TrimSpace(string(out)), fallbackErr, strings.TrimSpace(string(fallbackOut)))
		}
	}
	return nil
}

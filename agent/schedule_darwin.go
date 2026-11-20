//go:build darwin

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const launchLabel = "local.office-health.agent"

// ConfigDir — на Mac: ~/Library/Application Support/office-health
func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "office-health")
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

// InstallSchedule — на Mac расписанием занимается launchd: кладём «LaunchAgent» (маленький plist-файл)
// с интервалом 6 часов и просим launchd его загрузить. Прав администратора не нужно.
func InstallSchedule() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>agent</string></array>
  <key>StartInterval</key><integer>21600</integer>
  <key>RunAtLoad</key><false/>
</dict>
</plist>
`, launchLabel, exe)
	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(plistPath(), []byte(plist), 0o644); err != nil {
		return "", err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	exec.Command("launchctl", "bootout", domain, plistPath()).Run() // убрать старую версию, если была
	if out, err := exec.Command("launchctl", "bootstrap", domain, plistPath()).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl: %s", out)
	}
	return "проверка каждые 6 часов (launchd), файл программы — " + exe + " (не перемещайте его)", nil
}

func RemoveSchedule() error {
	exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), plistPath()).Run()
	os.Remove(plistPath())
	return nil
}

//go:build !windows && !darwin

package agent

import (
	"errors"
	"os"
	"path/filepath"
)

func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "office-health")
}

// На Linux расписание пока не ставим сами — достаточно строки в cron: 0 */6 * * * /путь/office-health agent
func InstallSchedule() (string, error) {
	return "", errors.New("добавьте в cron: 0 */6 * * * " + os.Args[0] + " agent")
}

func RemoveSchedule() error { return nil }

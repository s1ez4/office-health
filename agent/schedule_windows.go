//go:build windows

package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"office-health/collect"
)

const taskName = "office-health-agent"

// ConfigDir — на Windows настройки агента лежат в C:\ProgramData\office-health:
// задачу Планировщика запускает SYSTEM, и ему нужно видеть тот же файл, что и человеку, который подключал.
func ConfigDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "office-health")
}

// InstallSchedule ставит задачу в Планировщик: проверка каждые 6 часов от имени системы.
// Нужны права администратора — без них подскажем, как сделать вручную.
func InstallSchedule() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if !collect.IsAdmin() {
		return "", errors.New("чтобы проверка шла сама каждые 6 часов, запустите join от администратора " +
			"(или поставьте задачу вручную — см. README)")
	}
	out, err := exec.Command("schtasks", "/Create", "/TN", taskName,
		"/TR", fmt.Sprintf(`"%s" agent`, exe),
		"/SC", "HOURLY", "/MO", "6", "/RU", "SYSTEM", "/RL", "HIGHEST", "/F").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Планировщик задач: %s", out)
	}
	return "задача «" + taskName + "» в Планировщике: каждые 6 часов, файл программы — " + exe +
		" (не перемещайте и не удаляйте его)", nil
}

func RemoveSchedule() error {
	exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()
	return nil
}

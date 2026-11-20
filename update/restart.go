package update

import (
	"errors"
	"os"
	"os/exec"
)

// restartedEnv — метка в окружении: «меня только что перезапустили после обновления».
// Перезапущенная программа второй раз не проверяет обновления — так не выйдет бесконечного цикла,
// даже если GitHub по какой-то причине снова скажет «есть новее».
const restartedEnv = "OFFICE_HEALTH_UPDATED"

// JustRestarted — эту программу только что запустили после обновления.
func JustRestarted() bool { return os.Getenv(restartedEnv) != "" }

func asExit(err error, target **exec.ExitError) bool { return errors.As(err, target) }

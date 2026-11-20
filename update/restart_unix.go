//go:build !windows

package update

import (
	"os"
	"syscall"
)

// Restart заменяет текущий процесс обновлённой программой (тот же номер процесса, те же параметры).
// На Mac и Linux так можно: syscall.Exec не возвращается, если всё прошло хорошо.
func Restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, append(os.Environ(), restartedEnv+"=1"))
}

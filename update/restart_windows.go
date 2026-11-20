//go:build windows

package update

import (
	"os"
	"os/exec"
)

// Restart запускает обновлённую программу с теми же параметрами и завершает текущую с её кодом.
// На Windows «заменить себя» нельзя, поэтому новая версия — дочерний процесс в том же окне:
// старая ждёт, пока новая закончит, и выходит с тем же кодом (его видит Планировщик задач).
func Restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), restartedEnv+"=1")
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		os.Exit(0)
	case asExit(err, &exitErr):
		os.Exit(exitErr.ExitCode())
	}
	return err
}

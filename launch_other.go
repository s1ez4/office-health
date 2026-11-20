//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// На Mac и Linux программу запускают из терминала — окно само не закрывается.
func startedByDoubleClick() bool { return false }

func openFile(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", path).Start()
	}
	return exec.Command("xdg-open", path).Start()
}

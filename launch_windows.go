//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// GetConsoleProcessList — функция Windows: «какие процессы сидят в этом окне консоли».
// Её нет в готовом виде в x/sys, поэтому берём прямо из системной библиотеки kernel32.dll.
var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// startedByDoubleClick — запустили двойным щелчком из Проводника?
// Тогда окно консоли создано только для нас (мы в нём одни) и закроется сразу после выхода —
// человек не успеет ничего прочитать. Если запустили из командной строки, в окне есть ещё cmd/PowerShell.
// Сеанс 0 — это службы и задачи «без входа пользователя»: там ждать Enter некому.
func startedByDoubleClick() bool {
	var ids [4]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if n != 1 {
		return false
	}
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil || session == 0 {
		return false
	}
	return true
}

//go:build windows

// Строчка выше — build tag: этот файл попадает в программу только при сборке под Windows.
// (Суффикс _windows.go в имени файла делает то же самое; метка — для наглядности.)

package collect

import "golang.org/x/sys/windows"

// platformCollectors — сборщики для Windows.
func platformCollectors() []Collector {
	return []Collector{diskCollector{}, batteryCollector{}, whea{}}
}

// smartctlKnownPaths — куда обычно ставится smartmontools на Windows.
var smartctlKnownPaths = []string{
	`C:\Program Files\smartmontools\bin\smartctl.exe`,
	`C:\Program Files (x86)\smartmontools\bin\smartctl.exe`,
}

func osName(platform, version string) string { return platform + " " + version }

func volumeLabel(mount string) (string, bool) { return mount, true }

// На Windows без smartctl дисков не узнать — в отчёте будет инструкция, как его поставить.
func fallbackDisks() ([]Disk, error) { return nil, nil }

// IsAdmin — запущена ли программа «от имени администратора».
// Windows выдаёт каждому процессу «пропуск» (token); у админского он «повышенный» (elevated).
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

//go:build windows

// Строчка выше — build tag: этот файл попадает в программу только при сборке под Windows.
// (Суффикс _windows.go в имени файла делает то же самое; метка — для наглядности.)

package collect

import "golang.org/x/sys/windows"

// platformCollectors — сборщики, которые есть только на Windows.
// Пока пусто; диски, батарея и ошибки железа появятся на следующих шагах.
func platformCollectors() []Collector {
	return nil
}

// IsAdmin — запущена ли программа «от имени администратора».
// Windows выдаёт каждому процессу «пропуск» (token); у админского он «повышенный» (elevated).
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

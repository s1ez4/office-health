// office-health — проверка «здоровья» компьютера: диски, батарея, ошибки железа.
//
// Запуск:
//
//	office-health.exe          — проверить этот компьютер (лучше от администратора)
//	office-health.exe --demo   — показать пример отчёта на выдуманных данных
//	office-health.exe --out D:\отчёты — куда сохранить отчёт
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"office-health/collect"
	"office-health/report"
	"office-health/rules"
)

// main — точка входа: отсюда программа начинает работать.
func main() {
	// Флаги командной строки. flag.Bool возвращает указатель: значение появится после flag.Parse().
	demo := flag.Bool("demo", false, "выдуманные данные вместо настоящих — посмотреть, как выглядит отчёт")
	out := flag.String("out", defaultOutDir(), "папка для отчётов")
	flag.Parse()

	var s collect.Snapshot
	if *demo {
		fmt.Println("Демо-режим: данные выдуманные.")
		s = collect.Demo()
	} else {
		if !collect.IsAdmin() {
			fmt.Println("⚠ Программа запущена НЕ от администратора.")
			fmt.Println("  Здоровье дисков и журнал ошибок без прав администратора недоступны.")
			fmt.Println("  Нажмите на office-health.exe правой кнопкой → «Запуск от имени администратора».")
			fmt.Println()
		}
		fmt.Println("Проверяю компьютер…")
		s = collect.Run()
	}

	r := rules.Evaluate(s)
	path, err := report.Write(*out, s, r)
	if err != nil {
		fmt.Println("Не удалось сохранить отчёт:", err)
		os.Exit(1) // код 1 — «завершилось с ошибкой»; Планировщик задач это увидит
	}

	fmt.Println("Компьютер:", s.System.Hostname)
	fmt.Println("Общий статус:", r.Overall.Text())
	for _, w := range s.Warnings {
		fmt.Println("Не удалось:", w)
	}
	fmt.Println("Отчёт:", path)
}

// defaultOutDir — папка reports рядом с программой.
// Именно рядом с .exe, а не «в текущей папке»: Планировщик задач запускает программы
// из C:\Windows\System32, и отчёты улетели бы туда.
// Исключение — запуск через `go run`: тогда .exe временный, и берём текущую папку.
func defaultOutDir() string {
	exe, err := os.Executable()
	if err != nil || strings.HasPrefix(exe, os.TempDir()) {
		return "reports"
	}
	return filepath.Join(filepath.Dir(exe), "reports")
}

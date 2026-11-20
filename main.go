// office-health — проверка «здоровья» компьютера: диски, батарея, ошибки железа.
//
// Запуск:
//
//	office-health.exe          — проверить этот компьютер (лучше от администратора)
//	office-health.exe --demo   — показать пример отчёта на выдуманных данных
//	office-health.exe --out D:\отчёты — куда сохранить отчёт
//	office-health.exe --quiet  — для Планировщика задач: не открывать отчёт и не ждать Enter
package main

import (
	"bufio"
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
	quiet := flag.Bool("quiet", false, "не открывать отчёт и не ждать Enter (для Планировщика задач)")
	flag.Parse()

	// Запустили двойным щелчком — в конце откроем отчёт и подождём Enter, чтобы окно не исчезло.
	interactive := !*quiet && startedByDoubleClick()

	code := run(*demo, *out, interactive)
	if interactive {
		fmt.Print("\nНажмите Enter, чтобы закрыть окно…")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(code) // 0 — всё прошло, 1 — ошибка; Планировщик задач показывает этот код
}

// run делает всю работу и возвращает код завершения.
func run(demo bool, out string, interactive bool) int {
	var s collect.Snapshot
	if demo {
		fmt.Println("Демо-режим: данные выдуманные.")
		s = collect.Demo()
	} else {
		if !collect.IsAdmin() {
			fmt.Println("⚠ Программа запущена НЕ от администратора.")
			fmt.Println("  Без прав администратора здоровье некоторых дисков может быть недоступно.")
			fmt.Println("  Нажмите на office-health.exe правой кнопкой → «Запуск от имени администратора».")
			fmt.Println()
		}
		fmt.Println("Проверяю компьютер…")
		s = collect.Run()
	}

	r := rules.Evaluate(s)
	path, err := report.Write(out, s, r)
	if err != nil {
		fmt.Println("Не удалось сохранить отчёт:", err)
		return 1
	}

	fmt.Println("Компьютер:", s.System.Hostname)
	fmt.Println("Общий статус:", r.Overall.Text())
	if r.Incomplete {
		fmt.Println("  (не всё удалось проверить — подробности в отчёте)")
	}
	for _, w := range s.Warnings {
		fmt.Println("Не удалось:", w)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	fmt.Println("Отчёт:", path)
	if interactive {
		openFile(path)
	}
	return 0
}

// defaultOutDir — папка reports рядом с программой.
// Именно рядом с .exe, а не «в текущей папке»: Планировщик задач запускает программы
// из C:\Windows\System32, и отчёты улетели бы туда.
// Исключение — запуск через `go run`: тогда .exe временный (во временной папке или в кэше
// сборки go-build), и берём текущую папку.
func defaultOutDir() string {
	exe, err := os.Executable()
	if err != nil || strings.HasPrefix(exe, os.TempDir()) || strings.Contains(exe, "go-build") {
		return "reports"
	}
	return filepath.Join(filepath.Dir(exe), "reports")
}

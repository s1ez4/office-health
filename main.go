// office-health — проверка «здоровья» компьютеров: диски, батарея, ошибки железа.
//
// Режимы (первое слово после имени программы — «подкоманда»):
//
//	office-health                 — приложение: панель в браузере, этот компьютер + добавленные
//	office-health app --demo      — приложение с выдуманными компьютерами (посмотреть)
//	office-health join <код>      — подключить этот компьютер агентом к владельцу
//	office-health agent           — агент: проверить и отправить владельцу (так его запускает расписание)
//	office-health leave           — перестать быть агентом
//	office-health report [--demo] [--out папка] [--quiet] — разовый HTML-отчёт в файл, как раньше
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"office-health/agent"
	"office-health/app"
	"office-health/collect"
	"office-health/report"
	"office-health/rules"
	"office-health/store"
)

func main() {
	args := os.Args[1:]
	cmd := "app"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	} else if len(args) > 0 {
		cmd = "report" // старый способ запуска с флагами (--quiet в Планировщике) — продолжает работать
	}

	var code int
	switch cmd {
	case "app":
		code = runApp(args)
	case "report":
		code = runReport(args)
	case "join":
		code = runJoin(args)
	case "agent":
		code = done(agent.Send(), "Проверка отправлена владельцу.")
	case "leave":
		code = done(agent.Leave(), "Этот компьютер больше не отправляет проверки.")
	case "help", "-h", "--help":
		fmt.Println(usage)
	default:
		fmt.Printf("Неизвестная команда %q.\n\n%s\n", cmd, usage)
		code = 2
	}
	pauseIfDoubleClicked(cmd)
	os.Exit(code) // 0 — всё хорошо, не 0 — ошибка (Планировщик задач показывает этот код)
}

const usage = `Использование:
  office-health                 приложение (панель в браузере)
  office-health app --demo      приложение с выдуманными компьютерами
  office-health join <код>      подключить этот компьютер к владельцу по приглашению
  office-health agent           проверить и отправить владельцу
  office-health leave           отключить этот компьютер от владельца
  office-health report [--demo] [--out папка] [--quiet]   отчёт в HTML-файл`

// done печатает результат команды и возвращает код завершения.
func done(err error, ok string) int {
	if err != nil {
		fmt.Println("Ошибка:", err)
		return 1
	}
	fmt.Println(ok)
	return 0
}

func runApp(args []string) int {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	demo := fs.Bool("demo", false, "выдуманные компьютеры — посмотреть, как всё выглядит")
	port := fs.Int("port", 8765, "порт панели (только этот компьютер)")
	agentPort := fs.Int("agent-port", 8766, "порт для данных от других компьютеров")
	noOpen := fs.Bool("no-browser", false, "не открывать браузер")
	data := fs.String("data", store.DefaultDir(), "папка с данными приложения")
	fs.Parse(args)

	opt := app.Options{Dir: *data, UIPort: *port, AgentPort: *agentPort, Open: !*noOpen, Demo: *demo}
	if *demo {
		dir, err := os.MkdirTemp("", "office-health-demo-")
		if err != nil {
			fmt.Println("Ошибка:", err)
			return 1
		}
		defer os.RemoveAll(dir)
		opt.Dir = dir
	}
	if app.AlreadyRunning(opt) { // второй запуск — просто показать уже работающее приложение
		fmt.Println("Office Health уже работает — открываю панель.")
		app.OpenBrowser(opt.UIURL())
		return 0
	}
	if err := app.Run(opt); err != nil {
		fmt.Println("Ошибка:", err)
		return 1
	}
	return 0
}

func runJoin(args []string) int {
	if len(args) != 1 {
		fmt.Println("Нужен код приглашения: office-health join OH1-…\nКод даёт владелец: «Добавить компьютер» в его панели.")
		return 2
	}
	if err := agent.Join(args[0]); err != nil {
		fmt.Println("Ошибка:", err)
		return 1
	}
	fmt.Println("Готово: этот компьютер подключён, первая проверка отправлена владельцу.")
	if msg, err := agent.InstallSchedule(); err != nil {
		fmt.Println("Расписание не поставлено:", err)
	} else {
		fmt.Println("Дальше — " + msg + ".")
	}
	return 0
}

// runReport — прежний режим: проверить и сохранить HTML-отчёт в файл.
func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	demo := fs.Bool("demo", false, "выдуманные данные вместо настоящих — посмотреть, как выглядит отчёт")
	out := fs.String("out", defaultOutDir(), "папка для отчётов")
	quiet := fs.Bool("quiet", false, "не открывать отчёт (для Планировщика задач)")
	demoOS := fs.String("demo-os", runtime.GOOS, "для --demo: какой компьютер выдумать — windows или darwin (Mac)")
	fs.Parse(args)

	var s collect.Snapshot
	if *demo {
		fmt.Println("Демо-режим: данные выдуманные.")
		s = collect.DemoFor(*demoOS)
	} else {
		if !collect.IsAdmin() && collect.NeedsAdmin {
			fmt.Println("⚠ Программа запущена НЕ от администратора.")
			fmt.Println("  Без прав администратора здоровье некоторых дисков может быть недоступно.")
			fmt.Println("  " + collect.AdminHint)
			fmt.Println()
		}
		fmt.Println("Проверяю компьютер…")
		s = collect.Run()
	}

	r := rules.Evaluate(s)
	path, err := report.Write(*out, s, r)
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
	if !*quiet && startedByDoubleClick() {
		app.OpenBrowser(path)
	}
	return 0
}

// pauseIfDoubleClicked — на Windows при запуске двойным щелчком окно консоли закрылось бы сразу.
// Приложению это не нужно (оно и так работает, пока его не выключат), а остальным режимам — да.
func pauseIfDoubleClicked(cmd string) {
	if cmd == "app" || !startedByDoubleClick() {
		return
	}
	fmt.Print("\nНажмите Enter, чтобы закрыть окно…")
	bufio.NewReader(os.Stdin).ReadString('\n')
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

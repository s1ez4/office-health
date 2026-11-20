//go:build !windows

// Этот файл — для всех ОС, кроме Windows («!» значит «не»).
// Когда дойдём до Mac, появится platform_darwin.go (свои battery и kernel panic вместо WHEA),
// а здесь метка станет «!windows && !darwin».

package collect

import "os"

// На других ОС пока только диски: smartctl там работает так же.
// Батарея и журнал ошибок — Windows-специфичные, для Mac добавим свои сборщики.
func platformCollectors() []Collector {
	return []Collector{diskCollector{}}
}

var smartctlKnownPaths = []string{
	"/usr/local/sbin/smartctl",   // Homebrew на Intel-Mac, Linux
	"/opt/homebrew/sbin/smartctl", // Homebrew на Apple Silicon
	"/usr/sbin/smartctl",
}

// IsAdmin на Mac и Linux: администратор (root) — пользователь с номером 0.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

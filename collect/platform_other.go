//go:build !windows

// Этот файл — для всех ОС, кроме Windows («!» значит «не»).
// Когда дойдём до Mac, появится platform_darwin.go, а здесь метка станет «!windows && !darwin».

package collect

import "os"

func platformCollectors() []Collector {
	return nil
}

// IsAdmin на Mac и Linux: администратор (root) — пользователь с номером 0.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

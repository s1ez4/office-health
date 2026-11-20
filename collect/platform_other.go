//go:build !windows && !darwin

// Этот файл — для всех ОС, кроме Windows и Mac (то есть для Linux).
// Там пока только система и диски: smartctl работает так же.

package collect

import "os"

func platformCollectors() []Collector {
	return []Collector{diskCollector{}}
}

var smartctlKnownPaths = []string{"/usr/sbin/smartctl", "/usr/local/sbin/smartctl"}

// IsAdmin на Linux: администратор (root) — пользователь с номером 0.
func IsAdmin() bool {
	return os.Geteuid() == 0
}

const NeedsAdmin = true // smartctl на Linux требует root

func osName(platform, version string) string { return platform + " " + version }

func volumeLabel(mount string) (string, bool) { return mount, true }

func fallbackDisks() ([]Disk, error) { return nil, nil }

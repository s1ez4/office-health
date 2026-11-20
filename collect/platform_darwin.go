//go:build darwin

// Всё, что специфично для macOS. Разбор ответов — в mac_parse.go (там его проверяют тесты).

package collect

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

func platformCollectors() []Collector {
	return []Collector{diskCollector{}, macBattery{}, macPanics{}}
}

var smartctlKnownPaths = []string{
	"/opt/homebrew/sbin/smartctl", // Homebrew на Apple Silicon (M1, M2…)
	"/usr/local/sbin/smartctl",    // Homebrew на Intel-маках
	"/opt/local/sbin/smartctl",    // MacPorts
}

// IsAdmin на Mac: администратор (root) — пользователь с номером 0 (запуск через sudo).
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// На Mac smartctl без sudo может не открыть диск — стоит предупредить.
const NeedsAdmin = true

// init() вызывается сам при старте программы, ещё до main(). Здесь подменяем «общие» функции
// на маковские — файлы для Windows при этом не меняются.
func init() {
	AdminHint = "Запустите через sudo: sudo ./office-health report (Mac попросит пароль от вашей учётной записи)."
	machineModel = func() (string, string) {
		out, err := exec.Command("system_profiler", "SPHardwareDataType", "-json").Output()
		if err != nil {
			return "", ""
		}
		return parseHardwareProfile(out)
	}
}

func osName(platform, version string) string { return "macOS " + version }

// volumeLabel: на Mac десятки служебных разделов APFS. Показываем только данные пользователя
// («Macintosh HD») и подключённые диски из /Volumes.
func volumeLabel(mount string) (string, bool) {
	switch {
	case mount == "/System/Volumes/Data":
		return "Macintosh HD", true
	case strings.HasPrefix(mount, "/Volumes/"):
		return strings.TrimPrefix(mount, "/Volumes/"), true
	}
	return "", false
}

// attachVolumes — на Mac букв дисков нет; сопоставление дисков и томов APFS пока не делаем.
func attachVolumes(disks []Disk) {}

// fallbackDisks — smartctl нет (обычная ситуация на Mac): берём хотя бы общий вердикт SMART у diskutil.
func fallbackDisks() ([]Disk, error) {
	out, err := exec.Command("diskutil", "list", "physical").Output()
	if err != nil {
		return nil, err
	}
	var disks []Disk
	for _, id := range parseDiskutilList(string(out)) {
		info, err := exec.Command("diskutil", "info", id).Output()
		if err != nil {
			disks = append(disks, Disk{Device: "/dev/" + id, Error: err.Error()})
			continue
		}
		disks = append(disks, parseDiskutilInfo(id, string(info)))
	}
	return disks, nil
}

type macBattery struct{}

func (macBattery) Name() string { return "батарея" }

func (macBattery) Collect(s *Snapshot) error {
	// ioreg — «реестр устройств» macOS. -r -n AppleSmartBattery: показать только батарею.
	out, err := exec.Command("ioreg", "-rn", "AppleSmartBattery").Output()
	if err != nil {
		return err
	}
	b := parseIoregBattery(string(out))
	s.Battery = &b
	return nil
}

type macPanics struct{}

func (macPanics) Name() string { return "отчёты kernel panic" }

func (macPanics) Collect(s *Snapshot) error {
	dirs := []string{"/Library/Logs/DiagnosticReports", "/Library/Logs/DiagnosticReports/Retired"}
	res := collectPanics(dirs, time.Now(), HWErrorDays)
	s.HWErrors = &res
	return nil
}

package collect

// Разбор ответов маковских программ: ioreg (батарея), diskutil (диски) и отчётов kernel panic.
// Файл без метки сборки — он есть во всех версиях программы. Это нужно, чтобы тесты могли
// проверить разбор на Windows по образцам. Сам запуск этих программ — в *_darwin.go.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Регулярное выражение — шаблон для поиска в тексте. Этот ловит строки вида:  "CycleCount" = 112
// (\w+) и (\d+) — «группы»: то, что в скобках, мы потом достанем (имя и число).
var ioregNumber = regexp.MustCompile(`^\s*"(\w+)" = (\d+)\s*$`)

// parseIoregBattery разбирает `ioreg -rn AppleSmartBattery`. Пустой ответ — батареи нет (Mac mini, iMac).
func parseIoregBattery(out string) Battery {
	vals := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		if m := ioregNumber.FindStringSubmatch(line); m != nil {
			n, _ := strconv.ParseInt(m[2], 10, 64)
			if _, seen := vals[m[1]]; !seen { // первое вхождение — главное (дальше бывают вложенные копии)
				vals[m[1]] = n
			}
		}
	}
	design := vals["DesignCapacity"]
	if design == 0 {
		return Battery{Present: false}
	}
	// На Apple Silicon MaxCapacity — это уже проценты (100), а настоящие мА·ч — в AppleRawMaxCapacity.
	// На старых Intel-маках AppleRawMaxCapacity может не быть, тогда MaxCapacity — мА·ч.
	full := vals["AppleRawMaxCapacity"]
	if full == 0 && vals["MaxCapacity"] > 100 {
		full = vals["MaxCapacity"]
	}
	b := Battery{Present: true, Designed: design, FullCharge: full, Unit: "mAh"}
	b.HealthPct = float64(full) / float64(design) * 100
	if c, ok := vals["CycleCount"]; ok {
		b.Cycles = ptr(c)
	}
	return b
}

var diskutilDisk = regexp.MustCompile(`^/dev/(disk\d+) \(([^)]*)\):`)

// parseDiskutilList достаёт из `diskutil list physical` имена физических дисков: disk0, disk4…
func parseDiskutilList(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		if m := diskutilDisk.FindStringSubmatch(line); m != nil && strings.Contains(m[2], "physical") {
			ids = append(ids, m[1])
		}
	}
	return ids
}

var diskutilBytes = regexp.MustCompile(`\((\d+) Bytes\)`)

// parseDiskutilInfo разбирает `diskutil info disk0`: строки «Ключ: значение».
// Тут нет износа и секторов — только модель, размер, тип и общий вердикт SMART.
func parseDiskutilInfo(id, out string) Disk {
	info := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, ":") // Cut — разрезать строку по первому «:»
		if ok {
			info[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	d := Disk{Device: "/dev/" + id, Model: info["Device / Media Name"], Kind: "Диск"}
	if m := diskutilBytes.FindStringSubmatch(info["Disk Size"]); m != nil {
		n, _ := strconv.ParseFloat(m[1], 64)
		d.CapacityGB = n / gb
	}
	switch info["Solid State"] {
	case "Yes":
		d.Kind = "SSD"
	case "No":
		d.Kind = "HDD"
	}
	switch info["SMART Status"] {
	case "Verified":
		d.SmartPassed = ptr(true)
	case "Failing":
		d.SmartPassed = ptr(false)
	}
	return d
}

// collectPanics ищет отчёты kernel panic (файлы *.panic) в папках dirs за последние days дней.
// Kernel panic — аварийная остановка ядра macOS: Mac внезапно перезагружается с сообщением
// «Компьютер был перезагружен из-за ошибки». Частые паники — признак проблем с железом.
func collectPanics(dirs []string, now time.Time, days int) HWErrors {
	res := HWErrors{Source: SourcePanic, Days: days}
	since := now.AddDate(0, 0, -days)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // папки может не быть или нет прав — просто пропускаем
		}
		for _, e := range entries {
			if e.IsDir() || !strings.Contains(e.Name(), ".panic") {
				continue
			}
			fi, err := e.Info()
			if err != nil || fi.ModTime().Before(since) {
				continue
			}
			res.Recent = append(res.Recent, HWEvent{
				Time:    fi.ModTime(),
				Message: panicSummary(filepath.Join(dir, e.Name())),
			})
		}
	}
	res.Count = len(res.Recent)
	// sort.Slice сортирует по нашему правилу: новые сверху
	sort.Slice(res.Recent, func(i, j int) bool { return res.Recent[i].Time.After(res.Recent[j].Time) })
	if len(res.Recent) > HWErrorRecent {
		res.Recent = res.Recent[:HWErrorRecent] // [:5] — «первые пять»
	}
	return res
}

var panicString = regexp.MustCompile(`"panicString"\s*:\s*"([^"\\]*)`)

// panicSummary — первая строка причины паники из файла, или просто его имя.
func panicSummary(path string) string {
	data, err := os.ReadFile(path)
	if err == nil {
		if m := panicString.FindSubmatch(data); m != nil {
			if s := strings.TrimSpace(string(m[1])); s != "" {
				if len(s) > 160 {
					s = s[:160] + "…"
				}
				return "Kernel panic: " + s
			}
		}
	}
	return "Kernel panic: " + filepath.Base(path)
}

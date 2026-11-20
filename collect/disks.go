package collect

// Диски: спрашиваем у программы smartctl (пакет smartmontools) «медкарту» каждого диска — SMART.
// smartctl есть и для Windows, и для Mac, и для Linux, поэтому этот файл общий для всех ОС.
//
// Как это работает:
//  1. smartctl --scan -j          → список дисков
//  2. smartctl -a -j -d <тип> <диск> → всё про один диск в формате JSON
//  3. разбираем JSON в Go-структуры (encoding/json) и вытаскиваем нужные цифры

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type diskCollector struct{}

func (diskCollector) Name() string { return "диски" }

func (diskCollector) Collect(s *Snapshot) error {
	path := findSmartctl()
	if path == "" {
		// Не ошибка программы: в отчёте покажем, как поставить smartctl.
		// А на Mac пока возьмём хоть что-то у встроенной diskutil.
		s.Disks.SmartctlFound = false
		list, err := fallbackDisks()
		if len(list) > 0 {
			s.Disks.Source = "diskutil"
			s.Disks.List = list
		}
		return err
	}
	s.Disks.SmartctlFound = true
	s.Disks.SmartctlPath = path
	s.Disks.Source = "smartctl"

	devices, err := scanDisks(path)
	if err != nil {
		return err
	}
	for _, d := range devices {
		s.Disks.List = append(s.Disks.List, readDisk(path, d))
	}
	return nil
}

// findSmartctl ищет smartctl: рядом с нашей программой, потом в PATH, потом в обычных местах установки.
func findSmartctl() string {
	name := "smartctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	if p, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, smartctlKnownPaths...) // свой список у каждой ОС: platform_*.go
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// scannedDisk — одна строчка из `smartctl --scan -j`.
type scannedDisk struct {
	Name string `json:"name"` // /dev/sda
	Type string `json:"type"` // ata, nvme, sat…
}

func scanDisks(smartctl string) ([]scannedDisk, error) {
	out, _ := exec.Command(smartctl, "--scan", "-j").Output()
	var res struct {
		Devices []scannedDisk `json:"devices"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("не понял ответ smartctl --scan: %w", err)
	}
	return res.Devices, nil
}

func readDisk(smartctl string, d scannedDisk) Disk {
	args := []string{"-a", "-j"}
	if d.Type != "" {
		args = append(args, "-d", d.Type)
	}
	args = append(args, d.Name)
	// smartctl возвращает ненулевой код возврата даже когда всё прочитал (биты кода — «флажки» о здоровье),
	// поэтому ошибку запуска не смотрим, а разбираем то, что он напечатал.
	out, _ := exec.Command(smartctl, args...).Output()
	disk, err := parseSmart(out)
	if disk.Device == "" {
		disk.Device = d.Name
	}
	if err != nil {
		disk.Error = err.Error()
	}
	return disk
}

// smartJSON — та часть ответа `smartctl -a -j`, которая нам нужна. Остальное encoding/json пропустит сам.
// Имена полей JSON сопоставляются без учёта регистра: поле Passed поймает "passed".
type smartJSON struct {
	Smartctl struct {
		ExitStatus int `json:"exit_status"`
		Messages   []struct {
			String   string `json:"string"`
			Severity string `json:"severity"`
		} `json:"messages"`
	} `json:"smartctl"`
	Device struct {
		Name     string `json:"name"`
		Protocol string `json:"protocol"` // ATA, NVMe, SCSI
	} `json:"device"`
	ModelName        string                 `json:"model_name"`
	SerialNumber     string                 `json:"serial_number"`
	UserCapacity     struct{ Bytes int64 }  `json:"user_capacity"`
	NVMeCapacity     int64                  `json:"nvme_total_capacity"`
	LogicalBlockSize int64                  `json:"logical_block_size"`
	RotationRate     *int                   `json:"rotation_rate"` // 0 — SSD, 5400/7200 — HDD (обороты в минуту)
	SmartStatus      *struct{ Passed bool } `json:"smart_status"`
	PowerOnTime      *struct{ Hours int64 } `json:"power_on_time"`
	ATAAttributes    struct {
		Table []ataAttr `json:"table"`
	} `json:"ata_smart_attributes"`
	ATAStatistics struct {
		Pages []struct {
			Table []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"table"`
		} `json:"pages"`
	} `json:"ata_device_statistics"`
	NVMe *struct {
		PercentageUsed   int64 `json:"percentage_used"`
		DataUnitsWritten int64 `json:"data_units_written"`
		PowerOnHours     int64 `json:"power_on_hours"`
	} `json:"nvme_smart_health_information_log"`
}

// ataAttr — один SMART-атрибут HDD/SATA-диска: номер, имя, «нормализованное» значение и «сырое».
type ataAttr struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Value int64  `json:"value"` // нормализованное: обычно 100 (или 200, 253) — «как новый», падает с износом
	Raw   struct {
		Value int64 `json:"value"` // сырое: настоящее число — часы, сектора…
	} `json:"raw"`
}

const tb = 1e12 // производители дисков считают терабайт как 10^12 байт

// parseSmart превращает JSON от smartctl в нашу структуру Disk.
// Она отдельная (и не запускает smartctl), чтобы её можно было проверить тестом на образцах.
func parseSmart(data []byte) (Disk, error) {
	var j smartJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return Disk{}, fmt.Errorf("не понял ответ smartctl: %w", err)
	}
	d := Disk{
		Device: j.Device.Name,
		Model:  strings.TrimSpace(j.ModelName),
		Serial: strings.TrimSpace(j.SerialNumber),
	}
	// Биты 0 и 1 кода возврата — «не смог даже открыть диск». Остальные биты — про здоровье, это не ошибка.
	if j.Smartctl.ExitStatus&0b11 != 0 {
		return d, errors.New(smartctlMessage(j))
	}

	bytes := j.UserCapacity.Bytes
	if bytes == 0 {
		bytes = j.NVMeCapacity
	}
	d.CapacityGB = float64(bytes) / gb
	if j.SmartStatus != nil {
		d.SmartPassed = ptr(j.SmartStatus.Passed)
	}
	if j.PowerOnTime != nil {
		d.PowerOnHours = ptr(j.PowerOnTime.Hours)
	}

	switch {
	case j.NVMe != nil || strings.EqualFold(j.Device.Protocol, "NVMe"):
		d.Kind = "NVMe"
		if j.NVMe != nil {
			d.PercentUsed = ptr(j.NVMe.PercentageUsed)
			// В NVMe «единица данных» = 1000 блоков по 512 байт
			d.WrittenTB = ptr(float64(j.NVMe.DataUnitsWritten) * 512 * 1000 / tb)
			if d.PowerOnHours == nil {
				d.PowerOnHours = ptr(j.NVMe.PowerOnHours)
			}
		}
	case j.RotationRate != nil && *j.RotationRate == 0:
		d.Kind = "SSD"
	case j.RotationRate != nil:
		d.Kind = "HDD"
	default:
		d.Kind = "Диск"
	}

	// SMART-атрибуты (бывают у HDD и SATA SSD; у NVMe их нет)
	attrs := map[int]ataAttr{} // map — «словарь»: номер атрибута → атрибут
	for _, a := range j.ATAAttributes.Table {
		attrs[a.ID] = a
	}
	raw := func(id int) *int64 { // функция внутри функции: достать сырое значение, если атрибут есть
		if a, ok := attrs[id]; ok {
			return ptr(a.Raw.Value & 0xFFFFFFFF) // младшие 32 бита: некоторые производители прячут в старших мусор
		}
		return nil
	}
	d.Reallocated = raw(5)
	d.Pending = raw(197)
	d.Uncorrectable = raw(198)
	if d.PowerOnHours == nil {
		d.PowerOnHours = raw(9)
	}

	if d.Kind == "SSD" {
		d.PercentUsed, d.WrittenTB = sataSSDWear(j, attrs)
	}
	return d, nil
}

// sataSSDWear — износ SATA SSD. Стандартного поля, как у NVMe, нет: пробуем по очереди
// «статистику устройства», потом атрибуты разных производителей.
func sataSSDWear(j smartJSON, attrs map[int]ataAttr) (*int64, *float64) {
	var used *int64
	var written *float64
	block := j.LogicalBlockSize
	if block == 0 {
		block = 512
	}
	for _, page := range j.ATAStatistics.Pages {
		for _, row := range page.Table {
			switch row.Name {
			case "Percentage Used Endurance Indicator":
				used = ptr(row.Value)
			case "Logical Sectors Written":
				written = ptr(float64(row.Value*block) / tb)
			}
		}
	}
	if used == nil {
		// У этих атрибутов нормализованное значение = «сколько ресурса осталось, %»
		for _, id := range []int{231, 233, 177, 202} { // SSD_Life_Left, Media_Wearout, Wear_Leveling, Lifetime_Remain
			if a, ok := attrs[id]; ok && a.Value <= 100 {
				used = ptr(100 - a.Value)
				break
			}
		}
	}
	if a, ok := attrs[241]; ok && written == nil && strings.Contains(a.Name, "LBA") { // Total_LBAs_Written
		written = ptr(float64(a.Raw.Value*block) / tb)
	}
	return used, written
}

// smartctlMessage — понятная причина, почему smartctl не прочитал диск.
func smartctlMessage(j smartJSON) string {
	for _, m := range j.Smartctl.Messages {
		if m.Severity == "error" {
			if strings.Contains(strings.ToLower(m.String), "access denied") ||
				strings.Contains(strings.ToLower(m.String), "permission denied") {
				return "нет доступа — запустите программу от администратора"
			}
			return m.String
		}
	}
	return fmt.Sprintf("smartctl завершился с кодом %d", j.Smartctl.ExitStatus)
}

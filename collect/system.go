package collect

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// System — общие сведения о компьютере.
type System struct {
	Hostname    string   `json:"hostname"`
	OS          string   `json:"os"`
	CPU         string   `json:"cpu"`
	CPUCores    int      `json:"cpu_cores"`
	RAMTotalGB  float64  `json:"ram_total_gb"`
	RAMUsedPct  float64  `json:"ram_used_pct"`
	UptimeHours float64  `json:"uptime_hours"`
	Volumes     []Volume `json:"volumes"` // разделы: C:, D: …
}

// Volume — один раздел диска и насколько он заполнен.
type Volume struct {
	Mount   string  `json:"mount"` // «C:»
	FS      string  `json:"fs"`    // файловая система: NTFS, FAT32…
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
	UsedPct float64 `json:"used_pct"`
}

const gb = 1024 * 1024 * 1024 // байт в гигабайте

// systemCollector работает на любой ОС: gopsutil сам знает, как спросить у Windows, Mac или Linux.
// struct{} — пустая структура: данных у сборщика нет, только методы.
type systemCollector struct{}

func (systemCollector) Name() string { return "система" }

func (systemCollector) Collect(s *Snapshot) error {
	info, err := host.Info()
	if err != nil {
		return err
	}
	s.System.Hostname = info.Hostname
	s.System.OS = strings.TrimSpace(info.Platform + " " + info.PlatformVersion)
	s.System.UptimeHours = (time.Duration(info.Uptime) * time.Second).Hours()

	// Процессор: название берём у первого, ядра считаем логические (с учётом Hyper-Threading).
	if cpus, err := cpu.Info(); err == nil && len(cpus) > 0 {
		s.System.CPU = strings.TrimSpace(cpus[0].ModelName)
	}
	if n, err := cpu.Counts(true); err == nil {
		s.System.CPUCores = n
	}

	if m, err := mem.VirtualMemory(); err == nil {
		s.System.RAMTotalGB = float64(m.Total) / gb
		s.System.RAMUsedPct = m.UsedPercent
	}

	// Разделы. false — только «настоящие» диски, без виртуальных.
	parts, err := disk.Partitions(false)
	if err != nil {
		return err
	}
	for _, p := range parts {
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue // например, пустой картридер — пропускаем
		}
		s.System.Volumes = append(s.System.Volumes, Volume{
			Mount:   p.Mountpoint,
			FS:      p.Fstype,
			TotalGB: float64(u.Total) / gb,
			FreeGB:  float64(u.Free) / gb,
			UsedPct: u.UsedPercent,
		})
	}
	return nil
}

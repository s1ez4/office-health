package collect

import "time"

// ptr — маленький помощник: превращает значение в указатель на него.
// [T any] — это «дженерик»: функция работает с любым типом T (int64, float64, bool…),
// чтобы не писать отдельную копию для каждого.
func ptr[T any](v T) *T { return &v }

// Demo — правдоподобный выдуманный компьютер для флага --demo:
// один здоровый SSD, один умирающий HDD, батарея 55%, 3 ошибки WHEA.
func Demo() Snapshot {
	now := time.Now()
	return Snapshot{
		CollectedAt: now,
		Platform:    "windows",
		Demo:        true,
		IsAdmin:     true,
		System: System{
			Hostname:    "BUH-PC-03",
			OS:          "Microsoft Windows 10 Pro 22H2",
			CPU:         "Intel(R) Core(TM) i5-8400 CPU @ 2.80GHz",
			CPUCores:    6,
			RAMTotalGB:  7.9,
			RAMUsedPct:  71,
			UptimeHours: 312.4,
			Volumes: []Volume{
				{Mount: "C:", FS: "NTFS", TotalGB: 465.1, FreeGB: 31.2, UsedPct: 93.3},
				{Mount: "D:", FS: "NTFS", TotalGB: 931.5, FreeGB: 402.8, UsedPct: 56.8},
			},
		},
		Disks: Disks{
			SmartctlFound: true,
			SmartctlPath:  `C:\Program Files\smartmontools\bin\smartctl.exe`,
			List: []Disk{
				{
					Device: "/dev/nvme0", Model: "Samsung SSD 980 500GB", Serial: "S64DNX0R123456",
					Kind: "NVMe", CapacityGB: 465.8, SmartPassed: ptr(true), PowerOnHours: ptr(int64(6120)),
					PercentUsed: ptr(int64(12)), WrittenTB: ptr(18.4), Volumes: []string{"C:"},
				},
				{
					Device: "/dev/sda", Model: "WDC WD10EZEX-08WN4A0", Serial: "WD-WCC6Y0ABCDEF",
					Kind: "HDD", CapacityGB: 931.5, SmartPassed: ptr(true), PowerOnHours: ptr(int64(41230)),
					Reallocated: ptr(int64(128)), Pending: ptr(int64(24)), Uncorrectable: ptr(int64(8)),
					Volumes: []string{"D:"},
				},
			},
		},
		Battery: &Battery{Present: true, Designed: 50000, FullCharge: 27500, Unit: "mWh", HealthPct: 55, Cycles: ptr(int64(812))},
		HWErrors: &HWErrors{
			Source: SourceWHEA,
			Days:   30,
			Count:  3,
			Recent: []HWEvent{
				{Time: now.Add(-26 * time.Hour), ID: 19, Message: "Произошла исправленная аппаратная ошибка. Тип компонента: Ядро процессора"},
				{Time: now.Add(-6 * 24 * time.Hour), ID: 17, Message: "Произошла исправленная аппаратная ошибка. Тип компонента: PCI Express Root Port"},
				{Time: now.Add(-19 * 24 * time.Hour), ID: 47, Message: "Обнаружена и исправлена ошибка памяти"},
			},
		},
	}
}

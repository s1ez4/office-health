package collect

import "time"

// ptr — маленький помощник: превращает значение в указатель на него.
// [T any] — это «дженерик»: функция работает с любым типом T (int64, float64, bool…),
// чтобы не писать отдельную копию для каждого.
func ptr[T any](v T) *T { return &v }

// DemoFor — выдуманный компьютер нужной системы: "darwin" — Mac, иначе Windows.
func DemoFor(platform string) Snapshot {
	if platform == "darwin" {
		return DemoMac()
	}
	return Demo()
}

// DemoMac — выдуманный MacBook на M1: встроенный SSD отдаёт smartctl не всё (нет секторов —
// у NVMe их и не бывает), батарее 287 циклов, одна аварийная перезагрузка за месяц.
func DemoMac() Snapshot {
	now := time.Now()
	return Snapshot{
		CollectedAt: now,
		Platform:    "darwin",
		Demo:        true,
		IsAdmin:     true,
		System: System{
			Hostname: "Office-MacBook-Pro", Model: "MacBook Pro (MacBookPro18,3)",
			OS: "macOS 15.1", CPU: "Apple M1 Pro", CPUCores: 10, RAMTotalGB: 16, RAMUsedPct: 68, UptimeHours: 77,
			Volumes: []Volume{{Mount: "Macintosh HD", FS: "apfs", TotalGB: 460.4, FreeGB: 82.6, UsedPct: 82.1}},
		},
		Disks: Disks{
			SmartctlFound: true, SmartctlPath: "/opt/homebrew/sbin/smartctl", Source: "smartctl",
			List: []Disk{{
				Device: "/dev/disk0", Model: "APPLE SSD AP0512Q", Serial: "0ba01234a1b2c3d4", Kind: "NVMe",
				CapacityGB: 465.9, SmartPassed: ptr(true), PowerOnHours: ptr(int64(2840)),
				PercentUsed: ptr(int64(3)), WrittenTB: ptr(41.2), Volumes: []string{"Macintosh HD"},
			}},
		},
		Battery: &Battery{Present: true, Designed: 4382, FullCharge: 3772, Unit: "mAh", HealthPct: 86.1,
			Cycles: ptr(int64(287)), DesignCycles: ptr(int64(1000))},
		HWErrors: &HWErrors{
			Source: SourcePanic, Days: 30, Count: 1,
			Recent: []HWEvent{{Time: now.Add(-9 * 24 * time.Hour),
				Message: "Kernel panic: panic(cpu 4 caller 0xfffffe0029a1c2d4): watchdog timeout: no checkins from watchdogd in 90 seconds"}},
		},
	}
}

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

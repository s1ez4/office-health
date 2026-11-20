package app

import (
	"time"

	"office-health/collect"
	"office-health/store"
)

// fillDemo наполняет пустое хранилище выдуманными компьютерами (office-health app --demo):
// здоровый «этот», умирающая бухгалтерия с историей, Mac на ресепшене со старой батареей и
// склад, который давно молчит.
func fillDemo(st *store.Store) error {
	if _, ok := st.Latest(store.SelfID); ok {
		return nil // уже заполнено
	}
	now := time.Now()
	n := func(v int64) *int64 { return &v }
	f := func(v float64) *float64 { return &v }
	yes := true

	// «Этот» — здоровый ноутбук
	self := collect.Demo()
	self.Demo = false
	self.System.Hostname = "DIRECTOR-LAPTOP"
	self.System.OS = "Microsoft Windows 11 Pro 24H2"
	self.System.CPU = "AMD Ryzen 7 5800H with Radeon Graphics"
	self.System.CPUCores = 16
	self.System.RAMTotalGB = 15.4
	self.System.Volumes = []collect.Volume{{Mount: "C:", FS: "NTFS", TotalGB: 953, FreeGB: 318, UsedPct: 66.6}}
	self.Disks.List = []collect.Disk{{Device: "/dev/sda", Model: "WDC PC SN530 SDBPNPZ-1T00", Kind: "NVMe",
		CapacityGB: 953.9, SmartPassed: &yes, PowerOnHours: n(3855), PercentUsed: n(1), WrittenTB: f(26.1)}}
	self.Battery = &collect.Battery{Present: true, Designed: 57488, FullCharge: 45630, Unit: "mWh", HealthPct: 79.4}
	self.HWErrors = &collect.HWErrors{Source: collect.SourceWHEA, Days: 30}
	if err := st.AddSnapshot(store.SelfID, self); err != nil {
		return err
	}

	add := func(name string, snaps []collect.Snapshot, lastSeen time.Time) error {
		_, token, err := st.NewInvite()
		if err != nil {
			return err
		}
		id, _, err := st.Join(token, name, snaps[0].Platform)
		if err != nil {
			return err
		}
		for _, s := range snaps {
			if err := st.AddSnapshot(id, s); err != nil {
				return err
			}
		}
		return st.SetLastSeen(id, lastSeen)
	}

	// Бухгалтерия — HDD умирает, сектора растут от проверки к проверке
	var buh []collect.Snapshot
	for i, sectors := range []int64{6, 31, 74, 128} {
		s := collect.Demo()
		s.Demo = false
		s.CollectedAt = now.Add(time.Duration(i-3) * 30 * time.Hour)
		s.Disks.List[1].Reallocated = n(sectors)
		buh = append(buh, s)
	}
	if err := add("Бухгалтерия", buh, now.Add(-20*time.Minute)); err != nil {
		return err
	}

	// Ресепшен — iMac/MacBook: батарея старая, диск только через diskutil
	mac := collect.Snapshot{
		CollectedAt: now.Add(-2 * time.Hour), Platform: "darwin",
		System: collect.System{Hostname: "Reception-MacBook", OS: "macOS 14.6", CPU: "Apple M1", CPUCores: 8,
			RAMTotalGB: 8, RAMUsedPct: 64, UptimeHours: 140,
			Volumes: []collect.Volume{{Mount: "Macintosh HD", FS: "apfs", TotalGB: 228, FreeGB: 41, UsedPct: 82}}},
		Disks: collect.Disks{Source: "diskutil", List: []collect.Disk{{Device: "/dev/disk0", Model: "APPLE SSD AP0256Q",
			Kind: "SSD", CapacityGB: 233, SmartPassed: &yes}}},
		Battery:  &collect.Battery{Present: true, Designed: 4382, FullCharge: 2280, Unit: "mAh", HealthPct: 52, Cycles: n(911)},
		HWErrors: &collect.HWErrors{Source: collect.SourcePanic, Days: 30},
	}
	if err := add("Ресепшен", []collect.Snapshot{mac}, now.Add(-2*time.Hour)); err != nil {
		return err
	}

	// Склад — всё было хорошо, но 4 дня молчит
	sklad := self
	sklad.System.Hostname = "SKLAD-PC"
	sklad.System.OS = "Microsoft Windows 10 Pro 22H2"
	sklad.Battery = &collect.Battery{Present: false}
	sklad.CollectedAt = now.Add(-100 * time.Hour)
	return add("Склад", []collect.Snapshot{sklad}, now.Add(-100*time.Hour))
}

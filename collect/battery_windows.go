//go:build windows

package collect

// Батарея ноутбука через WMI — встроенную «справочную» Windows про железо.
// Ей задают вопросы на языке WQL (похож на SQL): SELECT поле FROM класс.
//
//   - Win32_Battery (раздел root\cimv2)           — есть ли батарея вообще
//   - BatteryStaticData.DesignedCapacity (root\wmi) — ёмкость новой батареи
//   - BatteryFullChargedCapacity.FullChargedCapacity (root\wmi) — сколько влезает сейчас
//
// Здоровье = сейчас / новая × 100%.

import (
	"errors"

	"github.com/yusufpapurcu/wmi"
)

type batteryCollector struct{}

func (batteryCollector) Name() string { return "батарея" }

// Структуры для ответов WMI: имена полей должны совпадать с именами в WMI.
type win32Battery struct{ Name string }
type batteryStaticData struct{ DesignedCapacity uint32 }
type batteryFullCharged struct{ FullChargedCapacity uint32 }

func (batteryCollector) Collect(s *Snapshot) error {
	var present []win32Battery
	if err := wmi.Query("SELECT Name FROM Win32_Battery", &present); err != nil {
		return err
	}
	if len(present) == 0 {
		s.Battery = &Battery{Present: false} // обычный ПК — батареи нет, это нормально
		return nil
	}

	var static []batteryStaticData
	if err := wmi.QueryNamespace("SELECT DesignedCapacity FROM BatteryStaticData", &static, `root\wmi`); err != nil {
		return err
	}
	var full []batteryFullCharged
	if err := wmi.QueryNamespace("SELECT FullChargedCapacity FROM BatteryFullChargedCapacity", &full, `root\wmi`); err != nil {
		return err
	}

	// Батарей бывает несколько (у некоторых ноутбуков две) — складываем.
	b := Battery{Present: true, Unit: "mWh"}
	for _, x := range static {
		b.Designed += int64(x.DesignedCapacity)
	}
	for _, x := range full {
		b.FullCharge += int64(x.FullChargedCapacity)
	}
	if b.Designed == 0 {
		return errors.New("батарея не сообщила свою заводскую ёмкость")
	}
	b.HealthPct = float64(b.FullCharge) / float64(b.Designed) * 100
	s.Battery = &b
	return nil
}

package collect

import "time"

// Здесь — «анкеты» для дисков, батареи и ошибок железа.
//
// Почему у некоторых полей тип со звёздочкой (*int64, *float64)? Это указатель — «ссылка на число».
// У указателя есть особое значение nil — «ничего». Так мы отличаем «0 плохих секторов» (всё хорошо)
// от «диск не сообщил число секторов» (неизвестно). Обычный int64 так не умеет: у него всегда есть число.

// Disks — всё про физические диски.
type Disks struct {
	SmartctlFound bool   `json:"smartctl_found"` // нашли ли программу smartctl
	SmartctlPath  string `json:"smartctl_path,omitempty"`
	List          []Disk `json:"list"`
}

// Disk — один физический диск (не раздел C:, а «железка» целиком).
type Disk struct {
	Device       string  `json:"device"` // как его назвал smartctl: /dev/sda, /dev/nvme0
	Model        string  `json:"model"`
	Serial       string  `json:"serial"`
	Kind         string  `json:"kind"` // HDD, SSD или NVMe
	CapacityGB   float64 `json:"capacity_gb"`
	SmartPassed  *bool   `json:"smart_passed"` // общий вердикт SMART: true — PASSED, false — FAILED
	PowerOnHours *int64  `json:"power_on_hours"`

	// Для HDD (и некоторых SATA SSD) — счётчики плохих секторов
	Reallocated   *int64 `json:"reallocated,omitempty"`   // атрибут 5: сектора, заменённые запасными
	Pending       *int64 `json:"pending,omitempty"`       // атрибут 197: сектора «под подозрением»
	Uncorrectable *int64 `json:"uncorrectable,omitempty"` // атрибут 198: сектора, которые не прочитать

	// Для SSD и NVMe — износ
	PercentUsed *int64   `json:"percent_used,omitempty"` // сколько % ресурса записи израсходовано
	WrittenTB   *float64 `json:"written_tb,omitempty"`   // сколько всего записано за жизнь, ТБ

	Error string `json:"error,omitempty"` // если про этот диск не удалось узнать
}

// Battery — батарея ноутбука. Present=false — батареи нет (обычный ПК).
type Battery struct {
	Present       bool    `json:"present"`
	DesignedMWh   int64   `json:"designed_mwh"`    // ёмкость новой батареи, мВт·ч
	FullChargeMWh int64   `json:"full_charge_mwh"` // сколько влезает сейчас при полной зарядке
	HealthPct     float64 `json:"health_pct"`      // FullCharge / Designed × 100
}

// HWErrors — аппаратные ошибки WHEA из журнала Windows.
type HWErrors struct {
	Days   int       `json:"days"`   // за сколько последних дней смотрели
	Count  int       `json:"count"`  // сколько всего
	Recent []HWEvent `json:"recent"` // несколько последних
}

// HWEvent — одна запись из журнала.
type HWEvent struct {
	Time    time.Time `json:"time"`
	ID      int       `json:"id"` // номер события: 17, 18, 19, 47…
	Message string    `json:"message"`
}

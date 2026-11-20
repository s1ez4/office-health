// Пакет rules решает, что «норм», что «скоро менять», а что «срочно».
// Здесь только логика — он ничего не собирает и ничего не рисует.
// Поэтому его легко проверять тестами и легко менять пороги.
package rules

import (
	"fmt"

	"office-health/collect"
)

// Пороги. Хотите строже или мягче — меняйте числа здесь.
const (
	SectorsSoon   = 0  // плохих секторов (5+197+198) больше этого → «скоро менять»
	SectorsUrgent = 50 // больше этого → «срочно»

	SSDWearSoon   = 80 // израсходовано ресурса SSD, % (и больше) → «скоро менять»
	SSDWearUrgent = 95 // → «срочно»

	BatterySoon   = 60 // здоровье батареи меньше, % → «скоро менять»
	BatteryUrgent = 40 // → «срочно»

	WHEASoon   = 1  // ошибок железа за 30 дней (и больше) → «скоро менять»
	WHEAUrgent = 10 // → «срочно»

	PanicSoon   = 1 // kernel panic на Mac за 30 дней → «скоро» (каждая — внезапная перезагрузка)
	PanicUrgent = 3 // → «срочно»

	VolumeFullSoon = 90 // раздел заполнен больше чем на, % → «скоро» (это не износ, но полезно знать)
)

// Level — уровень тревоги. Это просто число с именем типа:
// Unknown=0, OK=1, Soon=2, Urgent=3 (iota сам раздаёт номера по порядку).
// Чем больше число, тем хуже — поэтому «общий статус ПК» = самый большой уровень из всех.
type Level int

const (
	Unknown Level = iota
	OK
	Soon
	Urgent
)

// Text — подпись уровня по-русски.
func (l Level) Text() string {
	switch l {
	case OK:
		return "Норм"
	case Soon:
		return "Скоро менять"
	case Urgent:
		return "Срочно"
	}
	return "Не оценено"
}

// Class — имя CSS-класса для цвета в отчёте.
func (l Level) Class() string {
	return [...]string{"unknown", "ok", "soon", "urgent"}[l]
}

// Finding — одна находка: что не так и что делать.
type Finding struct {
	Level  Level
	Title  string // коротко: «Плохие сектора: 160»
	Advice string // по-человечески: что это значит и что делать
}

// Check — итог проверки одной вещи (диска, батареи…).
type Check struct {
	Level    Level
	Findings []Finding
}

// add добавляет находку и поднимает уровень проверки, если находка серьёзнее.
// (c *Check) — «указатель на получателя»: метод меняет саму проверку, а не её копию.
func (c *Check) add(l Level, title, advice string) {
	c.Findings = append(c.Findings, Finding{Level: l, Title: title, Advice: advice})
	c.Level = max(c.Level, l) // max — встроенная функция Go
}

// Result — итог по всему компьютеру. Disks и Volumes идут в том же порядке, что в снимке.
type Result struct {
	Overall    Level
	Incomplete bool // что-то важное проверить не удалось — «Норм» тогда с оговоркой
	Disks      []Check
	Volumes    []Check
	Battery    Check
	HWErrors   Check
}

// Evaluate проверяет весь снимок.
func Evaluate(s collect.Snapshot) Result {
	r := Result{}
	for _, d := range s.Disks.List {
		r.Disks = append(r.Disks, Disk(d))
	}
	for _, v := range s.System.Volumes {
		r.Volumes = append(r.Volumes, Volume(v))
	}
	if s.Battery != nil && s.Battery.Present {
		r.Battery = Battery(*s.Battery)
	}
	if s.HWErrors != nil {
		r.HWErrors = HWErrors(*s.HWErrors)
	}

	// Общий статус — худший из всех
	all := append(append([]Check{r.Battery, r.HWErrors}, r.Disks...), r.Volumes...)
	for _, c := range all {
		r.Overall = max(r.Overall, c.Level)
	}
	// Диски — главное. Если их не проверили (или проверили только поверхностно, через diskutil), «Норм» говорить рано.
	r.Incomplete = !s.Disks.SmartctlFound || len(s.Disks.List) == 0
	for _, c := range r.Disks {
		if c.Level == Unknown {
			r.Incomplete = true
		}
	}
	return r
}

// Disk — проверка одного физического диска.
func Disk(d collect.Disk) Check {
	if d.Error != "" {
		return Check{Level: Unknown}
	}
	c := Check{Level: OK}

	if d.SmartPassed != nil && !*d.SmartPassed {
		c.add(Urgent, "SMART: FAILED — диск сам сообщает о неисправности",
			"Встроенная самодиагностика диска считает, что он скоро откажет. "+
				"Сегодня же скопируйте важные файлы на другой диск или в облако и замените этот диск.")
	}

	// Плохие сектора: складываем три счётчика (nil — «неизвестно» — считаем как 0)
	var bad int64
	for _, p := range []*int64{d.Reallocated, d.Pending, d.Uncorrectable} {
		if p != nil {
			bad += *p
		}
	}
	detail := fmt.Sprintf("переназначено %d, под подозрением %d, нечитаемых %d",
		val(d.Reallocated), val(d.Pending), val(d.Uncorrectable))
	switch {
	case bad > SectorsUrgent:
		c.add(Urgent, fmt.Sprintf("Плохих секторов: %d — диск разрушается", bad),
			"Поверхность диска портится, и число плохих мест растёт ("+detail+"). "+
				"Файлы могут перестать открываться в любой момент. Срочно сделайте копию важных данных и замените диск.")
	case bad > SectorsSoon:
		c.add(Soon, fmt.Sprintf("Плохих секторов: %d", bad),
			"На диске появились места, которые плохо читаются ("+detail+"). "+
				"Пока он работает, но это первый признак износа. Держите свежую копию важных файлов "+
				"и запланируйте замену диска в ближайшие месяцы. Если число будет расти — меняйте сразу.")
	}

	if d.PercentUsed != nil {
		used := *d.PercentUsed
		switch {
		case used >= SSDWearUrgent:
			c.add(Urgent, fmt.Sprintf("SSD изношен на %d%%", used),
				"Ресурс записи почти исчерпан. Изношенный SSD может внезапно перестать записывать или "+
					"«замёрзнуть» только на чтение. Сделайте копию данных и замените диск как можно скорее.")
		case used >= SSDWearSoon:
			c.add(Soon, fmt.Sprintf("SSD изношен на %d%%", used),
				"У SSD ограниченное число перезаписей, и большая часть уже израсходована. "+
					"Работать он ещё будет, но заложите замену в бюджет на ближайшие полгода.")
		}
	}
	return c
}

// Volume — проверка заполненности раздела (C:, D:…).
func Volume(v collect.Volume) Check {
	c := Check{Level: OK}
	if v.UsedPct > VolumeFullSoon {
		c.add(Soon, fmt.Sprintf("Раздел %s почти заполнен (свободно %.0f ГБ)", v.Mount, v.FreeGB),
			"Это не износ, но когда место кончается, Windows тормозит и не может ставить обновления. "+
				"Почистите «Загрузки» и корзину, запустите «Очистку диска» (правой кнопкой по диску → Свойства) "+
				"или перенесите большие файлы на другой диск.")
	}
	return c
}

// Battery — проверка батареи ноутбука.
func Battery(b collect.Battery) Check {
	c := Check{Level: OK}
	switch h := b.HealthPct; {
	case h < BatteryUrgent:
		c.add(Urgent, fmt.Sprintf("Батарея держит %.0f%% от новой", h),
			"Ноутбук быстро разряжается и может выключиться без предупреждения, а старые батареи иногда вздуваются. "+
				"Замените батарею; если корпус выгнулся или тачпад приподнялся — не заряжайте и меняйте немедленно.")
	case h < BatterySoon:
		c.add(Soon, fmt.Sprintf("Батарея держит %.0f%% от новой", h),
			"Батарея постарела: от розетки ноутбук работает заметно меньше, чем новым. "+
				"Если его носят на встречи — пора заказать новую батарею. На столе от сети это не страшно.")
	}
	return c
}

// HWErrors — проверка журнала аппаратных ошибок.
func HWErrors(e collect.HWErrors) Check {
	if e.Source == collect.SourcePanic {
		return panics(e)
	}
	c := Check{Level: OK}
	switch n := e.Count; {
	case n >= WHEAUrgent:
		c.add(Urgent, fmt.Sprintf("Ошибок железа за %d дней: %d", e.Days, n),
			"Windows постоянно фиксирует аппаратные сбои — это часто бывает перед зависаниями и «синими экранами». "+
				"Покажите компьютер мастеру: проверить память (MemTest), перегрев, блок питания и материнскую плату.")
	case n >= WHEASoon:
		c.add(Soon, fmt.Sprintf("Ошибок железа за %d дней: %d", e.Days, n),
			"Windows записала аппаратные сбои, которые сама исправила. Единичные — не беда, "+
				"но если они повторяются, стоит проверить память и не перегревается ли компьютер (почистить от пыли).")
	}
	return c
}

// panics — то же для Mac: аварийные перезагрузки (kernel panic).
func panics(e collect.HWErrors) Check {
	c := Check{Level: OK}
	switch n := e.Count; {
	case n >= PanicUrgent:
		c.add(Urgent, fmt.Sprintf("Аварийных перезагрузок за %d дней: %d", e.Days, n),
			"Mac регулярно падает с kernel panic — так обычно проявляются неисправная память, диск, перегрев "+
				"или сбойное подключённое устройство. Сделайте резервную копию (Time Machine) и покажите Mac в сервисе; "+
				"перед этим попробуйте отключить все внешние устройства и посмотреть, повторится ли.")
	case n >= PanicSoon:
		c.add(Soon, fmt.Sprintf("Аварийных перезагрузок за %d дней: %d", e.Days, n),
			"Mac хотя бы раз перезагрузился из-за критической ошибки. Единичный случай бывает из-за программы или "+
				"переходника; если повторится — проверьте Mac в сервисе. Обновите macOS и держите свежую копию Time Machine.")
	}
	return c
}

func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

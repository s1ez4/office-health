package report

// ReportView — «представление»: всё, что нужно шаблону report.html, уже готовыми строками.
// Шаблон ничего не считает и не форматирует — только подставляет. Поэтому вся логика
// («01» с ведущим нулём, «41 230» с пробелом, подписи статусов) здесь, в Go, где её легко проверить тестом.

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"office-health/collect"
	"office-health/rules"
	"office-health/version"
)

type ReportView struct {
	Hostname, ScanTime, AgentVersion string
	Overall                          struct{ Level, Headline string }  // Level: "urgent" | "soon" | "ok"
	Counts                           struct{ Urgent, Soon, Ok string } // «01», «03», «02»
	Specs                            []string                          // «Windows 11 Pro», «RAM 8 ГБ»…
	Actions                          []Action                          // от срочных к менее срочным
	Disks                            []Disk
	DiskHint                         string   // подсказка под дисками: как поставить smartctl
	Battery                          *Battery // nil, если батареи нет
	HWErrors                         struct {
		Level, Label, Count string
		Caption             string // «журнал whea · 30 дней» или «отчёты о сбоях · 30 дней» (Mac)
		Items               []ErrItem
	}
	Sources string // откуда данные (подвал): «smartctl / wmi / журнал windows»
}

// Action — одно дело «что сделать». Label: «▲ срочно» / «◆ скоро» / «● норма»; Short — 2–4 слова для окна сводки.
type Action struct{ Level, Label, Short, Title, Detail string }

type Disk struct {
	Letter, Kind, Model, Level, Label   string
	IsSSD                               bool
	WearPct, UsedPct                    int
	Hours, WrittenTB                    string
	HasTrend                            bool
	TrendPoints                         string // точки SVG 300×70; тренды — следующий этап, пока HasTrend=false
	Reallocated, Pending, Uncorrectable string
	ReallocatedBad                      bool
}

type Battery struct {
	Level, Label       string
	HealthPct          int
	DesignMWh, FullMWh string
	Segments           []bool // 10 делений: true — закрашено
	// На Mac вместо мВт·ч показываем циклы зарядки: Cycles непустой — шаблон рисует их
	Cycles, DesignCycles string
}

type ErrItem struct{ Text, Time string }

// level переводит наш уровень в имя для CSS-класса шаблона. «Не оценено» шаблон не знает — красим как норму,
// а подпись (label) честно скажет «нет данных».
func level(l rules.Level) string {
	switch l {
	case rules.Urgent:
		return "urgent"
	case rules.Soon:
		return "soon"
	}
	return "ok"
}

func label(l rules.Level) string {
	switch l {
	case rules.Urgent:
		return "▲ срочно"
	case rules.Soon:
		return "◆ скоро"
	case rules.OK:
		return "● норма"
	}
	return "○ нет данных"
}

// thousands — «41230» → «41 230». Между разрядами неразрывный пробел: число не разорвётся на две строки.
func thousands(n int64) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func num(p *int64) string {
	if p == nil {
		return "—"
	}
	return thousands(*p)
}

func two(n int) string { return fmt.Sprintf("%02d", n) } // 1 → «01»

// BuildView собирает ReportView из снимка и результатов правил.
func BuildView(s collect.Snapshot, r rules.Result) ReportView {
	v := ReportView{
		Hostname:     s.System.Hostname,
		ScanTime:     s.CollectedAt.Format("02.01.2006 15:04"),
		AgentVersion: version.Version,
		Specs:        specs(s),
	}
	mac := s.Platform == "darwin"
	switch {
	case mac:
		v.Sources = "smartctl / ioreg / отчёты о сбоях macos"
	case s.Platform == "linux":
		v.Sources = "smartctl"
	default:
		v.Sources = "smartctl / wmi / журнал windows"
	}
	if !s.Disks.SmartctlFound {
		if mac {
			v.DiskHint = "Подробная проверка дисков (износ SSD) недоступна: нет программы smartctl. " +
				"Поставьте её в Терминале: brew install smartmontools — и запустите проверку ещё раз."
		} else {
			v.DiskHint = "Здоровье дисков не проверено: нет программы smartctl. Поставьте её: " +
				"winget install smartmontools.smartmontools — или положите smartctl.exe рядом с программой."
		}
	}

	// Общий статус и заголовок
	v.Overall.Level = level(r.Overall)
	switch {
	case r.Overall == rules.Urgent:
		v.Overall.Headline = "нужна срочная замена"
	case r.Overall == rules.Soon:
		v.Overall.Headline = "скоро понадобится замена"
	case r.Overall == rules.Unknown:
		v.Overall.Headline = "проверить не удалось"
	case r.Incomplete:
		v.Overall.Headline = "норма, но не всё проверено"
	default:
		v.Overall.Headline = "всё в норме"
	}

	// Сколько проверенных вещей (дисков, разделов, батарея, журнал) в каждом статусе
	checks := append(append([]rules.Check{}, r.Disks...), r.Volumes...)
	if s.Battery != nil && s.Battery.Present {
		checks = append(checks, r.Battery)
	}
	if s.HWErrors != nil {
		checks = append(checks, r.HWErrors)
	}
	counts := map[rules.Level]int{}
	for _, c := range checks {
		counts[c.Level]++
	}
	v.Counts.Urgent, v.Counts.Soon, v.Counts.Ok = two(counts[rules.Urgent]), two(counts[rules.Soon]), two(counts[rules.OK])

	// Диски
	for i, d := range s.Disks.List {
		v.Disks = append(v.Disks, diskView(i, d, r.Disks[i], s))
	}

	// Что сделать: все находки, срочные первыми. У дисков в заголовке — какой именно диск.
	type found struct {
		f    rules.Finding
		subj string
	}
	var all []found
	for i, c := range r.Disks {
		for _, f := range c.Findings {
			all = append(all, found{f, v.Disks[i].Letter})
		}
	}
	for _, c := range append(append([]rules.Check{}, r.Volumes...), r.Battery, r.HWErrors) {
		for _, f := range c.Findings {
			all = append(all, found{f, ""})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].f.Level > all[j].f.Level })
	for _, a := range all {
		title := a.f.Title
		if a.subj != "" {
			title += " (" + a.subj + ")"
		}
		v.Actions = append(v.Actions, Action{
			Level: level(a.f.Level), Label: label(a.f.Level), Short: a.f.Short, Title: title, Detail: a.f.Advice,
		})
	}

	// Батарея
	if b := s.Battery; b != nil && b.Present {
		health := int(math.Round(b.HealthPct))
		filled := min(max(int(math.Round(b.HealthPct/10)), 0), 10)
		bv := &Battery{Level: level(r.Battery.Level), Label: label(r.Battery.Level), HealthPct: health,
			DesignMWh: thousands(b.Designed), FullMWh: thousands(b.FullCharge)}
		if b.Unit == "mAh" { // Mac: ёмкость в мА·ч в карточке не нужна — важнее циклы
			bv.Cycles = num(b.Cycles)
			if b.DesignCycles != nil {
				bv.DesignCycles = thousands(*b.DesignCycles)
			}
		}
		for i := 0; i < 10; i++ {
			bv.Segments = append(bv.Segments, i < filled)
		}
		v.Battery = bv
	}

	// Ошибки железа
	v.HWErrors.Caption = "журнал whea · 30 дней"
	if mac {
		v.HWErrors.Caption = "отчёты о сбоях · 30 дней"
	}
	if e := s.HWErrors; e != nil {
		src := "журнал whea"
		if e.Source == collect.SourcePanic {
			src = "отчёты о сбоях"
		}
		v.HWErrors.Caption = fmt.Sprintf("%s · %d дней", src, e.Days)
		v.HWErrors.Level, v.HWErrors.Label, v.HWErrors.Count = level(r.HWErrors.Level), label(r.HWErrors.Level), two(e.Count)
		for _, it := range e.Recent {
			text := it.Message
			if it.ID != 0 {
				text = fmt.Sprintf("Событие %d · %s", it.ID, it.Message)
			}
			v.HWErrors.Items = append(v.HWErrors.Items, ErrItem{Text: text, Time: it.Time.Format("02.01 15:04")})
		}
	} else {
		v.HWErrors.Level, v.HWErrors.Label, v.HWErrors.Count = "ok", label(rules.Unknown), "—"
	}
	return v
}

func diskView(i int, d collect.Disk, c rules.Check, s collect.Snapshot) Disk {
	dv := Disk{
		Letter: strings.Join(d.Volumes, " "), Kind: d.Kind, Model: d.Model,
		Level: level(c.Level), Label: label(c.Level),
		Hours: num(d.PowerOnHours), WrittenTB: "—",
		Reallocated: num(d.Reallocated), Pending: num(d.Pending), Uncorrectable: num(d.Uncorrectable),
		ReallocatedBad: d.Reallocated != nil && *d.Reallocated > 0,
	}
	if dv.Letter == "" {
		dv.Letter = fmt.Sprintf("Диск %d", i+1)
	}
	if dv.Model == "" {
		dv.Model = "Неизвестный диск"
	}
	if d.Error != "" {
		dv.Model += " — " + d.Error
	}
	// Показ «как SSD» (большой процент износа) — только если износ известен; иначе таблица с прочерками.
	if (d.Kind == "SSD" || d.Kind == "NVMe") && d.PercentUsed != nil {
		dv.IsSSD = true
		dv.WearPct = min(max(int(*d.PercentUsed), 0), 100)
	}
	if d.WrittenTB != nil {
		dv.WrittenTB = comma(fmt.Sprintf("%.1f", *d.WrittenTB))
	}
	// Занято: по разделам этого диска. Диск один, а буквы не нашлись — значит, все разделы его.
	vols := d.Volumes
	if len(vols) == 0 && len(s.Disks.List) == 1 {
		for _, vol := range s.System.Volumes {
			vols = append(vols, vol.Mount)
		}
	}
	var used, total float64
	for _, vol := range s.System.Volumes {
		for _, m := range vols {
			if vol.Mount == m {
				used += vol.TotalGB - vol.FreeGB
				total += vol.TotalGB
			}
		}
	}
	if total > 0 {
		dv.UsedPct = int(math.Round(used / total * 100))
	}
	return dv
}

// specs — короткие характеристики компьютера для окна сводки.
func specs(s collect.Snapshot) []string {
	clean := strings.NewReplacer("Microsoft ", "", "(R)", "", "(TM)", "", "  ", " ")
	var out []string
	if s.System.Model != "" {
		out = append(out, s.System.Model)
	}
	out = append(out, strings.TrimSpace(clean.Replace(s.System.OS)))
	if s.System.CPU != "" {
		out = append(out, strings.TrimSpace(clean.Replace(s.System.CPU)))
	}
	if s.System.RAMTotalGB > 0 {
		out = append(out, fmt.Sprintf("RAM %.0f ГБ", s.System.RAMTotalGB))
	}
	if s.System.UptimeHours > 0 {
		if s.System.UptimeHours < 48 {
			out = append(out, fmt.Sprintf("Без перезагрузки %.0f ч", s.System.UptimeHours))
		} else {
			out = append(out, fmt.Sprintf("Без перезагрузки %.0f дн.", s.System.UptimeHours/24))
		}
	}
	if s.Demo {
		out = append(out, "Демо-данные")
	}
	return out
}

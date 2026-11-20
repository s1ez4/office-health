// Пакет report превращает снимок и оценки в HTML-отчёт и сохраняет снимок в JSON.
package report

import (
	_ "embed" // подчёркивание: пакет нужен только ради директивы //go:embed ниже
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"office-health/collect"
	"office-health/rules"
)

// Директива //go:embed вшивает файл template.html прямо внутрь .exe при сборке.
// Поэтому на офисный компьютер нужно копировать только один файл — программу.
//
//go:embed template.html
var templateHTML string

// page — всё, что видит шаблон. Диски склеены со своими оценками, чтобы в шаблоне было проще.
type page struct {
	S         collect.Snapshot
	R         rules.Result
	Disks     []diskRow
	Volumes   []volumeRow
	Generated string
}

type diskRow struct {
	D collect.Disk
	C rules.Check
}

type volumeRow struct {
	V collect.Volume
	C rules.Check
}

// funcs — маленькие функции, которые можно вызывать прямо из шаблона: {{f1 .RAMTotalGB}}.
var funcs = template.FuncMap{
	"f0":  func(v float64) string { return fmt.Sprintf("%.0f", v) },
	"f1":  func(v float64) string { return comma(fmt.Sprintf("%.1f", v)) },
	"f1p": func(p *float64) string { return comma(fmt.Sprintf("%.1f", *p)) },
	"n": func(p *int64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprint(*p)
	},
	"hours": func(p *int64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%d ч (≈ %s лет)", *p, comma(fmt.Sprintf("%.1f", float64(*p)/24/365)))
	},
	"smart": func(p *bool) string {
		switch {
		case p == nil:
			return "неизвестно"
		case *p:
			return "PASSED (в порядке)"
		}
		return "FAILED (диск сам сообщает о неисправности)"
	},
	"uptime": func(h float64) string {
		if h < 48 {
			return fmt.Sprintf("%.0f ч", h)
		}
		return fmt.Sprintf("%.0f дн.", h/24)
	},
	"date": func(t time.Time) string { return t.Format("02.01.2006 15:04") },
	"wh":   func(mwh int64) string { return comma(fmt.Sprintf("%.1f", float64(mwh)/1000)) }, // мВт·ч → Вт·ч
}

// comma — в России дробную часть отделяют запятой: 7,9 а не 7.9.
func comma(s string) string { return strings.Replace(s, ".", ",", 1) }

// Write сохраняет отчёт и снимок в папку dir. Возвращает путь к HTML-файлу.
func Write(dir string, s collect.Snapshot, r rules.Result) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Имя файла: office-health_BUH-PC-03_2026-11-20_1830
	base := fmt.Sprintf("office-health_%s_%s", s.System.Hostname, s.CollectedAt.Format("2006-01-02_1504"))
	htmlPath := filepath.Join(dir, base+".html")

	p := page{S: s, R: r, Generated: s.CollectedAt.Format("02.01.2006 в 15:04")}
	for i, d := range s.Disks.List {
		p.Disks = append(p.Disks, diskRow{D: d, C: r.Disks[i]})
	}
	for i, v := range s.System.Volumes {
		p.Volumes = append(p.Volumes, volumeRow{V: v, C: r.Volumes[i]})
	}

	// template.Must — «если шаблон с ошибкой, сразу остановись»: это ошибка программиста, а не пользователя.
	tmpl := template.Must(template.New("report").Funcs(funcs).Parse(templateHTML))
	f, err := os.Create(htmlPath)
	if err != nil {
		return "", err
	}
	defer f.Close() // defer — «закрой файл, когда функция закончится», что бы ни случилось
	if err := tmpl.Execute(f, p); err != nil {
		return "", err
	}

	// Снимок в JSON — пригодится на этапе 2 (история и тренды).
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, base+".json"), data, 0o644); err != nil {
		return "", err
	}
	return htmlPath, nil
}

// Пакет report превращает снимок и оценки в HTML-отчёт и сохраняет снимок в JSON.
// Один и тот же шаблон report.html рисует и отчёт-файл, и страницу компьютера в приложении.
package report

import (
	_ "embed" // подчёркивание: пакет нужен только ради директивы //go:embed ниже
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"office-health/collect"
	"office-health/rules"
)

// Директива //go:embed вшивает файл report.html прямо внутрь программы при сборке.
// Поэтому на компьютер нужно копировать только один файл — программу.
// Вёрстка в нём готовая, данные приходят в виде ReportView (view.go).
//
//go:embed report.html
var reportHTML string

// template.Must — «если шаблон с ошибкой, сразу остановись»: это ошибка программиста, а не пользователя.
var reportTmpl = template.Must(template.New("report.html").Parse(reportHTML))

// Render рисует отчёт в w (в файл или в ответ браузеру).
func Render(w io.Writer, v ReportView) error { return reportTmpl.Execute(w, v) }

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

	f, err := os.Create(htmlPath)
	if err != nil {
		return "", err
	}
	defer f.Close() // defer — «закрой файл, когда функция закончится», что бы ни случилось
	if err := Render(f, BuildView(s, r)); err != nil {
		return "", err
	}

	// Снимок в JSON — для истории и трендов.
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, base+".json"), data, 0o644); err != nil {
		return "", err
	}
	return htmlPath, nil
}

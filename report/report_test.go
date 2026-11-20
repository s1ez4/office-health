package report

import (
	"os"
	"strings"
	"testing"

	"office-health/collect"
	"office-health/rules"
)

func TestDemoReport(t *testing.T) {
	dir := t.TempDir() // временная папка, Go сам удалит её после теста
	s := collect.Demo()
	path, err := Write(dir, s, rules.Evaluate(s))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"BUH-PC-03", "Срочно", "Скоро менять", "Плохих секторов: 160", "55%", "Демо-режим"} {
		if !strings.Contains(html, want) {
			t.Errorf("в отчёте нет %q", want)
		}
	}
	// «<no value>» шаблон пишет, когда обратились к несуществующему полю
	if strings.Contains(html, "<no value>") || strings.Contains(html, "http") {
		t.Error("в отчёте пустое поле или внешняя ссылка")
	}
	if _, err := os.Stat(strings.TrimSuffix(path, ".html") + ".json"); err != nil {
		t.Error("рядом нет JSON-снимка")
	}
}

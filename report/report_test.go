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
	for _, want := range []string{
		"BUH-PC-03", "( нужна срочная замена )", "status-urgent",
		"WDC WD10EZEX-08WN4A0", "41 230", `class="bad">128`, // HDD: часы с пробелом, сектора красным
		"Samsung SSD 980 500GB", "18,4 ТБ", "width:12%", // SSD: износ, записано, полоска
		"55%", "50 000", // батарея
		"Заменить HDD", "Заказать батарею", // короткие дела в сводке
		"v0.2.0",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("в отчёте нет %q", want)
		}
	}
	// «<no value>» шаблон пишет, когда обратились к несуществующему полю; ZgotmplZ — когда значение
	// показалось ему опасным в этом месте (например, в стиле) и он его вырезал.
	for _, bad := range []string{"<no value>", "ZgotmplZ"} {
		if strings.Contains(html, bad) {
			t.Errorf("в отчёте %q", bad)
		}
	}
	if _, err := os.Stat(strings.TrimSuffix(path, ".html") + ".json"); err != nil {
		t.Error("рядом нет JSON-снимка")
	}
}

func TestBuildView(t *testing.T) {
	s := collect.Demo()
	v := BuildView(s, rules.Evaluate(s))
	// Демо: HDD срочно; батарея, WHEA и C: — скоро; NVMe и D: — норма
	if v.Counts.Urgent != "01" || v.Counts.Soon != "03" || v.Counts.Ok != "02" {
		t.Errorf("счётчики %+v, ждали 01/03/02", v.Counts)
	}
	if v.Overall.Level != "urgent" || v.Actions[0].Level != "urgent" || v.Actions[len(v.Actions)-1].Level != "soon" {
		t.Errorf("порядок дел: %+v", v.Actions)
	}
	if !strings.Contains(v.Actions[0].Title, "(D:)") {
		t.Errorf("у дела по диску нет буквы: %q", v.Actions[0].Title)
	}
	ssd, hdd := v.Disks[0], v.Disks[1]
	if !ssd.IsSSD || ssd.WearPct != 12 || ssd.UsedPct != 93 || ssd.Letter != "C:" || ssd.Hours != "6 120" {
		t.Errorf("SSD: %+v", ssd)
	}
	if hdd.IsSSD || !hdd.ReallocatedBad || hdd.Reallocated != "128" || hdd.Label != "▲ срочно" {
		t.Errorf("HDD: %+v", hdd)
	}
	segs := 0
	for _, on := range v.Battery.Segments {
		if on {
			segs++
		}
	}
	if v.Battery.HealthPct != 55 || len(v.Battery.Segments) != 10 || segs != 6 { // 55% → 5,5 → округляем до 6
		t.Errorf("батарея: %+v (закрашено %d)", v.Battery, segs)
	}
	if v.HWErrors.Count != "03" || len(v.HWErrors.Items) != 3 {
		t.Errorf("ошибки: %+v", v.HWErrors)
	}

	// Нет батареи — nil, блок в шаблоне не рисуется
	s.Battery = &collect.Battery{Present: false}
	if BuildView(s, rules.Evaluate(s)).Battery != nil {
		t.Error("у ПК без батареи появилась батарея")
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1 000", 41230: "41 230", 1234567: "1 234 567", -5000: "-5 000"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, ждали %q", n, got, want)
		}
	}
}

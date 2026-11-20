package rules

import (
	"testing"

	"office-health/collect"
)

// Демо-компьютер: здоровый NVMe, умирающий HDD, батарея 55%, 3 ошибки WHEA, C: заполнен на 93%.
func TestDemo(t *testing.T) {
	r := Evaluate(collect.Demo())
	checks := []struct { // «таблица» проверок — так в Go принято писать похожие тесты
		name      string
		got, want Level
	}{
		{"NVMe", r.Disks[0].Level, OK},
		{"HDD", r.Disks[1].Level, Urgent},
		{"батарея", r.Battery.Level, Soon},
		{"WHEA", r.HWErrors.Level, Soon},
		{"C:", r.Volumes[0].Level, Soon},
		{"D:", r.Volumes[1].Level, OK},
		{"общий", r.Overall, Urgent},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: %s, ждали %s", c.name, c.got.Text(), c.want.Text())
		}
	}
	if r.Incomplete {
		t.Error("в демо всё проверено — оговорки не нужно")
	}
}

// Границы порогов: ровно на пороге и рядом.
func TestThresholds(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	f := false

	cases := []struct {
		name string
		got  Level
		want Level
	}{
		{"0 секторов", Disk(collect.Disk{Reallocated: n(0)}).Level, OK},
		{"1 сектор", Disk(collect.Disk{Pending: n(1)}).Level, Soon},
		{"50 секторов", Disk(collect.Disk{Reallocated: n(50)}).Level, Soon},
		{"51 сектор (сумма)", Disk(collect.Disk{Reallocated: n(40), Uncorrectable: n(11)}).Level, Urgent},
		{"SMART FAILED", Disk(collect.Disk{SmartPassed: &f}).Level, Urgent},
		{"SSD 79%", Disk(collect.Disk{PercentUsed: n(79)}).Level, OK},
		{"SSD 80%", Disk(collect.Disk{PercentUsed: n(80)}).Level, Soon},
		{"SSD 95%", Disk(collect.Disk{PercentUsed: n(95)}).Level, Urgent},
		{"диск не прочитан", Disk(collect.Disk{Error: "нет доступа"}).Level, Unknown},
		{"батарея 60%", Battery(collect.Battery{HealthPct: 60}).Level, OK},
		{"батарея 59%", Battery(collect.Battery{HealthPct: 59}).Level, Soon},
		{"батарея 39%", Battery(collect.Battery{HealthPct: 39}).Level, Urgent},
		{"0 WHEA", HWErrors(collect.HWErrors{Count: 0}).Level, OK},
		{"1 WHEA", HWErrors(collect.HWErrors{Count: 1}).Level, Soon},
		{"10 WHEA", HWErrors(collect.HWErrors{Count: 10}).Level, Urgent},
		{"0 паник", HWErrors(collect.HWErrors{Source: collect.SourcePanic, Count: 0}).Level, OK},
		{"1 паника", HWErrors(collect.HWErrors{Source: collect.SourcePanic, Count: 1}).Level, Soon},
		{"3 паники", HWErrors(collect.HWErrors{Source: collect.SourcePanic, Count: 3}).Level, Urgent},
		{"раздел 90%", Volume(collect.Volume{UsedPct: 90}).Level, OK},
		{"раздел 90,5%", Volume(collect.Volume{UsedPct: 90.5}).Level, Soon},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %s, ждали %s", c.name, c.got.Text(), c.want.Text())
		}
	}
}

// Диски не проверены (нет прав) — общий статус «Норм», но с оговоркой.
func TestIncomplete(t *testing.T) {
	s := collect.Snapshot{Disks: collect.Disks{SmartctlFound: true, List: []collect.Disk{{Error: "нет доступа"}}}}
	if r := Evaluate(s); !r.Incomplete {
		t.Error("ждали оговорку «не всё проверено»")
	}
}

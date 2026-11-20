package collect

// Тесты. Файлы *_test.go не попадают в программу — их запускает команда `go test ./...`.
// Каждая функция TestXxx(t *testing.T) — одна проверка. t.Errorf сообщает о провале, но идёт дальше.
// Образцы ответов smartctl лежат в папке testdata (Go специально не считает её кодом).

import (
	"math"
	"os"
	"testing"
)

func load(t *testing.T, name string) Disk {
	t.Helper() // если упадёт — показывать строку в тесте, а не здесь
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err) // Fatal — дальше этот тест не выполнять
	}
	d, err := parseSmart(data)
	if err != nil {
		t.Fatalf("parseSmart(%s): %v", name, err)
	}
	return d
}

func TestNVMe(t *testing.T) {
	d := load(t, "nvme.json")
	if d.Kind != "NVMe" || *d.PercentUsed != 7 || *d.PowerOnHours != 5321 || !*d.SmartPassed {
		t.Errorf("неверно разобран NVMe: %+v", d)
	}
	// 39 062 500 единиц × 512 000 байт = 20 ТБ
	if math.Abs(*d.WrittenTB-20) > 0.01 {
		t.Errorf("записано %.2f ТБ, ждали 20", *d.WrittenTB)
	}
	if d.Reallocated != nil {
		t.Error("у NVMe нет атрибута 5 — должно быть nil, а не 0")
	}
}

func TestDyingHDD(t *testing.T) {
	d := load(t, "hdd_dying.json")
	if d.Kind != "HDD" || *d.SmartPassed {
		t.Errorf("ждали HDD с SMART FAILED: %+v", d)
	}
	if *d.Reallocated != 128 || *d.Pending != 24 || *d.Uncorrectable != 8 || *d.PowerOnHours != 41230 {
		t.Errorf("неверные атрибуты: 5=%d 197=%d 198=%d 9=%d",
			*d.Reallocated, *d.Pending, *d.Uncorrectable, *d.PowerOnHours)
	}
	if d.PercentUsed != nil {
		t.Error("у HDD нет износа в процентах")
	}
}

func TestSataSSD(t *testing.T) {
	d := load(t, "sata_ssd.json")
	if d.Kind != "SSD" || *d.Reallocated != 0 {
		t.Errorf("ждали SSD без плохих секторов: %+v", d)
	}
	if *d.PercentUsed != 17 { // SSD_Life_Left = 83% осталось → израсходовано 17%
		t.Errorf("износ %d%%, ждали 17%%", *d.PercentUsed)
	}
	if math.Abs(*d.WrittenTB-10) > 0.01 { // 19 531 250 000 × 512 байт = 10 ТБ
		t.Errorf("записано %.2f ТБ, ждали 10", *d.WrittenTB)
	}
}

func TestAccessDenied(t *testing.T) {
	data, _ := os.ReadFile("testdata/access_denied.json")
	_, err := parseSmart(data)
	if err == nil || err.Error() != "нет доступа — запустите программу от администратора" {
		t.Errorf("ждали понятную ошибку про администратора, получили: %v", err)
	}
}

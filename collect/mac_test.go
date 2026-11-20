package collect

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestIoregAppleSilicon(t *testing.T) {
	b := parseIoregBattery(readText(t, "ioreg_apple_silicon.txt"))
	// 3772 / 4382 = 86,1%. MaxCapacity=100 — это проценты, его брать нельзя.
	if !b.Present || b.Designed != 4382 || b.FullCharge != 3772 || b.Unit != "mAh" {
		t.Fatalf("батарея: %+v", b)
	}
	if math.Abs(b.HealthPct-86.08) > 0.1 || *b.Cycles != 287 {
		t.Errorf("здоровье %.2f%%, циклов %d", b.HealthPct, *b.Cycles)
	}
}

func TestIoregIntel(t *testing.T) {
	b := parseIoregBattery(readText(t, "ioreg_intel.txt"))
	// Нет AppleRawMaxCapacity — берём MaxCapacity в мА·ч: 2401 / 6669 = 36%
	if math.Abs(b.HealthPct-36.0) > 0.1 || *b.Cycles != 1104 {
		t.Errorf("Intel: %+v", b)
	}
}

func TestIoregNoBattery(t *testing.T) {
	if b := parseIoregBattery(""); b.Present {
		t.Error("Mac mini без батареи, а нашли батарею")
	}
}

func TestDiskutil(t *testing.T) {
	ids := parseDiskutilList(readText(t, "diskutil_list.txt"))
	if len(ids) != 2 || ids[0] != "disk0" || ids[1] != "disk4" {
		t.Errorf("диски: %v", ids)
	}
	d := parseDiskutilInfo("disk0", readText(t, "diskutil_info.txt"))
	if d.Model != "APPLE SSD AP0512Q" || d.Kind != "SSD" || d.SmartPassed == nil || !*d.SmartPassed {
		t.Errorf("diskutil info: %+v", d)
	}
	if math.Abs(d.CapacityGB-465.9) > 0.1 {
		t.Errorf("размер %.1f ГБ", d.CapacityGB)
	}
}

func TestPanics(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name, body string, age time.Duration) {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o644)
		os.Chtimes(p, now.Add(-age), now.Add(-age)) // «состарить» файл
	}
	write("panic-full-2026-11-18-101010.panic", `{"bug_type":"210"}`+"\n"+`{"panicString":"panic(cpu 2 caller 0xfffffe0012): watchdog timeout\nmore"}`, 48*time.Hour)
	write("Kernel-2026-11-01-090000.panic", "старый формат", 19*24*time.Hour)
	write("Kernel-2026-09-01-090000.panic", "слишком старый", 80*24*time.Hour)
	write("Safari-2026-11-19.ips", "не паника", time.Hour)

	res := collectPanics([]string{dir, filepath.Join(dir, "нет-такой-папки")}, now, 30)
	if res.Source != SourcePanic || res.Count != 2 {
		t.Fatalf("паник %d, ждали 2: %+v", res.Count, res)
	}
	if res.Recent[0].Message != "Kernel panic: panic(cpu 2 caller 0xfffffe0012): watchdog timeout" {
		t.Errorf("причина: %q", res.Recent[0].Message)
	}
	if res.Recent[1].Message != "Kernel panic: Kernel-2026-11-01-090000.panic" {
		t.Errorf("без причины: %q", res.Recent[1].Message)
	}
}

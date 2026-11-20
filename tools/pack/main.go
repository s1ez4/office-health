// pack — собирает готовые к раздаче архивы в папку dist:
//
//	dist/OfficeHealth-mac.zip      — «Office Health.app» (одна программа для Mac на M1…M4 и на Intel)
//	dist/OfficeHealth-windows.zip  — office-health.exe + README
//
// Запуск из папки проекта: go run ./tools/pack
//
// Работает на любой ОС, в том числе на Windows: Apple-утилиты (lipo, iconutil) не нужны —
// «универсальную» программу и иконку собираем сами, форматы там простые.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	appName  = "Office Health"
	bundleID = "local.office-health"
	version  = "0.2.0"
)

func main() {
	if err := run(); err != nil {
		fmt.Println("Ошибка:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "oh-pack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// 1. Собираем программу под три цели
	builds := map[string][2]string{
		"darwin-arm64": {"darwin", "arm64"}, // Mac на Apple M1, M2, M3, M4
		"darwin-amd64": {"darwin", "amd64"}, // Mac на Intel
		"windows":      {"windows", "amd64"},
	}
	for name, t := range builds {
		out := filepath.Join(tmp, name)
		fmt.Println("Сборка", name, "…")
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, ".")
		cmd.Env = append(os.Environ(), "GOOS="+t[0], "GOARCH="+t[1], "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	// 2. Mac: склеиваем две программы в одну «универсальную»
	arm, _ := os.ReadFile(filepath.Join(tmp, "darwin-arm64"))
	amd, _ := os.ReadFile(filepath.Join(tmp, "darwin-amd64"))
	universal := fatMachO(amd, arm)

	// 3. Mac: собираем Office Health.app и кладём в zip
	icon := icns(drawIcon(1024), drawIcon(256))
	app := appName + ".app/Contents/"
	macFiles := []zipFile{
		{app + "Info.plist", []byte(infoPlist), 0o644},
		{app + "MacOS/office-health", universal, 0o755}, // 0755 — «можно запускать», иначе Mac не откроет
		{app + "Resources/AppIcon.icns", icon, 0o644},
		{"Как открыть.txt", []byte(macReadme), 0o644},
	}
	if err := writeZip("dist/OfficeHealth-mac.zip", macFiles); err != nil {
		return err
	}

	// 4. Windows
	exe, _ := os.ReadFile(filepath.Join(tmp, "windows"))
	readme, _ := os.ReadFile("README.md")
	winFiles := []zipFile{
		{"office-health/office-health.exe", exe, 0o755},
		{"office-health/README.md", readme, 0o644},
	}
	if err := writeZip("dist/OfficeHealth-windows.zip", winFiles); err != nil {
		return err
	}
	if err := os.WriteFile("dist/office-health.exe", exe, 0o755); err != nil {
		return err
	}
	for _, f := range []string{"dist/OfficeHealth-mac.zip", "dist/OfficeHealth-windows.zip"} {
		info, _ := os.Stat(f)
		fmt.Printf("Готово: %s (%.1f МБ)\n", f, float64(info.Size())/1024/1024)
	}
	return nil
}

// fatMachO склеивает программы для разных процессоров в один «толстый» (fat) файл Mach-O.
// Устроен он просто: заголовок со списком «вот кусок для Intel, вот для ARM», дальше сами куски.
// Mac при запуске сам берёт кусок под свой процессор. Все числа в заголовке — big-endian.
func fatMachO(amd64, arm64 []byte) []byte {
	const align = 1 << 14 // куски кладём с шагом 16 КБ, как делает Apple
	type slice struct {
		cpu, sub uint32
		data     []byte
	}
	slices := []slice{
		{0x01000007, 3, amd64}, // CPU_TYPE_X86_64, CPU_SUBTYPE_X86_64_ALL
		{0x0100000C, 0, arm64}, // CPU_TYPE_ARM64, CPU_SUBTYPE_ARM64_ALL
	}
	var head bytes.Buffer
	binary.Write(&head, binary.BigEndian, uint32(0xCAFEBABE)) // «волшебное число» fat-файла
	binary.Write(&head, binary.BigEndian, uint32(len(slices)))
	offset := uint32(align)
	var offsets []uint32
	for _, s := range slices {
		offsets = append(offsets, offset)
		for _, v := range []uint32{s.cpu, s.sub, offset, uint32(len(s.data)), 14} {
			binary.Write(&head, binary.BigEndian, v)
		}
		offset += (uint32(len(s.data)) + align - 1) / align * align
	}
	out := make([]byte, offset)
	copy(out, head.Bytes())
	for i, s := range slices {
		copy(out[offsets[i]:], s.data)
	}
	return out
}

// drawIcon рисует иконку: зелёный скруглённый квадрат с белой «кардиограммой».
func drawIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	bg := color.NRGBA{0x1f, 0x6f, 0x5c, 0xff}
	r := s * 0.22 // радиус скругления
	m := s * 0.08 // отступ от края, как у иконок macOS
	// линия пульса: точки в долях от размера
	pts := [][2]float64{{0.20, 0.54}, {0.38, 0.54}, {0.45, 0.36}, {0.55, 0.70}, {0.62, 0.48}, {0.80, 0.48}}
	width := s * 0.055
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// внутри скруглённого квадрата?
			dx := math.Max(math.Max(m+r-fx, fx-(s-m-r)), 0)
			dy := math.Max(math.Max(m+r-fy, fy-(s-m-r)), 0)
			if dx*dx+dy*dy > r*r || fx < m || fx > s-m || fy < m || fy > s-m {
				continue
			}
			c := bg
			for i := 0; i+1 < len(pts); i++ {
				if distToSegment(fx, fy, pts[i][0]*s, pts[i][1]*s, pts[i+1][0]*s, pts[i+1][1]*s) < width/2 {
					c = color.NRGBA{0xff, 0xff, 0xff, 0xff}
					break
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func distToSegment(px, py, ax, ay, bx, by float64) float64 {
	vx, vy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((px-ax)*vx+(py-ay)*vy)/(vx*vx+vy*vy)))
	return math.Hypot(px-(ax+t*vx), py-(ay+t*vy))
}

// icns — файл иконки macOS: заголовок 'icns' + записи «тип, длина, PNG-картинка».
// ic10 — 1024×1024 (512@2x), ic08 — 256×256.
func icns(big, small image.Image) []byte {
	var body bytes.Buffer
	for _, e := range []struct {
		kind string
		img  image.Image
	}{{"ic10", big}, {"ic08", small}} {
		var p bytes.Buffer
		png.Encode(&p, e.img)
		body.WriteString(e.kind)
		binary.Write(&body, binary.BigEndian, uint32(p.Len()+8))
		body.Write(p.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	return out.Bytes()
}

type zipFile struct {
	name string
	data []byte
	mode os.FileMode
}

// writeZip пишет архив и сохраняет права файлов (иначе на Mac программа не будет «запускаемой»).
func writeZip(path string, files []zipFile) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, file := range files {
		h := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: time.Now()}
		h.SetMode(file.mode) // заодно помечает архив как «сделанный в Unix», чтобы Mac прочитал права
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, bytes.NewReader(file.data)); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Info.plist — «паспорт» приложения для macOS.
// LSUIElement=true: приложение без значка в Доке — оно живёт в браузере, а выключается кнопкой «Выключить».
var infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>` + appName + `</string>
  <key>CFBundleDisplayName</key><string>` + appName + `</string>
  <key>CFBundleIdentifier</key><string>` + bundleID + `</string>
  <key>CFBundleVersion</key><string>` + version + `</string>
  <key>CFBundleShortVersionString</key><string>` + version + `</string>
  <key>CFBundleExecutable</key><string>office-health</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`

var macReadme = `Office Health для Mac
=====================

1. Перетащите «Office Health» в папку «Программы» (Applications).

2. Первый запуск. Программа не из App Store и не подписана Apple, поэтому macOS её сначала не пустит.
   Самый надёжный способ — один раз выполнить команду в программе «Терминал»
   (Программы → Утилиты → Терминал), скопировать целиком и нажать Enter:

       xattr -cr "/Applications/Office Health.app"

   После этого откройте Office Health обычным двойным щелчком.

   Без Терминала: двойной щелчок → macOS скажет, что не может проверить программу → «Готово».
   Затем  → Системные настройки → Конфиденциальность и безопасность → внизу «Всё равно открыть»
   (Open Anyway) → подтвердите паролем.

3. Откроется браузер с панелью: там видно здоровье этого Mac (диск, батарея, аварийные перезагрузки).
   Значка в Доке нет — приложение работает в фоне. Выключить: кнопка «Выключить» в правом верхнем углу
   панели. Открыть панель снова — ещё раз двойной щелчок по Office Health.

4. Подробная проверка диска (износ SSD) — нужна программа smartctl. Если есть Homebrew (brew.sh):

       brew install smartmontools

   Без неё Office Health покажет только общий вердикт диска «исправен / неисправен».

Данные приложения: ~/Library/Application Support/office-health
`

// Пакет rules решает, что «норм», что «скоро менять», а что «срочно».
// Сейчас (шаг 2) это заготовка: уровни есть, а правил ещё нет — всё «не оценено».
package rules

import "office-health/collect"

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
	Title  string // коротко: «Плохие сектора: 128»
	Advice string // по-человечески: что это значит и что делать
}

// Check — итог проверки одной вещи (диска, батареи…).
type Check struct {
	Level    Level
	Findings []Finding
}

// Result — итог по всему компьютеру. Disks и Volumes идут в том же порядке, что в снимке.
type Result struct {
	Overall  Level
	Disks    []Check
	Volumes  []Check
	Battery  Check
	HWErrors Check
}

// Evaluate проверяет снимок. Пока правил нет — всё Unknown; появятся на шаге 6.
func Evaluate(s collect.Snapshot) Result {
	return Result{
		Disks:   make([]Check, len(s.Disks.List)),
		Volumes: make([]Check, len(s.System.Volumes)),
	}
}

package collect

// Ошибки железа. На Windows это события WHEA-Logger в журнале System:
// так Windows записывает, что процессор, память или шина PCIe сообщили об аппаратной ошибке.
// Здесь — общая часть (разбор ответа); сам запрос к журналу — в errors_windows.go.
// Для Mac потом появится errors_darwin.go: там вместо WHEA — отчёты kernel panic.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	HWErrorDays   = 30 // за сколько последних дней смотреть журнал
	HWErrorRecent = 5  // сколько последних записей показывать в отчёте
)

// winEvent — одна запись так, как её отдаёт наш PowerShell-скрипт.
type winEvent struct {
	Time    time.Time `json:"Time"`
	ID      int       `json:"Id"`
	Message string    `json:"Message"`
}

// parseWinEvents разбирает JSON-список событий (от новых к старым) в HWErrors.
func parseWinEvents(data []byte) (HWErrors, error) {
	var events []winEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return HWErrors{}, fmt.Errorf("не понял ответ журнала: %w", err)
	}
	res := HWErrors{Days: HWErrorDays, Count: len(events)}
	for i, e := range events {
		if i == HWErrorRecent {
			break
		}
		res.Recent = append(res.Recent, HWEvent{
			Time:    e.Time.Local(),
			ID:      e.ID,
			Message: firstLine(e.Message),
		})
	}
	return res, nil
}

// firstLine — у событий WHEA длинный текст на много строк; в отчёт берём первую.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "(без описания)"
	}
	return s
}

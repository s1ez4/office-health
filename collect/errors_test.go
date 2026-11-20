package collect

import "testing"

func TestParseWinEvents(t *testing.T) {
	data := []byte(`[
		{"Time":"2026-11-19T10:00:00.0000000Z","Id":19,"Message":"Произошла исправленная аппаратная ошибка.\r\n\r\nТип компонента: Ядро процессора"},
		{"Time":"2026-11-10T08:30:00.0000000Z","Id":17,"Message":"Произошла исправленная аппаратная ошибка."},
		{"Time":"2026-11-01T08:30:00.0000000Z","Id":47,"Message":null},
		{"Time":"2026-10-30T08:30:00.0000000Z","Id":17,"Message":"x"},
		{"Time":"2026-10-29T08:30:00.0000000Z","Id":17,"Message":"x"},
		{"Time":"2026-10-28T08:30:00.0000000Z","Id":17,"Message":"x"}
	]`)
	res, err := parseWinEvents(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 6 || len(res.Recent) != HWErrorRecent {
		t.Errorf("всего %d, показано %d — ждали 6 и %d", res.Count, len(res.Recent), HWErrorRecent)
	}
	if res.Recent[0].Message != "Произошла исправленная аппаратная ошибка." || res.Recent[0].ID != 19 {
		t.Errorf("первая запись: %+v", res.Recent[0])
	}
	if res.Recent[2].Message != "(без описания)" {
		t.Errorf("пустое описание: %q", res.Recent[2].Message)
	}
}

func TestParseNoEvents(t *testing.T) {
	res, err := parseWinEvents([]byte(`[]`))
	if err != nil || res.Count != 0 || res.Days != HWErrorDays {
		t.Errorf("пустой журнал: %+v, %v", res, err)
	}
}

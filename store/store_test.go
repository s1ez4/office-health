package store

import (
	"testing"

	"office-health/collect"
)

// Много проверок подряд, быстрее, чем тикают часы: ни одна не должна потеряться,
// и история должна идти от новых к старым.
func TestHistoryKeepsEverySnapshot(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 50
	for i := 0; i < n; i++ {
		s := collect.Demo()
		s.System.UptimeHours = float64(i) // метка, по которой узнаем порядок
		if err := st.AddSnapshot(SelfID, s); err != nil {
			t.Fatal(err)
		}
	}
	h := st.History(SelfID, 100)
	if len(h) != n {
		t.Fatalf("сохранилось %d проверок из %d", len(h), n)
	}
	for i, s := range h {
		if int(s.System.UptimeHours) != n-1-i {
			t.Fatalf("порядок нарушен: на месте %d проверка %v", i, s.System.UptimeHours)
		}
	}
	if latest, _ := st.Latest(SelfID); latest.System.UptimeHours != n-1 {
		t.Error("последней считается не последняя проверка")
	}
}

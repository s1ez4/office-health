package app

// Тесты приложения без настоящей сети: httptest подставляет «поддельный» запрос и записывает ответ.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"office-health/agent"
	"office-health/collect"
	"office-health/store"
)

func newTestApp(t *testing.T, demo bool) *App {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if demo {
		if err := fillDemo(st); err != nil {
			t.Fatal(err)
		}
	}
	return &App{opt: Options{UIPort: 8765, AgentPort: 8766, Demo: demo}, store: st, pages: parsePages(), quit: make(chan struct{})}
}

// do — выполнить запрос к обработчику и вернуть ответ.
func do(h http.Handler, method, url, host string, body []byte, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, bytes.NewReader(body))
	req.Host = host
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const local = "127.0.0.1:8765"

func TestPagesRender(t *testing.T) {
	a := newTestApp(t, true)
	ui := a.uiRoutes()

	rec := do(ui, "GET", "/", local, nil, nil)
	body := rec.Body.String()
	for _, want := range []string{"DIRECTOR-LAPTOP", "Бухгалтерия", "Ресепшен", "Склад", "1 срочно", "4 дн. не присылал данные", "Заменить HDD"} {
		if !strings.Contains(body, want) {
			t.Errorf("на главной нет %q", want)
		}
	}
	for _, m := range a.store.Machines() {
		rec := do(ui, "GET", "/m/"+m.ID, local, nil, nil)
		if rec.Code != 200 || strings.Contains(rec.Body.String(), "<no value>") {
			t.Errorf("страница %s: код %d", m.Name, rec.Code)
		}
		if m.Name == "Бухгалтерия" && !strings.Contains(rec.Body.String(), `<span class="t-urgent">&#43;54</span>`) {
			t.Error("в истории бухгалтерии не видно роста секторов +54")
		}
	}
	if rec := do(ui, "GET", "/add", local, nil, nil); rec.Code != 200 {
		t.Errorf("/add: %d", rec.Code)
	}
	rec = do(ui, "POST", "/add", local, nil, nil)
	if !strings.Contains(rec.Body.String(), "join OH1-") {
		t.Error("после «Создать приглашение» нет кода")
	}
}

func TestGuard(t *testing.T) {
	ui := newTestApp(t, false).uiRoutes()
	// Чужое имя хоста: так выглядит атака «DNS rebinding» — чужой сайт притворяется нашим адресом
	if rec := do(ui, "GET", "/", "evil.example:8765", nil, nil); rec.Code != 403 {
		t.Errorf("чужой Host: %d, ждали 403", rec.Code)
	}
	// Кнопку нажимает чужой сайт из того же браузера
	if rec := do(ui, "POST", "/add", local, nil, map[string]string{"Origin": "http://evil.example"}); rec.Code != 403 {
		t.Errorf("чужой Origin: %d, ждали 403", rec.Code)
	}
	if rec := do(ui, "GET", "/api/ping", "localhost:8765", nil, nil); rec.Body.String() != pingAnswer {
		t.Error("ping по localhost не работает")
	}
}

func TestAgentFlow(t *testing.T) {
	a := newTestApp(t, false)
	ag := a.agentRoutes()
	_, token, _ := a.store.NewInvite()

	join := func(tok string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(agent.JoinRequest{Token: tok, Name: "BUH-PC", Platform: "windows"})
		return do(ag, "POST", "/api/agent/join", "192.168.1.5:8766", body, nil)
	}
	rec := join(token)
	if rec.Code != 200 {
		t.Fatalf("join: %d %s", rec.Code, rec.Body)
	}
	var jr agent.JoinResponse
	json.Unmarshal(rec.Body.Bytes(), &jr)

	if rec := join(token); rec.Code != 403 {
		t.Errorf("приглашение сработало второй раз: %d", rec.Code)
	}
	if rec := join("подобранный-код"); rec.Code != 403 {
		t.Errorf("чужой код: %d", rec.Code)
	}

	snap, _ := json.Marshal(collect.Demo())
	send := func(id, key string) int {
		return do(ag, "POST", "/api/agent/report?id="+id, "192.168.1.5:8766", snap,
			map[string]string{"Authorization": "Bearer " + key}).Code
	}
	if code := send(jr.ID, jr.Key); code != 204 {
		t.Errorf("отчёт агента: %d", code)
	}
	if code := send(jr.ID, "не-тот-ключ"); code != 401 {
		t.Errorf("чужой ключ: %d", code)
	}
	if code := send(store.SelfID, jr.Key); code != 401 {
		t.Errorf("агент подсунул данные за «этот» компьютер: %d", code)
	}
	if s, ok := a.store.Latest(jr.ID); !ok || s.System.Hostname != "BUH-PC-03" || s.Demo {
		t.Error("снимок агента не сохранился")
	}

	// Агенту читать нечего: ни панели, ни статусов
	for _, path := range []string{"/", "/m/" + jr.ID, "/api/agent/report?id=" + jr.ID} {
		if rec := do(ag, "GET", path, "192.168.1.5:8766", nil, nil); rec.Code < 400 {
			t.Errorf("GET %s через вход агентов: %d — а должно быть закрыто", path, rec.Code)
		}
	}

	// Убрали компьютер — ключ больше не действует
	a.store.Remove(jr.ID)
	if code := send(jr.ID, jr.Key); code != 401 {
		t.Errorf("после удаления: %d", code)
	}
}

func TestCodeRoundTrip(t *testing.T) {
	c := agent.Code{URLs: []string{"http://192.168.1.5:8766", "http://10.0.0.7:8766"}, Token: "abc"}
	got, err := agent.DecodeCode("  " + agent.EncodeCode(c) + "\n")
	if err != nil || got.Token != "abc" || len(got.URLs) != 2 {
		t.Errorf("код: %+v %v", got, err)
	}
	if _, err := agent.DecodeCode("OH1-мусор"); err == nil {
		t.Error("испорченный код принят")
	}
}

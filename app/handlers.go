package app

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"office-health/agent"
	"office-health/collect"
	"office-health/report"
	"office-health/rules"
	"office-health/store"
)

//go:embed ui/*.html
var uiFiles embed.FS // все страницы приложения вшиты в программу

// uiRoutes — адреса панели владельца. Шаблон «МЕТОД /путь/{переменная}» понимает Go 1.22+.
func (a *App) uiRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.dashboard) // {$} — ровно «/», а не «всё, что начинается с /»
	mux.HandleFunc("GET /m/{id}", a.machine)
	mux.HandleFunc("POST /check", a.check)
	mux.HandleFunc("GET /add", a.addPage)
	mux.HandleFunc("POST /add", a.createInvite)
	mux.HandleFunc("POST /invites/{id}/cancel", a.cancelInvite)
	mux.HandleFunc("POST /m/{id}/rename", a.rename)
	mux.HandleFunc("POST /m/{id}/remove", a.remove)
	mux.HandleFunc("POST /join", a.joinOther)
	mux.HandleFunc("POST /quit", a.quitApp)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, pingAnswer) })
	return a.guard(mux)
}

// agentRoutes — вход для агентов из сети. Ничего не отдаёт, только принимает.
func (a *App) agentRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/agent/join", a.agentJoin)
	mux.HandleFunc("POST /api/agent/report", a.agentReport)
	return mux
}

// --- что показываем ---

// card — один компьютер на главной странице.
type card struct {
	M        store.Machine
	Has      bool // была ли хоть одна проверка
	S        collect.Snapshot
	R        rules.Result
	Findings []rules.Finding // самое важное, худшее сверху
	Silent   time.Duration   // сколько агент молчит (0 — всё в порядке)
}

const silentAfter = 48 * time.Hour // агент не выходил на связь двое суток — показываем предупреждение

func (a *App) card(m store.Machine) card {
	c := card{M: m}
	if snap, ok := a.store.Latest(m.ID); ok {
		c.Has, c.S, c.R = true, snap, rules.Evaluate(snap)
		c.Findings = topFindings(c.R, 3)
	}
	if !m.Self && !m.LastSeen.IsZero() && time.Since(m.LastSeen) > silentAfter {
		c.Silent = time.Since(m.LastSeen)
	}
	if c.M.Name == "" {
		c.M.Name = c.S.System.Hostname
	}
	if c.M.Name == "" && m.Self {
		c.M.Name, _ = os.Hostname()
	}
	if c.M.Name == "" {
		c.M.Name = "Новый компьютер"
	}
	return c
}

// topFindings — все находки компьютера, худшие первыми.
func topFindings(r rules.Result, n int) []rules.Finding {
	var all []rules.Finding
	for _, c := range append(append([]rules.Check{r.Battery, r.HWErrors}, r.Disks...), r.Volumes...) {
		all = append(all, c.Findings...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Level > all[j].Level })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

type dashboardData struct {
	Cards    []card
	Checking bool
	Counts   map[rules.Level]int
	Demo     bool
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	d := dashboardData{Checking: a.checking.Load(), Counts: map[rules.Level]int{}, Demo: a.opt.Demo}
	for _, m := range a.store.Machines() {
		c := a.card(m)
		d.Cards = append(d.Cards, c)
		if c.Has {
			d.Counts[c.R.Overall]++
		}
	}
	// «Этот» компьютер — первым, дальше худшие сверху
	sort.SliceStable(d.Cards, func(i, j int) bool {
		if d.Cards[i].M.Self != d.Cards[j].M.Self {
			return d.Cards[i].M.Self
		}
		return d.Cards[i].R.Overall > d.Cards[j].R.Overall
	})
	a.render(w, "dashboard", d)
}

// historyRow — строка истории: главные цифры одной проверки.
type historyRow struct {
	Time    time.Time
	Level   rules.Level
	Sectors int64  // плохих секторов на всех дисках
	Wear    string // износ SSD, худший
	Battery string
	Errors  string
	Delta   int64 // насколько выросло число плохих секторов с предыдущей проверки
}

type machineData struct {
	Card     card
	Page     report.Page
	History  []historyRow
	Checking bool
	Demo     bool
}

func (a *App) machine(w http.ResponseWriter, r *http.Request) {
	m, ok := a.store.Machine(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	d := machineData{Card: a.card(m), Checking: a.checking.Load(), Demo: a.opt.Demo}
	if d.Card.Has {
		d.Page = report.NewPage(d.Card.S, d.Card.R)
	}
	snaps := a.store.History(m.ID, 15)
	for i, s := range snaps {
		row := summarize(s)
		if i+1 < len(snaps) { // сравниваем с проверкой перед этой (история идёт от новых к старым)
			row.Delta = row.Sectors - summarize(snaps[i+1]).Sectors
		}
		d.History = append(d.History, row)
	}
	a.render(w, "machine", d)
}

func summarize(s collect.Snapshot) historyRow {
	row := historyRow{Time: s.CollectedAt, Level: rules.Evaluate(s).Overall, Wear: "—", Battery: "—", Errors: "—"}
	var wear int64 = -1
	for _, d := range s.Disks.List {
		for _, p := range []*int64{d.Reallocated, d.Pending, d.Uncorrectable} {
			if p != nil {
				row.Sectors += *p
			}
		}
		if d.PercentUsed != nil && *d.PercentUsed > wear {
			wear = *d.PercentUsed
		}
	}
	if wear >= 0 {
		row.Wear = fmt.Sprintf("%d%%", wear)
	}
	if s.Battery != nil && s.Battery.Present {
		row.Battery = fmt.Sprintf("%.0f%%", s.Battery.HealthPct)
	}
	if s.HWErrors != nil {
		row.Errors = fmt.Sprint(s.HWErrors.Count)
	}
	return row
}

func (a *App) check(w http.ResponseWriter, r *http.Request) {
	if !a.opt.Demo {
		go a.checkSelf() // проверка идёт несколько секунд — не заставляем браузер ждать
		time.Sleep(300 * time.Millisecond)
	}
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") { // только наши адреса
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// --- добавление компьютеров ---

type addData struct {
	Code      string // только что созданный код (показываем один раз)
	Invites   []store.Invite
	LAN       []string
	AgentPort int
	AgentErr  error
	JoinMsg   string
	JoinErr   string
	Platform  string
}

func (a *App) addData() addData {
	a.agentMu.Lock()
	err := a.agentErr
	a.agentMu.Unlock()
	return addData{Invites: a.store.Invites(), LAN: LANAddrs(), AgentPort: a.opt.AgentPort,
		AgentErr: err, Platform: runtime.GOOS}
}

func (a *App) addPage(w http.ResponseWriter, r *http.Request) { a.render(w, "add", a.addData()) }

func (a *App) createInvite(w http.ResponseWriter, r *http.Request) {
	_, token, err := a.store.NewInvite()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.startAgentServer()
	d := a.addData()
	var urls []string
	for _, ip := range d.LAN {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, a.opt.AgentPort))
	}
	if len(urls) == 0 { // нет сети — код всё равно покажем, но предупредим в шаблоне
		urls = []string{fmt.Sprintf("http://127.0.0.1:%d", a.opt.AgentPort)}
	}
	d.Code = agent.EncodeCode(agent.Code{URLs: urls, Token: token})
	a.render(w, "add", d)
}

func (a *App) cancelInvite(w http.ResponseWriter, r *http.Request) {
	a.store.CancelInvite(r.PathValue("id"))
	http.Redirect(w, r, "/add", http.StatusSeeOther)
}

func (a *App) rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if name := strings.TrimSpace(r.FormValue("name")); name != "" && len(name) <= 80 {
		a.store.Rename(id, name)
	}
	http.Redirect(w, r, "/m/"+id, http.StatusSeeOther)
}

func (a *App) remove(w http.ResponseWriter, r *http.Request) {
	a.store.Remove(r.PathValue("id"))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// joinOther — сделать ЭТОТ компьютер агентом другого владельца (вставили код в панели).
func (a *App) joinOther(w http.ResponseWriter, r *http.Request) {
	d := a.addData()
	if err := agent.Join(r.FormValue("code")); err != nil {
		d.JoinErr = err.Error()
	} else if msg, err := agent.InstallSchedule(); err != nil {
		d.JoinMsg = "Подключено, первая проверка отправлена. Но расписание не поставилось: " + err.Error()
	} else {
		d.JoinMsg = "Подключено, первая проверка отправлена. Дальше — " + msg + "."
	}
	a.render(w, "add", d)
}

func (a *App) quitApp(w http.ResponseWriter, r *http.Request) {
	a.render(w, "message", "Office Health выключен. Окно браузера можно закрыть.")
	go func() { // выключаемся чуть позже — чтобы браузер успел получить страницу
		time.Sleep(500 * time.Millisecond)
		close(a.quit)
	}()
}

// --- вход для агентов ---

const maxReport = 4 << 20 // 4 МБ: снимок весит несколько килобайт, больше — подозрительно

func (a *App) agentJoin(w http.ResponseWriter, r *http.Request) {
	var req agent.JoinRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "плохой запрос", http.StatusBadRequest)
		return
	}
	id, key, err := a.store.Join(req.Token, req.Name, req.Platform)
	if errors.Is(err, store.ErrBadInvite) {
		http.Error(w, "приглашение недействительно: оно одноразовое и живёт 7 дней — попросите новое", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, "ошибка на стороне владельца", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent.JoinResponse{ID: id, Key: key})
}

func (a *App) agentReport(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !a.store.Auth(id, key) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var snap collect.Snapshot
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReport)).Decode(&snap); err != nil {
		http.Error(w, "плохой снимок", http.StatusBadRequest)
		return
	}
	snap.Demo = false
	if err := a.store.AddSnapshot(id, snap); err != nil {
		http.Error(w, "не сохранить", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent) // «принято», и больше ничего: статус агенту не сообщаем
}

// --- функции для шаблонов страниц ---

var uiFuncs = template.FuncMap{
	"ago": func(t time.Time) string {
		if t.IsZero() {
			return "ещё не было"
		}
		d := time.Since(t)
		now := time.Now()
		switch {
		case d < time.Minute:
			return "только что"
		case d < time.Hour:
			return fmt.Sprintf("%d мин назад", int(d.Minutes()))
		case t.YearDay() == now.YearDay() && t.Year() == now.Year():
			return "сегодня в " + t.Format("15:04")
		case d < 48*time.Hour && t.YearDay() == now.Add(-24*time.Hour).YearDay():
			return "вчера в " + t.Format("15:04")
		}
		return t.Format("02.01.2006 15:04")
	},
	"days": func(d time.Duration) int { return int(d.Hours() / 24) },
	"platform": func(p string) string {
		switch p {
		case "windows":
			return "Windows"
		case "darwin":
			return "Mac"
		case "linux":
			return "Linux"
		}
		return "компьютер"
	},
	"count": func(m map[rules.Level]int, l int) int { return m[rules.Level(l)] },
	"plus": func(n int64) string {
		if n > 0 {
			return fmt.Sprintf("+%d", n)
		}
		return ""
	},
	"lvl": func(l int) rules.Level { return rules.Level(l) },
}

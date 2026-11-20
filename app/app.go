// Пакет app — приложение владельца: маленький веб-сервер внутри программы и страницы в браузере.
//
// Два «входа» (два порта), и это главное про безопасность:
//
//	127.0.0.1:8765 — панель владельца. Адрес 127.0.0.1 значит «только с этого же компьютера»:
//	                 из сети её не открыть, поэтому агенты (и вообще никто) статусы не видят.
//	0.0.0.0:8766   — приём данных от агентов из локальной сети. Там всего два действия:
//	                 «подключиться по приглашению» и «принять снимок по ключу». Читать оттуда нечего.
//	                 Этот вход открывается, только когда есть приглашение или добавленный компьютер.
package app

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"office-health/collect"
	"office-health/report"
	"office-health/store"
)

// Options — настройки запуска приложения.
type Options struct {
	Dir       string // где хранить данные
	UIPort    int    // панель владельца (только 127.0.0.1)
	AgentPort int    // приём от агентов (локальная сеть)
	Open      bool   // открыть браузер после запуска
	Demo      bool   // выдуманные компьютеры, ничего не проверять по-настоящему
}

const pingAnswer = "office-health"

// CheckEvery — как часто приложение само проверяет «этот» компьютер, пока запущено.
const CheckEvery = 6 * time.Hour

type App struct {
	opt      Options
	store    *store.Store
	pages    map[string]*template.Template
	checking atomic.Bool // идёт ли сейчас проверка (atomic — безопасно читать и писать из разных потоков)

	agentMu  sync.Mutex
	agentSrv *http.Server
	agentErr error // не удалось открыть вход для агентов (например, порт занят)

	ui   *http.Server
	quit chan struct{}
}

// UIURL — адрес панели в браузере.
func (o Options) UIURL() string { return fmt.Sprintf("http://127.0.0.1:%d/", o.UIPort) }

// AlreadyRunning — приложение уже запущено (например, второй двойной щелчок)?
func AlreadyRunning(o Options) bool {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(o.UIURL() + "api/ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	return string(b) == pingAnswer
}

// Run запускает приложение и работает, пока его не выключат кнопкой «Выключить».
func Run(opt Options) error {
	st, err := store.Open(opt.Dir)
	if err != nil {
		return fmt.Errorf("не открыть папку с данными %s: %w", opt.Dir, err)
	}
	a := &App{opt: opt, store: st, pages: parsePages(), quit: make(chan struct{})}
	if opt.Demo {
		if err := fillDemo(st); err != nil {
			return err
		}
	}

	// net.Listen сначала «занимает» порт — так мы сразу узнаём, если он занят.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opt.UIPort))
	if err != nil {
		return fmt.Errorf("порт %d занят другой программой: %w", opt.UIPort, err)
	}
	a.ui = &http.Server{Handler: a.uiRoutes(), ReadHeaderTimeout: 10 * time.Second}

	// go — запустить функцию «параллельно», в отдельной горутине (лёгком потоке).
	// Основная программа при этом идёт дальше, не дожидаясь её.
	go a.ui.Serve(ln)
	if st.HasAgents() {
		a.startAgentServer()
	}
	if !opt.Demo {
		go a.selfCheckLoop()
	}

	fmt.Println("Office Health работает. Панель:", opt.UIURL())
	fmt.Println("Не закрывайте это окно — закроете, и приложение выключится.")
	if opt.Open {
		OpenBrowser(opt.UIURL())
	}

	<-a.quit // ждём сигнала «выключиться» (кнопка в панели) — до тех пор программа стоит здесь
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a.ui.Shutdown(ctx)
	a.agentMu.Lock()
	if a.agentSrv != nil {
		a.agentSrv.Shutdown(ctx)
	}
	a.agentMu.Unlock()
	return nil
}

// selfCheckLoop проверяет «этот» компьютер при запуске (если давно не проверяли) и дальше каждые 6 часов.
func (a *App) selfCheckLoop() {
	if snap, ok := a.store.Latest(store.SelfID); !ok || time.Since(snap.CollectedAt) > CheckEvery {
		a.checkSelf()
	}
	ticker := time.NewTicker(CheckEvery)
	defer ticker.Stop()
	for {
		select { // select ждёт то событие, которое наступит первым
		case <-ticker.C:
			a.checkSelf()
		case <-a.quit:
			return
		}
	}
}

// checkSelf — проверить «этот» компьютер. Если проверка уже идёт — вторую не начинаем.
func (a *App) checkSelf() {
	if !a.checking.CompareAndSwap(false, true) { // «если было false — поставь true», атомарно
		return
	}
	defer a.checking.Store(false)
	snap := collect.Run()
	if err := a.store.AddSnapshot(store.SelfID, snap); err != nil {
		log.Println("не сохранить проверку:", err)
	}
}

// startAgentServer открывает вход для агентов, если он ещё не открыт.
func (a *App) startAgentServer() {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	if a.agentSrv != nil {
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", a.opt.AgentPort)) // ":8766" — на всех сетевых адресах
	if err != nil {
		a.agentErr = fmt.Errorf("не открыть порт %d для других компьютеров: %v", a.opt.AgentPort, err)
		return
	}
	a.agentErr = nil
	a.agentSrv = &http.Server{Handler: a.agentRoutes(), ReadHeaderTimeout: 10 * time.Second}
	go a.agentSrv.Serve(ln)
}

// LANAddrs — адреса этого компьютера в локальной сети (192.168.x.x, 10.x.x.x…), куда будут стучаться агенты.
func LANAddrs() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet) // «приведение типа»: это сетевой адрес с маской?
			if !ok {
				continue
			}
			if ip := ipnet.IP.To4(); ip != nil && ip.IsPrivate() {
				out = append(out, ip.String())
			}
		}
	}
	return out
}

// OpenBrowser открывает адрес в браузере по умолчанию.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

// --- помощники для обработчиков ---

func (a *App) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		log.Println("страница", page, err)
	}
}

func isLocalHost(host string, port int) bool {
	for _, h := range []string{"127.0.0.1", "localhost"} {
		if strings.EqualFold(host, fmt.Sprintf("%s:%d", h, port)) {
			return true
		}
	}
	return false
}

// guard — «охранник» перед панелью владельца. Пропускает только запросы к 127.0.0.1/localhost
// и не даёт чужим сайтам в браузере нажимать наши кнопки (проверка Origin у POST-запросов).
func (a *App) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host, a.opt.UIPort) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if o := r.Header.Get("Origin"); o != "" && !isLocalHost(strings.TrimPrefix(o, "http://"), a.opt.UIPort) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// parsePages собирает шаблоны: к шаблону отчёта добавляем общий макет и по одной странице.
func parsePages() map[string]*template.Template {
	base := template.Must(report.NewTemplate().Funcs(uiFuncs).ParseFS(uiFiles, "ui/layout.html"))
	pages := map[string]*template.Template{}
	for _, name := range []string{"dashboard", "machine", "add", "message"} {
		pages[name] = template.Must(template.Must(base.Clone()).ParseFS(uiFiles, "ui/"+name+".html"))
	}
	return pages
}

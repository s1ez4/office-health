// Пакет agent — режим «добавленного» компьютера. Агент ничего не показывает и ничего не читает
// у владельца: он только собирает снимок и отправляет его владельцу по своему ключу.
//
//	office-health join <код>  — подключиться по приглашению (один раз)
//	office-health agent       — отправить проверку сейчас (это и запускает расписание)
//	office-health leave       — отключиться: удалить ключ и расписание
package agent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"office-health/collect"
)

const codePrefix = "OH1-" // «Office Health, версия 1» — чтобы узнавать свой код среди любого текста

// Code — содержимое кода приглашения: куда стучаться и одноразовый секрет.
type Code struct {
	URLs  []string `json:"u"` // адреса владельца в локальной сети: http://192.168.1.5:8766
	Token string   `json:"t"`
}

// EncodeCode / DecodeCode превращают Code в строку «OH1-…» и обратно (JSON → base64).
func EncodeCode(c Code) string {
	data, _ := json.Marshal(c)
	return codePrefix + base64.RawURLEncoding.EncodeToString(data)
}

func DecodeCode(s string) (Code, error) {
	var c Code
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, codePrefix) {
		return c, errors.New("это не код приглашения office-health (он начинается с OH1-)")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, codePrefix))
	if err == nil {
		err = json.Unmarshal(data, &c)
	}
	if err != nil || len(c.URLs) == 0 || c.Token == "" {
		return c, errors.New("код приглашения повреждён — скопируйте его целиком")
	}
	return c, nil
}

// Config — что агент помнит после подключения.
type Config struct {
	URLs []string `json:"urls"`
	ID   string   `json:"id"`
	Key  string   `json:"key"`
}

func configPath() string { return filepath.Join(ConfigDir(), "agent.json") }

// LoadConfig читает настройки агента. Ошибка — компьютер ещё не подключён.
func LoadConfig() (Config, error) {
	var c Config
	data, err := os.ReadFile(configPath())
	if err != nil {
		return c, errors.New("этот компьютер не подключён — сначала office-health join <код>")
	}
	return c, json.Unmarshal(data, &c)
}

func saveConfig(c Config) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(), data, 0o600) // 0600 — читать может только владелец файла
}

var client = &http.Client{Timeout: 20 * time.Second}

// post отправляет JSON на первый откликнувшийся адрес владельца. Возвращает ответ и адрес, который сработал.
func post(urls []string, path, key string, body any) (*http.Response, string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	var lastErr error
	for _, u := range urls {
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(u, "/")+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue // этот адрес не отвечает — пробуем следующий
		}
		return resp, u, nil
	}
	return nil, "", fmt.Errorf("компьютер владельца не отвечает (%v). Он должен быть включён, "+
		"приложение office-health на нём — запущено, а оба компьютера — в одной сети", lastErr)
}

// JoinRequest / JoinResponse — что агент шлёт при подключении и что получает в ответ.
type JoinRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type JoinResponse struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// Join подключает этот компьютер по коду приглашения и сразу отправляет первую проверку.
func Join(code string) error {
	c, err := DecodeCode(code)
	if err != nil {
		return err
	}
	fmt.Println("Проверяю этот компьютер…")
	snap := collect.Run()

	resp, url, err := post(c.URLs, "/api/agent/join", "", JoinRequest{
		Token: c.Token, Name: snap.System.Hostname, Platform: snap.Platform,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("владелец не принял приглашение: %s", readError(resp))
	}
	var jr JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return err
	}
	// Сработавший адрес ставим первым — с ним и будем работать дальше
	urls := append([]string{url}, without(c.URLs, url)...)
	cfg := Config{URLs: urls, ID: jr.ID, Key: jr.Key}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	return send(cfg, snap)
}

// Send — собрать проверку и отправить владельцу (режим `office-health agent`).
func Send() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	return send(cfg, collect.Run())
}

func send(cfg Config, snap collect.Snapshot) error {
	resp, _, err := post(cfg.URLs, "/api/agent/report?id="+cfg.ID, cfg.Key, snap)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnauthorized:
		return errors.New("владелец больше не принимает данные с этого компьютера (его убрали из списка). " +
			"Отключите агента: office-health leave")
	}
	return fmt.Errorf("владелец ответил ошибкой: %s", readError(resp))
}

// Leave — удалить настройки агента и расписание.
func Leave() error {
	err := RemoveSchedule()
	os.Remove(configPath())
	return err
}

func readError(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	if msg := strings.TrimSpace(string(b)); msg != "" {
		return msg
	}
	return resp.Status
}

func without(list []string, x string) []string {
	var out []string
	for _, v := range list {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

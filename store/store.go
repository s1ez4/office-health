// Пакет store хранит данные приложения на диске: какие компьютеры добавлены, приглашения
// и историю проверок каждого компьютера.
//
// Устроено просто, без базы данных — обычные файлы в папке приложения:
//
//	state.json                 — компьютеры и приглашения
//	history/<id>/<время>.json  — снимки (Snapshot) каждого компьютера, по одному на проверку
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"office-health/collect"
)

const (
	SelfID      = "self"             // id «этого» компьютера, на котором стоит приложение
	KeepHistory = 90                 // сколько последних проверок хранить на компьютер
	InviteTTL   = 7 * 24 * time.Hour // сколько живёт приглашение
)

// Machine — компьютер в приложении: «этот» или добавленный агент.
type Machine struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Platform string    `json:"platform,omitempty"`
	Self     bool      `json:"self,omitempty"`
	KeyHash  string    `json:"key_hash,omitempty"` // хеш ключа агента; сам ключ знает только агент
	AddedAt  time.Time `json:"added_at"`
	LastSeen time.Time `json:"last_seen,omitempty"` // когда последний раз прислал данные
}

// Invite — приглашение добавить компьютер. Храним только хеш кода: даже если кто-то
// прочитает state.json, по хешу приглашение не воспользоваться.
type Invite struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"token_hash"`
	Created   time.Time `json:"created"`
	Expires   time.Time `json:"expires"`
}

type state struct {
	Machines []Machine `json:"machines"`
	Invites  []Invite  `json:"invites"`
}

// Store — хранилище. Его вызывают одновременно и страница владельца, и агенты из сети,
// поэтому все методы берут «замок» mu: пока один меняет данные, другой ждёт.
type Store struct {
	dir    string
	mu     sync.Mutex
	st     state
	latest map[string]collect.Snapshot // последний снимок каждого компьютера — держим в памяти
}

// Open открывает (или создаёт) хранилище в папке dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "history"), 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, latest: map[string]collect.Snapshot{}}
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		// первый запуск — файла ещё нет
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.st); err != nil {
			return nil, err
		}
	}
	if s.find(SelfID) < 0 {
		s.st.Machines = append([]Machine{{ID: SelfID, Self: true, AddedAt: time.Now()}}, s.st.Machines...)
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	for _, m := range s.st.Machines {
		if h := s.history(m.ID, 1); len(h) > 0 {
			s.latest[m.ID] = h[0]
		}
	}
	return s, nil
}

// DefaultDir — где приложение хранит данные:
// Windows: C:\Users\<имя>\AppData\Roaming\office-health, Mac: ~/Library/Application Support/office-health.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "office-health")
}

// save записывает state.json. Сначала во временный файл, потом подменяем —
// так при сбое посреди записи старый файл не испортится.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "state.json"))
}

func (s *Store) find(id string) int {
	for i, m := range s.st.Machines {
		if m.ID == id {
			return i
		}
	}
	return -1
}

// Machines — копия списка компьютеров («этот» первым).
func (s *Store) Machines() []Machine {
	s.mu.Lock()
	defer s.mu.Unlock() // «отпусти замок, когда выйдешь из функции»
	return append([]Machine(nil), s.st.Machines...)
}

// Machine — один компьютер по id.
func (s *Store) Machine(id string) (Machine, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.find(id); i >= 0 {
		return s.st.Machines[i], true
	}
	return Machine{}, false
}

// Latest — последний снимок компьютера (ok=false — проверок ещё не было).
func (s *Store) Latest(id string) (collect.Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.latest[id]
	return snap, ok
}

// AddSnapshot сохраняет новую проверку компьютера и удаляет самые старые сверх KeepHistory.
func (s *Store) AddSnapshot(id string, snap collect.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	if i < 0 {
		return errors.New("нет такого компьютера")
	}
	dir := filepath.Join(s.dir, "history", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	name := time.Now().UTC().Format("20060102T150405.000") + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return err
	}
	s.latest[id] = snap
	m := &s.st.Machines[i] // & — берём адрес элемента, чтобы менять его, а не копию
	m.LastSeen = time.Now()
	m.Platform = snap.Platform
	if m.Name == "" {
		m.Name = snap.System.Hostname
	}
	s.prune(dir)
	return s.save()
}

// History — до n последних проверок компьютера, новые первыми.
func (s *Store) History(id string, n int) []collect.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history(id, n)
}

func (s *Store) history(id string, n int) []collect.Snapshot {
	files := s.files(filepath.Join(s.dir, "history", id))
	var out []collect.Snapshot
	for i := len(files) - 1; i >= 0 && len(out) < n; i-- {
		data, err := os.ReadFile(files[i])
		if err != nil {
			continue
		}
		var snap collect.Snapshot
		if json.Unmarshal(data, &snap) == nil {
			out = append(out, snap)
		}
	}
	return out
}

// files — файлы истории по порядку (имя = время, поэтому сортировка по имени = по времени).
func (s *Store) files(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

func (s *Store) prune(dir string) {
	files := s.files(dir)
	for len(files) > KeepHistory {
		os.Remove(files[0])
		files = files[1:]
	}
}

// SetLastSeen — поменять время последней связи (нужно для демо-режима и тестов).
func (s *Store) SetLastSeen(id string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.find(id); i >= 0 {
		s.st.Machines[i].LastSeen = t
	}
	return s.save()
}

// Rename — переименовать компьютер.
func (s *Store) Rename(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	if i < 0 {
		return errors.New("нет такого компьютера")
	}
	s.st.Machines[i].Name = strings.TrimSpace(name)
	return s.save()
}

// Remove — убрать агента вместе с историей. «Этот» компьютер убрать нельзя.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	if i < 0 || s.st.Machines[i].Self {
		return errors.New("этот компьютер нельзя убрать")
	}
	s.st.Machines = append(s.st.Machines[:i], s.st.Machines[i+1:]...) // вырезать i-й элемент
	delete(s.latest, id)
	os.RemoveAll(filepath.Join(s.dir, "history", id))
	return s.save()
}

// --- приглашения и ключи агентов ---

// NewInvite создаёт приглашение и возвращает его код (секрет). Код показываем владельцу один раз.
func (s *Store) NewInvite() (Invite, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := randomHex(16)
	inv := Invite{ID: randomHex(4), TokenHash: hash(token), Created: time.Now(), Expires: time.Now().Add(InviteTTL)}
	s.dropExpired()
	s.st.Invites = append(s.st.Invites, inv)
	return inv, token, s.save()
}

// Invites — действующие приглашения.
func (s *Store) Invites() []Invite {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropExpired()
	return append([]Invite(nil), s.st.Invites...)
}

// CancelInvite — отозвать приглашение.
func (s *Store) CancelInvite(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, inv := range s.st.Invites {
		if inv.ID == id {
			s.st.Invites = append(s.st.Invites[:i], s.st.Invites[i+1:]...)
			return s.save()
		}
	}
	return nil
}

func (s *Store) dropExpired() {
	kept := s.st.Invites[:0]
	for _, inv := range s.st.Invites {
		if time.Now().Before(inv.Expires) {
			kept = append(kept, inv)
		}
	}
	s.st.Invites = kept
}

// ErrBadInvite — приглашение неверное, уже использовано или просрочено.
var ErrBadInvite = errors.New("приглашение недействительно")

// Join — агент пришёл с кодом приглашения. Приглашение сгорает (одноразовое),
// агент получает свой постоянный ключ. Возвращает id компьютера и ключ.
func (s *Store) Join(token, name, platform string) (id, key string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropExpired()
	for i, inv := range s.st.Invites {
		if equal(inv.TokenHash, hash(token)) {
			s.st.Invites = append(s.st.Invites[:i], s.st.Invites[i+1:]...)
			id, key = randomHex(6), randomHex(32)
			s.st.Machines = append(s.st.Machines, Machine{
				ID: id, Name: strings.TrimSpace(name), Platform: platform, KeyHash: hash(key), AddedAt: time.Now(),
			})
			return id, key, s.save()
		}
	}
	return "", "", ErrBadInvite
}

// Auth — по ключу агента найти его компьютер.
func (s *Store) Auth(id, key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(id)
	return i >= 0 && !s.st.Machines[i].Self && equal(s.st.Machines[i].KeyHash, hash(key))
}

// HasAgents — есть ли добавленные компьютеры или ждущие приглашения (тогда нужно слушать сеть).
func (s *Store) HasAgents() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropExpired()
	return len(s.st.Machines) > 1 || len(s.st.Invites) > 0
}

// randomHex — n случайных байт в виде текста. crypto/rand — «настоящая» случайность для секретов
// (обычный math/rand для этого не годится: его можно угадать).
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// hash — отпечаток SHA-256: по нему нельзя восстановить секрет, но можно проверить, совпадает ли.
func hash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// equal сравнивает за одинаковое время, чтобы по скорости ответа нельзя было подбирать ключ.
func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

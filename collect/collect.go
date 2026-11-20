// Пакет collect собирает данные о компьютере.
//
// Устройство простое: есть «сборщики» (Collector). Каждый умеет собрать свой кусочек
// (система, диски, батарея, ошибки железа) и положить его в общий Snapshot.
// Если сборщик сломался — это не катастрофа: ошибка попадает в Snapshot.Warnings,
// а остальные сборщики продолжают работу.
//
// Сборщики, которые зависят от операционной системы, лежат в файлах с суффиксом
// _windows.go (а для Mac потом появятся _darwin.go). Go сам берёт при сборке только
// файлы для нужной ОС — это называется build tags (метки сборки).
package collect

import (
	"fmt"
	"time"
)

// Snapshot — всё, что мы узнали о компьютере за один запуск.
// Метки `json:"..."` подсказывают, как называть поля при сохранении в JSON-файл.
type Snapshot struct {
	CollectedAt time.Time `json:"collected_at"`
	Demo        bool      `json:"demo"`     // true — данные выдуманные (флаг --demo)
	IsAdmin     bool      `json:"is_admin"` // запущено ли от администратора
	System      System    `json:"system"`
	Disks       Disks     `json:"disks"`
	Battery     *Battery  `json:"battery"`            // nil — не проверяли (например, не Windows)
	HWErrors    *HWErrors `json:"hw_errors"`          // nil — не проверяли
	Warnings    []string  `json:"warnings,omitempty"` // что не удалось собрать и почему
}

// Collector — «сборщик». Это интерфейс: описание того, что должен уметь любой сборщик.
// Любой тип, у которого есть методы Name() и Collect(), автоматически считается Collector.
type Collector interface {
	Name() string
	Collect(s *Snapshot) error
}

// All — список всех сборщиков для текущей ОС: общие + специфичные для платформы.
func All() []Collector {
	// «...» разворачивает список: добавить все элементы, а не список целиком
	return append([]Collector{systemCollector{}}, platformCollectors()...)
}

// Run запускает всех сборщиков по очереди и возвращает готовый снимок.
func Run() Snapshot {
	s := Snapshot{CollectedAt: time.Now(), IsAdmin: IsAdmin()}
	for _, c := range All() {
		if err := c.Collect(&s); err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("%s: %v", c.Name(), err))
		}
	}
	return s
}

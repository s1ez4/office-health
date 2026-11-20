// office-health — проверка «здоровья» компьютера: диски, батарея, ошибки железа.
// Запуск: office-health.exe  (лучше от администратора)
package main

import (
	"fmt"

	"office-health/collect"
)

// main — точка входа: отсюда программа начинает работать.
func main() {
	s := collect.Run()

	if !s.IsAdmin {
		fmt.Println("⚠ Программа запущена НЕ от администратора.")
		fmt.Println("  Часть данных (здоровье дисков, журнал ошибок) без прав администратора недоступна.")
		fmt.Println("  Нажмите на office-health.exe правой кнопкой → «Запуск от имени администратора».")
		fmt.Println()
	}

	sys := s.System
	fmt.Println("Компьютер:", sys.Hostname)
	fmt.Println("ОС:       ", sys.OS)
	fmt.Printf("Процессор: %s (%d потоков)\n", sys.CPU, sys.CPUCores)
	fmt.Printf("Память:    %.1f ГБ, занято %.0f%%\n", sys.RAMTotalGB, sys.RAMUsedPct)
	fmt.Printf("Работает без перезагрузки: %.1f ч\n", sys.UptimeHours)
	for _, v := range sys.Volumes {
		fmt.Printf("Раздел %s (%s): %.0f ГБ, свободно %.0f ГБ, занято %.0f%%\n",
			v.Mount, v.FS, v.TotalGB, v.FreeGB, v.UsedPct)
	}
	for _, w := range s.Warnings {
		fmt.Println("Не удалось:", w)
	}
}

package collect

// Какой физический диск — это C:, а какой — D:? smartctl знает про «железку» (модель, серийный номер),
// а Windows — про разделы с буквами. Склеиваем их по серийному номеру, а если не вышло — по модели.

import "strings"

// driveInfo — физический диск так, как его видит Windows (WMI), вместе с буквами его разделов.
type driveInfo struct {
	Index   int
	Model   string
	Serial  string
	Volumes []string // «C:», «D:»
}

// normSerial — серийные номера в разных источниках пишут по-разному: с пробелами, «_», точкой в конце.
func normSerial(s string) string {
	s = strings.ToUpper(s)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' || r == '-' || r == '.' {
			return -1 // -1 — «выбросить символ»
		}
		return r
	}, s)
}

// matchVolumes проставляет каждому диску буквы его разделов.
func matchVolumes(disks []Disk, drives []driveInfo) {
	used := make([]bool, len(drives))
	assign := func(i, j int) {
		disks[i].Volumes = drives[j].Volumes
		used[j] = true
	}
	try := func(same func(d Disk, w driveInfo) bool) {
		for i := range disks {
			if disks[i].Volumes != nil {
				continue
			}
			for j, w := range drives {
				if !used[j] && same(disks[i], w) {
					assign(i, j)
					break
				}
			}
		}
	}
	try(func(d Disk, w driveInfo) bool { // 1) серийный номер
		return d.Serial != "" && normSerial(d.Serial) == normSerial(w.Serial)
	})
	try(func(d Disk, w driveInfo) bool { // 2) модель
		dm, wm := strings.ToUpper(strings.TrimSpace(d.Model)), strings.ToUpper(strings.TrimSpace(w.Model))
		return dm != "" && wm != "" && (dm == wm || strings.Contains(wm, dm) || strings.Contains(dm, wm))
	})
	if len(disks) == 1 && len(drives) == 1 && disks[0].Volumes == nil { // 3) диск всего один — значит, он
		assign(0, 0)
	}
}

//go:build windows

package collect

import (
	"regexp"
	"strconv"

	"github.com/yusufpapurcu/wmi"
)

type win32DiskDrive struct {
	Index        uint32
	Model        string
	SerialNumber string
}

// Win32_LogicalDiskToPartition — «связка»: какой раздел диска стал какой буквой.
// Antecedent:  …Win32_DiskPartition.DeviceID="Disk #0, Partition #2"
// Dependent:   …Win32_LogicalDisk.DeviceID="C:"
type win32LogicalToPartition struct {
	Antecedent string
	Dependent  string
}

var (
	reDiskNum = regexp.MustCompile(`Disk #(\d+)`)
	reLetter  = regexp.MustCompile(`DeviceID="([A-Za-z]:)"`)
)

// attachVolumes — на Windows спрашиваем у WMI, какие буквы на каком физическом диске.
// Ошибку не возвращаем: без букв отчёт просто покажет «Диск 1».
func attachVolumes(disks []Disk) {
	var drives []win32DiskDrive
	if wmi.Query("SELECT Index, Model, SerialNumber FROM Win32_DiskDrive", &drives) != nil {
		return
	}
	var links []win32LogicalToPartition
	if wmi.Query("SELECT Antecedent, Dependent FROM Win32_LogicalDiskToPartition", &links) != nil {
		return
	}
	byIndex := map[int][]string{}
	for _, l := range links {
		n, letter := reDiskNum.FindStringSubmatch(l.Antecedent), reLetter.FindStringSubmatch(l.Dependent)
		if n != nil && letter != nil {
			idx, _ := strconv.Atoi(n[1])
			byIndex[idx] = append(byIndex[idx], letter[1])
		}
	}
	var infos []driveInfo
	for _, d := range drives {
		infos = append(infos, driveInfo{Index: int(d.Index), Model: d.Model, Serial: d.SerialNumber, Volumes: byIndex[int(d.Index)]})
	}
	matchVolumes(disks, infos)
}

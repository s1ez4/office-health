package collect

import (
	"reflect"
	"testing"
)

func TestMatchVolumes(t *testing.T) {
	disks := []Disk{
		{Model: "Samsung SSD 980 500GB", Serial: "S64DNX0R123456"},
		{Model: "WDC WD10EZEX-08WN4A0", Serial: "WD-WCC6Y0ABCDEF"},
	}
	drives := []driveInfo{
		{Index: 0, Model: "WDC WD10EZEX-08WN4A0", Serial: "     WD-WCC6Y0ABCDEF", Volumes: []string{"D:"}},  // серийник с пробелами
		{Index: 1, Model: "Samsung SSD 980 500GB", Serial: "0025_3881_1234_5678.", Volumes: []string{"C:"}}, // серийник другой — сработает модель
	}
	matchVolumes(disks, drives)
	if !reflect.DeepEqual(disks[0].Volumes, []string{"C:"}) || !reflect.DeepEqual(disks[1].Volumes, []string{"D:"}) {
		t.Errorf("буквы: %v %v", disks[0].Volumes, disks[1].Volumes)
	}

	// Один диск — один диск в Windows: сопоставляем, даже если ни серийник, ни модель не совпали
	one := []Disk{{Model: "X", Serial: "1"}}
	matchVolumes(one, []driveInfo{{Model: "Y", Serial: "2", Volumes: []string{"C:"}}})
	if len(one[0].Volumes) != 1 {
		t.Error("единственный диск остался без буквы")
	}
}

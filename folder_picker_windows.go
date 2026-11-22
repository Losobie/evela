package main

import "golang.org/x/sys/windows"

func folderPickerRoots() []string {
	mask, _ := windows.GetLogicalDrives()
	var roots []string
	for drive := range 26 {
		if mask&(1<<drive) != 0 {
			roots = append(roots, string(rune('A'+drive))+`:\`)
		}
	}
	return roots
}

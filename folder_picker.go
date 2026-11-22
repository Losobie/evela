package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func openFolderPicker(parent fyne.Window, selected func(string)) {
	if open := lastPopupWindow(parent); open != nil {
		open.Show()
		return
	}
	address := widget.NewEntry()
	address.SetPlaceHolder("Folder path")
	var current string
	var folders []string
	list := widget.NewList(func() int { return len(folders) }, func() fyne.CanvasObject {
		return widget.NewLabel("")
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		object.(*widget.Label).SetText(folders[id])
	})
	var picker *popupWindow
	report := func(err error) {
		reportRuntimeError("Browse log folders", err)
		showPopupError(err, picker)
	}
	browse := func(path string) {
		path, err := filepath.Abs(strings.TrimSpace(path))
		if err != nil {
			report(err)
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			report(err)
			return
		}
		current = path
		address.SetText(path)
		folders = nil
		for _, entry := range entries {
			if entry.IsDir() {
				folders = append(folders, entry.Name())
			} else if entry.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(filepath.Join(path, entry.Name())); err == nil && info.IsDir() {
					folders = append(folders, entry.Name())
				}
			}
		}
		list.Refresh()
		list.ScrollToTop()
	}
	list.OnSelected = func(id widget.ListItemID) {
		list.UnselectAll()
		browse(filepath.Join(current, folders[id]))
	}
	selectFolder := func() {
		path, err := filepath.Abs(strings.TrimSpace(address.Text))
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(path)
			if err == nil && !info.IsDir() {
				err = fmt.Errorf("%s is not a folder", path)
			}
		}
		if err != nil {
			report(err)
			return
		}
		picker.Close()
		selected(path)
	}
	address.OnSubmitted = func(path string) { browse(path) }
	up := widget.NewButton("Up", func() {
		if current != "" {
			browse(filepath.Dir(current))
		}
	})
	homePath, err := os.UserHomeDir()
	if err != nil {
		homePath = string(os.PathSeparator)
	}
	home := widget.NewButton("Home", func() { browse(homePath) })
	drives := widget.NewSelect(folderPickerRoots(), browse)
	drives.PlaceHolder = "Locations"
	newFolder := widget.NewButton("New folder…", func() {
		if current == "" {
			return
		}
		if open := lastPopupWindow(picker); open != nil {
			open.Show()
			return
		}
		directory := current
		name := widget.NewEntry()
		name.SetPlaceHolder("Folder name")
		form := container.NewVBox(name)
		create := newPopupWindow("New Folder", form, picker)
		makeFolder := func() {
			value := strings.TrimSpace(name.Text)
			if value == "" || value == "." || value == ".." || filepath.Base(value) != value || filepath.VolumeName(value) != "" {
				showPopupError(fmt.Errorf("enter a folder name"), create)
				return
			}
			if err := os.Mkdir(filepath.Join(directory, value), 0o750); err != nil {
				showPopupError(err, create)
				return
			}
			create.Close()
			browse(directory)
		}
		name.OnSubmitted = func(string) { makeFolder() }
		form.Add(widget.NewButton("Create folder", makeFolder))
		create.Resize(fyne.NewSize(380, 110))
		create.Show()
	})
	options := container.NewHBox(up, home, drives, newFolder)
	top := container.NewVBox(options, container.NewBorder(nil, nil, nil, widget.NewButton("Go", func() { browse(address.Text) }), address))
	body := container.NewBorder(top, widget.NewButton("Select folder", selectFolder), nil, nil, list)
	picker = newPopupWindow("Open Folder", body, parent)
	picker.Resize(fyne.NewSize(650, 460))
	picker.Show()
	browse(homePath)
}

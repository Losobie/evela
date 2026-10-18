package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type characterGroupRow struct {
	widget.BaseWidget
	label            *widget.Label
	onSelect, onMove func()
}

func newCharacterGroupRow() *characterGroupRow {
	row := &characterGroupRow{label: widget.NewLabel("Character")}
	row.label.Truncation = fyne.TextTruncateEllipsis
	row.ExtendBaseWidget(row)
	return row
}

func (row *characterGroupRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(row.label)
}
func (row *characterGroupRow) Tapped(*fyne.PointEvent) {
	if row.onSelect != nil {
		row.onSelect()
	}
}
func (row *characterGroupRow) DoubleTapped(*fyne.PointEvent) {
	if row.onMove != nil {
		row.onMove()
	}
}

type characterTransferLayout struct{}

func (characterTransferLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	left, buttons, right := objects[0].MinSize(), objects[1].MinSize(), objects[2].MinSize()
	return fyne.NewSize(left.Width+buttons.Width+right.Width+2*theme.Padding(), max(left.Height, buttons.Height, right.Height))
}
func (characterTransferLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	buttons := objects[1].MinSize()
	width := max(float32(1), (size.Width-buttons.Width-2*theme.Padding())/2)
	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(width, size.Height))
	objects[1].Move(fyne.NewPos(width+theme.Padding(), max(float32(0), (size.Height-buttons.Height)/2)))
	objects[1].Resize(buttons)
	objects[2].Move(fyne.NewPos(width+buttons.Width+2*theme.Padding(), 0))
	objects[2].Resize(fyne.NewSize(width, size.Height))
}

type characterGroupEditor struct {
	ui                              *analyzerUI
	dialog                          *dialog.CustomDialog
	choice                          *widget.Select
	name                            *widget.Entry
	status, leftTitle, rightTitle   *widget.Label
	leftList, rightList             *widget.List
	addButton, removeButton         *widget.Button
	saveButton, deleteButton        *widget.Button
	id, selectedLeft, selectedRight string
	members, removed                map[string]string
	available, included             []string
	loading                         bool
}

func (ui *analyzerUI) openCharacterGroupSettings() {
	if ui.characterGroupEditor != nil {
		ui.characterGroupEditor.dialog.Show()
		return
	}
	editor := &characterGroupEditor{ui: ui, members: map[string]string{}, removed: map[string]string{}}
	ui.characterGroupEditor = editor
	editor.name = widget.NewEntry()
	editor.name.SetPlaceHolder("Group name")
	editor.name.Validator = func(value string) error { return ui.validateCharacterGroupName(editor.id, value) }
	editor.status = widget.NewLabel("")
	editor.status.Wrapping = fyne.TextWrapWord
	editor.leftTitle, editor.rightTitle = widget.NewLabel(""), widget.NewLabel("")
	editor.addButton = widget.NewButtonWithIcon("Add", theme.NavigateNextIcon(), func() { editor.add(editor.selectedLeft) })
	editor.removeButton = widget.NewButtonWithIcon("Remove", theme.NavigateBackIcon(), func() { editor.remove(editor.selectedRight) })
	editor.saveButton = widget.NewButton("Save group", editor.save)
	editor.deleteButton = widget.NewButton("Delete group", editor.delete)
	editor.leftList = widget.NewList(func() int { return len(editor.available) }, func() fyne.CanvasObject { return newCharacterGroupRow() }, func(index widget.ListItemID, object fyne.CanvasObject) {
		row := object.(*characterGroupRow)
		name := editor.available[index]
		row.label.SetText(name)
		row.onSelect = func() { editor.leftList.Select(index) }
		row.onMove = func() { editor.add(name) }
	})
	editor.rightList = widget.NewList(func() int { return len(editor.included) }, func() fyne.CanvasObject { return newCharacterGroupRow() }, func(index widget.ListItemID, object fyne.CanvasObject) {
		row := object.(*characterGroupRow)
		name := editor.included[index]
		row.label.SetText(name)
		row.onSelect = func() { editor.rightList.Select(index) }
		row.onMove = func() { editor.remove(name) }
	})
	editor.leftList.OnSelected = func(index widget.ListItemID) {
		if index >= 0 && index < len(editor.available) {
			editor.selectedLeft = editor.available[index]
			editor.addButton.Enable()
		}
	}
	editor.rightList.OnSelected = func(index widget.ListItemID) {
		if index >= 0 && index < len(editor.included) {
			editor.selectedRight = editor.included[index]
			editor.removeButton.Enable()
		}
	}
	editor.leftList.OnUnselected = func(widget.ListItemID) {
		editor.selectedLeft = ""
		editor.addButton.Disable()
	}
	editor.rightList.OnUnselected = func(widget.ListItemID) {
		editor.selectedRight = ""
		editor.removeButton.Disable()
	}
	editor.choice = widget.NewSelect(nil, func(value string) {
		if editor.loading {
			return
		}
		id := ""
		for _, group := range ui.characterGroups {
			if value == "Edit: "+group.Name {
				id = group.ID
			}
		}
		editor.loadDraft(id)
	})
	editor.name.OnChanged = func(string) { editor.validateName() }
	note := widget.NewLabel("Select a character and use the arrows, or double-click a character to move it. Available characters have a log entry in the current time window or were removed during this settings session. Existing members remain listed even outside the window.")
	note.Wrapping = fyne.TextWrapWord
	top := container.NewVBox(editor.choice, widget.NewForm(widget.NewFormItem("Group name", editor.name)), note)
	left := container.NewBorder(editor.leftTitle, nil, nil, nil, editor.leftList)
	right := container.NewBorder(editor.rightTitle, nil, nil, nil, editor.rightList)
	panels := container.New(characterTransferLayout{}, left, container.NewVBox(editor.addButton, editor.removeButton), right)
	saveNote := widget.NewLabel("Save before switching groups. Closing discards unsaved edits. Characters may belong to more than one group.")
	saveNote.Wrapping = fyne.TextWrapWord
	bottom := container.NewVBox(editor.status, container.NewGridWithColumns(2, editor.saveButton, editor.deleteButton), saveNote)
	editor.dialog = dialog.NewCustom("Character groups", "Close", container.NewBorder(top, bottom, nil, nil, panels), ui.window)
	editor.dialog.SetOnClosed(func() { ui.characterGroupEditor = nil })
	editor.dialog.Resize(fyne.NewSize(900, 680))
	editor.loadDraft("")
	editor.dialog.Show()
}

func (editor *characterGroupEditor) refreshChoice() {
	editor.loading = true
	editor.choice.Options = []string{"New group…"}
	value := "New group…"
	for _, group := range editor.ui.characterGroups {
		option := "Edit: " + group.Name
		editor.choice.Options = append(editor.choice.Options, option)
		if editor.id == group.ID {
			value = option
		}
	}
	editor.choice.SetSelected(value)
	editor.choice.Refresh()
	editor.loading = false
}

func (editor *characterGroupEditor) loadDraft(id string) {
	editor.id, editor.members = id, map[string]string{}
	name := ""
	for _, group := range editor.ui.characterGroups {
		if group.ID == id {
			name = group.Name
			for _, member := range group.Members {
				editor.members[characterKey(member)] = member
			}
		}
	}
	editor.name.SetText(name)
	editor.refreshChoice()
	editor.refreshLists()
	editor.validateName()
	if id == "" {
		editor.deleteButton.Disable()
	} else {
		editor.deleteButton.Enable()
	}
}

func (editor *characterGroupEditor) validateName() {
	if err := editor.ui.validateCharacterGroupName(editor.id, editor.name.Text); err != nil {
		editor.saveButton.Disable()
		editor.status.SetText(err.Error())
	} else {
		editor.saveButton.Enable()
		editor.status.SetText("")
	}
}

func (editor *characterGroupEditor) refreshLists() {
	selectedLeft, selectedRight := editor.selectedLeft, editor.selectedRight
	available := charactersWithLogEntries(editor.ui.snapshot, editor.ui.view.Start, editor.ui.view.End)
	for key, name := range editor.removed {
		available[key] = name
	}
	for key := range editor.members {
		delete(available, key)
	}
	editor.available, editor.included = sortedCharacterNames(available), sortedCharacterNames(editor.members)
	editor.selectedLeft, editor.selectedRight = "", ""
	editor.leftList.UnselectAll()
	editor.rightList.UnselectAll()
	editor.addButton.Disable()
	editor.removeButton.Disable()
	editor.leftTitle.SetText(fmt.Sprintf("Available characters (%d)", len(editor.available)))
	editor.rightTitle.SetText(fmt.Sprintf("Group members (%d)", len(editor.included)))
	editor.leftList.Refresh()
	editor.rightList.Refresh()
	for index, name := range editor.available {
		if name == selectedLeft {
			editor.leftList.Select(index)
		}
	}
	for index, name := range editor.included {
		if name == selectedRight {
			editor.rightList.Select(index)
		}
	}
}

func (editor *characterGroupEditor) add(name string) {
	for _, available := range editor.available {
		if characterKey(name) == characterKey(available) {
			editor.members[characterKey(available)] = available
			break
		}
	}
	editor.refreshLists()
}

func (editor *characterGroupEditor) remove(name string) {
	key := characterKey(name)
	if member, ok := editor.members[key]; ok {
		editor.removed[key] = member
		delete(editor.members, key)
	}
	editor.refreshLists()
}

func (editor *characterGroupEditor) save() {
	group, err := editor.ui.saveCharacterGroup(characterGroup{ID: editor.id, Name: editor.name.Text, Members: sortedCharacterNames(editor.members)})
	if err != nil {
		editor.status.SetText(err.Error())
		return
	}
	editor.loadDraft(group.ID)
	editor.status.SetText("Saved " + group.Name + ". Choose New group… to create another.")
}

func (editor *characterGroupEditor) delete() {
	if editor.id == "" {
		return
	}
	for key, name := range editor.members {
		editor.removed[key] = name
	}
	editor.ui.deleteCharacterGroup(editor.id)
	editor.loadDraft("")
	editor.status.SetText("Group deleted.")
}

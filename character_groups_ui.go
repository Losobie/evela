package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type characterGroupRow struct {
	widget.BaseWidget
	label         *widget.Label
	background    *canvas.Rectangle
	mouseSelected bool
	onSelect      func(fyne.KeyModifier)
	onMove        func()
}

func newCharacterGroupRow() *characterGroupRow {
	row := &characterGroupRow{label: widget.NewLabel("Character"), background: canvas.NewRectangle(theme.Color(theme.ColorNameSelection))}
	row.background.Hide()
	row.label.Truncation = fyne.TextTruncateEllipsis
	row.ExtendBaseWidget(row)
	return row
}

func (row *characterGroupRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(row.background, row.label))
}
func (row *characterGroupRow) setSelected(selected bool) {
	row.background.FillColor = row.Theme().Color(theme.ColorNameSelection, fyne.CurrentApp().Settings().ThemeVariant())
	row.background.CornerRadius = row.Theme().Size(theme.SizeNameSelectionRadius)
	if selected {
		row.background.Show()
	} else {
		row.background.Hide()
	}
	row.background.Refresh()
}
func (row *characterGroupRow) MouseDown(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		// Fyne delays Tapped on double-tappable widgets until the double-click
		// timeout. Select immediately and consume that later tap so Ctrl-click
		// does not toggle the same character twice.
		row.mouseSelected = true
		if row.onSelect != nil {
			row.onSelect(event.Modifier)
		}
	}
}
func (row *characterGroupRow) MouseUp(*desktop.MouseEvent) {}
func (row *characterGroupRow) Tapped(*fyne.PointEvent) {
	if row.mouseSelected {
		row.mouseSelected = false
		return
	}
	if row.onSelect != nil {
		row.onSelect(0)
	}
}
func (row *characterGroupRow) DoubleTapped(*fyne.PointEvent) {
	row.mouseSelected = false
	if row.onMove != nil {
		row.onMove()
	}
}

type characterGroupSelection struct {
	names  map[string]bool
	anchor string
}

func (selection *characterGroupSelection) selectAt(names []string, index int, modifiers fyne.KeyModifier) {
	if index < 0 || index >= len(names) {
		return
	}
	control := modifiers&fyne.KeyModifierControl != 0
	shift := modifiers&fyne.KeyModifierShift != 0
	anchor := -1
	for i, name := range names {
		if name == selection.anchor {
			anchor = i
			break
		}
	}
	if !control || selection.names == nil {
		selection.names = map[string]bool{}
	}
	if shift && anchor >= 0 {
		for i := min(anchor, index); i <= max(anchor, index); i++ {
			selection.names[names[i]] = true
		}
		return
	}
	name := names[index]
	if control && !shift && selection.names[name] {
		delete(selection.names, name)
	} else {
		selection.names[name] = true
	}
	selection.anchor = name
}

func (selection *characterGroupSelection) retain(names []string) {
	retained := map[string]bool{}
	anchorPresent := false
	for _, name := range names {
		if selection.names[name] {
			retained[name] = true
		}
		anchorPresent = anchorPresent || name == selection.anchor
	}
	selection.names = retained
	if !anchorPresent {
		selection.anchor = ""
	}
}

func (selection *characterGroupSelection) selectedNames(names []string) []string {
	var selected []string
	for _, name := range names {
		if selection.names[name] {
			selected = append(selected, name)
		}
	}
	return selected
}

func characterGroupKeyModifiers() fyne.KeyModifier {
	if driver, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		return driver.CurrentKeyModifiers()
	}
	return 0
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
	ui                            *analyzerUI
	dialog                        *dialog.CustomDialog
	choice                        *widget.Select
	name                          *widget.Entry
	status, leftTitle, rightTitle *widget.Label
	leftList, rightList           *widget.List
	addButton, removeButton       *widget.Button
	saveButton, deleteButton      *widget.Button
	id                            string
	leftSelection, rightSelection characterGroupSelection
	members, removed              map[string]string
	available, included           []string
	loading                       bool
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
	editor.addButton = widget.NewButtonWithIcon("Add", theme.NavigateNextIcon(), func() { editor.add(editor.leftSelection.selectedNames(editor.available)...) })
	editor.removeButton = widget.NewButtonWithIcon("Remove", theme.NavigateBackIcon(), func() { editor.remove(editor.rightSelection.selectedNames(editor.included)...) })
	editor.saveButton = widget.NewButton("Save group", editor.save)
	editor.deleteButton = widget.NewButton("Delete group", editor.delete)
	editor.leftList = widget.NewList(func() int { return len(editor.available) }, func() fyne.CanvasObject { return newCharacterGroupRow() }, func(index widget.ListItemID, object fyne.CanvasObject) {
		row := object.(*characterGroupRow)
		name := editor.available[index]
		row.label.SetText(name)
		row.setSelected(editor.leftSelection.names[name])
		row.onSelect = func(modifiers fyne.KeyModifier) { editor.selectCharacter(true, index, modifiers) }
		row.onMove = func() { editor.add(name) }
	})
	editor.rightList = widget.NewList(func() int { return len(editor.included) }, func() fyne.CanvasObject { return newCharacterGroupRow() }, func(index widget.ListItemID, object fyne.CanvasObject) {
		row := object.(*characterGroupRow)
		name := editor.included[index]
		row.label.SetText(name)
		row.setSelected(editor.rightSelection.names[name])
		row.onSelect = func(modifiers fyne.KeyModifier) { editor.selectCharacter(false, index, modifiers) }
		row.onMove = func() { editor.remove(name) }
	})
	editor.leftList.OnSelected = func(index widget.ListItemID) {
		editor.leftList.UnselectAll()
		editor.selectCharacter(true, index, characterGroupKeyModifiers())
	}
	editor.rightList.OnSelected = func(index widget.ListItemID) {
		editor.rightList.UnselectAll()
		editor.selectCharacter(false, index, characterGroupKeyModifiers())
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
	note := widget.NewLabel("Select characters and use the arrows. Ctrl-click toggles individual characters; Shift-click selects a range. Double-click a character to move it. Available characters have a log entry in the current time window or were removed during this settings session. Existing members remain listed even outside the window.")
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
	editor.leftSelection, editor.rightSelection = characterGroupSelection{}, characterGroupSelection{}
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
	available := charactersWithLogEntries(editor.ui.snapshot, editor.ui.view.Start, editor.ui.view.End)
	for key, name := range editor.removed {
		available[key] = name
	}
	for key := range editor.members {
		delete(available, key)
	}
	editor.available, editor.included = sortedCharacterNames(available), sortedCharacterNames(editor.members)
	editor.leftSelection.retain(editor.available)
	editor.rightSelection.retain(editor.included)
	editor.leftList.UnselectAll()
	editor.rightList.UnselectAll()
	editor.updateTransferButtons()
	editor.leftTitle.SetText(fmt.Sprintf("Available characters (%d)", len(editor.available)))
	editor.rightTitle.SetText(fmt.Sprintf("Group members (%d)", len(editor.included)))
	editor.leftList.Refresh()
	editor.rightList.Refresh()
}

func (editor *characterGroupEditor) selectCharacter(left bool, index int, modifiers fyne.KeyModifier) {
	if left {
		editor.leftSelection.selectAt(editor.available, index, modifiers)
		editor.leftList.Refresh()
	} else {
		editor.rightSelection.selectAt(editor.included, index, modifiers)
		editor.rightList.Refresh()
	}
	editor.updateTransferButtons()
}

func (editor *characterGroupEditor) updateTransferButtons() {
	if len(editor.leftSelection.names) > 0 {
		editor.addButton.Enable()
	} else {
		editor.addButton.Disable()
	}
	if len(editor.rightSelection.names) > 0 {
		editor.removeButton.Enable()
	} else {
		editor.removeButton.Disable()
	}
}

func (editor *characterGroupEditor) add(names ...string) {
	for _, name := range names {
		for _, available := range editor.available {
			if characterKey(name) == characterKey(available) {
				editor.members[characterKey(available)] = available
				break
			}
		}
	}
	editor.refreshLists()
}

func (editor *characterGroupEditor) remove(names ...string) {
	for _, name := range names {
		key := characterKey(name)
		if member, ok := editor.members[key]; ok {
			editor.removed[key] = member
			delete(editor.members, key)
		}
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

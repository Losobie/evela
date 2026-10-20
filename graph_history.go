package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type graphRangeState struct {
	Start, End time.Time
	Preset     string
}

type graphRangeChange struct {
	Before, After graphRangeState
	wheel         bool
	changedAt     time.Time
}

const graphWheelGroupInterval = 600 * time.Millisecond

func (ui *analyzerUI) initGraphRangeShortcuts() {
	ui.window.Canvas().AddShortcut(&fyne.ShortcutUndo{}, func(fyne.Shortcut) {
		ui.navigateGraphRange(false)
	})
	ui.window.Canvas().AddShortcut(&fyne.ShortcutRedo{}, func(fyne.Shortcut) {
		ui.navigateGraphRange(true)
	})
}

func (ui *analyzerUI) navigateGraphRange(redo bool) {
	if !ui.graphRangeShortcutsAvailable() {
		return
	}
	if redo {
		ui.redoGraphRange()
	} else {
		ui.undoGraphRange()
	}
}

func (ui *analyzerUI) graphRangeShortcutsAvailable() bool {
	// Text entries keep their own undo/redo. Dialogs and popup menus should
	// not change the graphs behind them when these keys are pressed.
	editing := false
	switch ui.window.Canvas().Focused().(type) {
	case *widget.Entry, *dateTimePicker:
		editing = true
	}
	return !editing && ui.window.Canvas().Overlays().Top() == nil
}

func (ui *analyzerUI) clearGraphRangeHistory() { ui.graphUndo, ui.graphRedo = nil, nil }

func (ui *analyzerUI) panGraphRange(forward bool) {
	width := ui.view.End.Sub(ui.view.Start)
	if width <= 0 {
		return
	}
	// UTC fields and logs resolve to whole seconds. Round symmetrically so
	// opposite button presses return to exactly the same range at any width.
	shift := (width/4 + time.Second/2).Truncate(time.Second)
	if shift < time.Second {
		shift = time.Second
	}
	if !forward {
		shift = -shift
	}
	ui.applyGraphRange(ui.view.Start.Add(shift), ui.view.End.Add(shift))
}

func (ui *analyzerUI) applyGraphRange(start, end time.Time) {
	ui.applyGraphRangeAction(start, end, false, time.Time{})
}

func (ui *analyzerUI) applyGraphWheelRange(start, end time.Time) {
	if ui.window.Canvas().Overlays().Top() != nil {
		return
	}
	ui.applyGraphRangeAction(start, end, true, time.Now())
}

func (ui *analyzerUI) applyGraphRangeAction(start, end time.Time, wheel bool, at time.Time) {
	if !wheel {
		ui.finishGraphWheelBurst()
	}
	start, end = start.UTC().Truncate(time.Second), end.UTC().Truncate(time.Second)
	if !end.After(start) || (start.Equal(ui.view.Start) && end.Equal(ui.view.End)) {
		return
	}
	before := graphRangeState{Start: ui.view.Start, End: ui.view.End, Preset: ui.rangeSelect.Selected}
	after := graphRangeState{Start: start, End: end, Preset: "Custom"}
	if before.End.After(before.Start) {
		merged := false
		if wheel && len(ui.graphUndo) > 0 && len(ui.graphRedo) == 0 {
			last := &ui.graphUndo[len(ui.graphUndo)-1]
			gap := at.Sub(last.changedAt)
			if last.wheel && !last.changedAt.IsZero() && gap >= 0 && gap <= graphWheelGroupInterval && last.After == before {
				last.After, last.changedAt = after, at
				merged = true
			}
		}
		if !merged {
			ui.graphUndo = append(ui.graphUndo, graphRangeChange{Before: before, After: after, wheel: wheel, changedAt: at})
		}
	}
	ui.graphRedo = nil
	ui.restoreGraphRange(after)
	ui.window.Canvas().Unfocus()
}

func (ui *analyzerUI) finishGraphWheelBurst() {
	if len(ui.graphUndo) > 0 {
		ui.graphUndo[len(ui.graphUndo)-1].changedAt = time.Time{}
	}
}

func (ui *analyzerUI) restoreGraphRange(state graphRangeState) {
	ui.settingRange = true
	ui.start.SetText(state.Start.UTC().Format("2006-01-02 15:04:05"))
	ui.end.SetText(state.End.UTC().Format("2006-01-02 15:04:05"))
	ui.settingRange = false
	ui.end.Validate()
	ui.rangeSelect.Selected = state.Preset
	ui.rangeSelect.Refresh()
	// Restore the exact previous bounds. A restored relative preset resumes
	// following new log data on the next normal refresh.
	ui.rebuildView()
}

func (ui *analyzerUI) undoGraphRange() {
	ui.finishGraphWheelBurst()
	if len(ui.graphUndo) == 0 {
		return
	}
	change := ui.graphUndo[len(ui.graphUndo)-1]
	ui.graphUndo = ui.graphUndo[:len(ui.graphUndo)-1]
	ui.graphRedo = append(ui.graphRedo, change)
	ui.restoreGraphRange(change.Before)
}

func (ui *analyzerUI) redoGraphRange() {
	ui.finishGraphWheelBurst()
	if len(ui.graphRedo) == 0 {
		return
	}
	change := ui.graphRedo[len(ui.graphRedo)-1]
	change.changedAt = time.Time{}
	ui.graphRedo = ui.graphRedo[:len(ui.graphRedo)-1]
	ui.graphUndo = append(ui.graphUndo, change)
	ui.restoreGraphRange(change.After)
}

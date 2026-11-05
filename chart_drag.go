package main

import (
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (chart *damageChart) beginRangeDrag(position fyne.Position) bool {
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	if width <= 0 || position.X < chartPlotLeft || position.X > chartPlotLeft+width || position.Y < chartPlotTop || position.Y > chart.Size().Height-chartPlotBottom {
		return false
	}
	chart.mu.RLock()
	start, end := chart.data.Start, chart.data.End
	chart.mu.RUnlock()
	if !end.After(start) {
		return false
	}
	x := position.X - chartPlotLeft
	chart.drag = &chartRangeDrag{Start: start, End: end, Width: width, Anchor: x, Current: x}
	if chart.hoverGroup != nil {
		chart.hoverGroup.leave(chart)
	}
	return true
}

func (chart *damageChart) MouseDown(event *desktop.MouseEvent) {
	chart.pan = nil
	chart.cancelRangeDrag()
	if event.Button == desktop.MouseButtonSecondary {
		chart.dragBlocked = true
		chart.beginPan(event.Position)
		return
	}
	chart.dragBlocked = event.Button != desktop.MouseButtonPrimary
	if !chart.dragBlocked {
		chart.dragBlocked = !chart.beginRangeDrag(event.Position)
		if !chart.dragBlocked {
			if canvas := fyne.CurrentApp().Driver().CanvasForObject(chart); canvas != nil {
				canvas.Unfocus()
			}
		}
	}
}

func (chart *damageChart) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonSecondary {
		chart.updatePan(event.Position)
		chart.pan = nil
	}
	if event.Button == desktop.MouseButtonPrimary {
		click := chart.drag != nil && !chart.drag.Moved && !chart.dragBlocked
		// Some drivers send MouseUp before DragEnd; GLFW sends it afterwards.
		// Finishing clears the gesture, so either order applies the range once.
		if chart.drag != nil && chart.drag.Moved {
			chart.drag.Current = max(float32(0), min(chart.drag.Width, event.Position.X-chartPlotLeft))
			chart.drag.Moved = math.Abs(float64(chart.drag.Current-chart.drag.Anchor)) >= float64(chartDragThreshold)
		}
		chart.DragEnd()
		if click {
			chart.selectSeriesAt(event.Position)
		}
	}
	chart.dragBlocked = false
}

func (chart *damageChart) Dragged(event *fyne.DragEvent) {
	if chart.pan != nil {
		chart.updatePan(event.Position)
		return
	}
	if chart.dragBlocked {
		return
	}
	if chart.drag == nil {
		// Touch and programmatic drag events do not have a preceding MouseDown.
		origin := fyne.NewPos(event.Position.X-event.Dragged.DX, event.Position.Y-event.Dragged.DY)
		if !chart.beginRangeDrag(origin) {
			chart.dragBlocked = true
			return
		}
	}
	chart.drag.Current = max(float32(0), min(chart.drag.Width, event.Position.X-chartPlotLeft))
	chart.drag.Moved = math.Abs(float64(chart.drag.Current-chart.drag.Anchor)) >= float64(chartDragThreshold)
	chart.updateRangeSelection()
}

func (drag *chartRangeDrag) selectedRange() (time.Time, time.Time) {
	left, right := min(drag.Anchor, drag.Current), max(drag.Anchor, drag.Current)
	duration := float64(drag.End.Sub(drag.Start))
	start := drag.Start.Add(time.Duration(float64(left) / float64(drag.Width) * duration)).Truncate(time.Second)
	rawEnd := drag.Start.Add(time.Duration(float64(right) / float64(drag.Width) * duration))
	end := rawEnd.Truncate(time.Second)
	if end.Before(rawEnd) {
		end = end.Add(time.Second)
	}
	if start.Before(drag.Start) {
		start = drag.Start
	}
	if end.After(drag.End) {
		end = drag.End
	}
	return start, end
}

func selectionDuration(duration time.Duration) string {
	unit := func(count int64, name string) string {
		if count != 1 {
			name += "s"
		}
		return fmt.Sprintf("%d %s", count, name)
	}
	hours, minutes := int64(duration/time.Hour), int64(duration/time.Minute)%60
	if hours > 0 {
		label := unit(hours, "hour")
		if minutes > 0 {
			label += " " + unit(minutes, "minute")
		}
		return label
	}
	if minutes > 0 {
		return unit(minutes, "minute")
	}
	return unit(int64(duration/time.Second), "second")
}

func (chart *damageChart) updateRangeSelection() {
	if chart.selection == nil || chart.drag == nil {
		return
	}
	drag := chart.drag
	chart.cursor.Hide()
	if !drag.Moved {
		chart.selection.Hide()
		return
	}
	left, right := min(drag.Anchor, drag.Current), max(drag.Anchor, drag.Current)
	chart.selection.Move(fyne.NewPos(chartPlotLeft+left, chartPlotTop))
	chart.selection.Resize(fyne.NewSize(right-left, max(float32(1), chart.Size().Height-chartPlotTop-chartPlotBottom)))
	chart.selection.Show()
	start, end := drag.selectedRange()
	chart.setHoverHint("Select " + start.UTC().Format("Jan 02 15:04:05") + " – " + end.UTC().Format("Jan 02 15:04:05") + " EVE time · (" + selectionDuration(end.Sub(start)) + ") · release to zoom")
}

func (chart *damageChart) cancelRangeDrag() {
	chart.drag = nil
	if chart.selection != nil {
		chart.selection.Hide()
	}
	chart.setHoverHint(chartHint)
}

func (chart *damageChart) DragEnd() {
	if chart.pan != nil {
		chart.pan = nil
		chart.dragBlocked = false
		return
	}
	drag := chart.drag
	chart.cancelRangeDrag()
	chart.dragBlocked = false
	if drag == nil || !drag.Moved || chart.OnRangeSelected == nil {
		return
	}
	start, end := drag.selectedRange()
	if end.After(start) {
		chart.OnRangeSelected(start, end)
	}
}

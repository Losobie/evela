package main

import (
	"cmp"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type totalsCell struct {
	widget.BaseWidget
	label *widget.Label
	icon  *widget.Icon
}

func newTotalsCell() *totalsCell {
	cell := &totalsCell{label: widget.NewLabel("Character name"), icon: widget.NewIcon(nil)}
	cell.icon.Hide()
	cell.ExtendBaseWidget(cell)
	return cell
}

func (cell *totalsCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, cell.icon, nil, cell.label))
}

func (ui *analyzerUI) sortTotalsByColumn(column int) {
	if column < 0 || column >= ui.totalsColumnCount() {
		return
	}
	if ui.totalsSortColumn == column {
		ui.totalsSortDescending = !ui.totalsSortDescending
	} else {
		ui.totalsSortColumn = column
		ui.totalsSortDescending = column >= 2 || (column == 1 && ui.anyGrouping())
	}
	ui.sortDamageTotals()
	ui.totals.Refresh()
}

func compareTotalsText(left, right string) int {
	if order := cmp.Compare(strings.ToLower(left), strings.ToLower(right)); order != 0 {
		return order
	}
	return cmp.Compare(left, right)
}

func (ui *analyzerUI) sortDamageTotals() {
	sort.SliceStable(ui.tableSeries, func(i, j int) bool {
		left, right := ui.tableSeries[i], ui.tableSeries[j]
		var order int
		switch ui.totalsSortColumn {
		case 0:
			order = compareTotalsText(left.Name, right.Name)
		case 1:
			if ui.anyGrouping() {
				order = cmp.Compare(len(left.Members), len(right.Members))
			} else {
				order = compareTotalsText(shipLabel(left.Name, ui.ships), shipLabel(right.Name, ui.ships))
			}
		case 2, 4:
			// Every row uses the same range duration, so total damage and
			// average DPS have the same numeric order, without display rounding.
			order = cmp.Compare(left.TotalOutgoing, right.TotalOutgoing)
		case 3, 5:
			order = cmp.Compare(left.TotalIncoming, right.TotalIncoming)
		case 6:
			order = cmp.Compare(left.OutgoingHits, right.OutgoingHits)
			if order == 0 {
				order = cmp.Compare(left.IncomingHits, right.IncomingHits)
			}
		case 7, 8, 9, 10:
			order = cmp.Compare(miningColumnValue(left, ui.totalsSortColumn), miningColumnValue(right, ui.totalsSortColumn))
		}
		if order == 0 {
			// Keep ties deterministic as new logs and ESI snapshots arrive.
			return compareTotalsText(left.Name, right.Name) < 0
		}
		if ui.totalsSortDescending {
			return order > 0
		}
		return order < 0
	})
}

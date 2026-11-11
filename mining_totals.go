package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type miningAmounts struct {
	Units, Volume, CritUnits, CritVolume float64
	UnknownVolume, UnknownCritVolume     bool
}

func miningEventVolume(event miningEvent) (float64, bool) {
	ore := event.Ore
	if event.Kind == miningCompressed && !strings.HasPrefix(strings.ToLower(ore), "compressed ") {
		ore = "Compressed " + ore
	}
	volume, ok := oreVolume(ore)
	return event.Amount * volume, ok
}

func (total *miningAmounts) add(event miningEvent) {
	if event.Kind != miningMined {
		return
	}
	volume, known := miningEventVolume(event)
	total.Units += event.Amount
	total.Volume += volume
	total.UnknownVolume = total.UnknownVolume || !known && event.Amount > 0
	if event.Critical {
		total.CritUnits += event.Amount
		total.CritVolume += volume
		total.UnknownCritVolume = total.UnknownCritVolume || !known && event.Amount > 0
	}
}

var miningNumberPrinter = message.NewPrinter(language.English)

func formatMiningVolume(volume float64, unknown bool) string {
	if unknown {
		return "Unknown"
	}
	return strings.TrimRight(strings.TrimRight(miningNumberPrinter.Sprintf("%.2f", volume), "0"), ".")
}

func (ui *analyzerUI) totalsColumnCount() int {
	if ui.hasMining {
		return 11
	}
	return 7
}

func miningTotalsCell(series characterSeries, column int) string {
	total := series.Mining
	switch column {
	case 0:
		return series.Name
	case 7:
		return fmt.Sprintf("%.0f", total.Units)
	case 8:
		return formatMiningVolume(total.Volume, total.UnknownVolume)
	case 9:
		return fmt.Sprintf("%.0f", total.CritUnits)
	case 10:
		return formatMiningVolume(total.CritVolume, total.UnknownCritVolume)
	default:
		return ""
	}
}

func miningColumnValue(series characterSeries, column int) float64 {
	if series.Mining == nil {
		return 0
	}
	switch column {
	case 7:
		return series.Mining.Units
	case 8:
		return series.Mining.Volume
	case 9:
		return series.Mining.CritUnits
	case 10:
		return series.Mining.CritVolume
	default:
		return 0
	}
}

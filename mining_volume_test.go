package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func newMiningVolumeTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	ui := newMiningTestUI(t)
	log, err := parseLog(strings.NewReader("Listener: Miner\n" +
		"[ 2026.11.11 02:00:00 ] (mining) You mined 100 units of Loparite with a lost residue of 3 units\n" +
		"[ 2026.11.11 02:00:01 ] (mining) Critical mining success! You mined an additional 156 units of Loparite\n" +
		"[ 2026.11.11 02:00:02 ] (mining) You mined 10 units of Veldspar\n" +
		"[ 2026.11.11 02:00:03 ] (notify) Successfully compressed Loparite into 20 Compressed Loparite.\n" +
		"[ 2026.11.11 02:00:03 ] (combat) 200 to Enemy - Laser - Hits\n"))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{SourceFiles: map[string]cachedLog{"mining.txt": {Log: log}}}
	ui.snapshot.populateLogs()
	ui.selectQuickRange("All history")
	return ui, time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC)
}

func TestMiningVolumeMenuAndGraph(t *testing.T) {
	ui, start := newMiningVolumeTestUI(t)
	menu := ui.eventGroupingMenu(graphMining)
	last := len(menu.Items) - 1
	if !menu.Items[last-2].IsSeparator || menu.Items[last-1].Label != "Units" || !menu.Items[last-1].Checked || menu.Items[last].Label != "Volume" || menu.Items[last].Checked {
		t.Fatal("mining units and volume need an independent menu section")
	}
	chooseGraphMenuItem(t, menu, "Volume")
	if ui.mining.data.HitUnit != "m3" || ui.mining.data.RateUnit != "m3/s" || ui.eventGraphGroupings[graphMining] != eventByCharacter {
		t.Fatal("volume did not update units or changed grouping")
	}
	mined := findLogisticsSeries(t, ui.mining, "Miner · Ore mined")
	if len(mined.Hits) != 3 || mined.Hits[0].Damage != 1000 || mined.Hits[1].Damage != 1560 || mined.Hits[2].Damage != 1 {
		t.Fatalf("ore-specific volume conversion failed: %+v", mined.Hits)
	}
	if findLogisticsSeries(t, ui.mining, "Miner · Compressed").Hits[0].Damage != 2 || findLogisticsSeries(t, ui.mining, "Miner · Waste").Hits[0].Damage != 30 {
		t.Fatal("compression must use compressed volume and waste raw ore volume")
	}
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByCriticals)
	if findLogisticsSeries(t, ui.mining, "Critical mining").Hits[0].Damage != 1560 {
		t.Fatal("critical volume lost")
	}
	ui.graphAverageWindow = 10 * time.Second
	ui.applyGraphRange(start, start.Add(4*time.Second))
	setTestGraphBucket(ui, time.Second)
	if ui.mining.data.AverageWindow != 10*time.Second || findLogisticsSeries(t, ui.mining, "Critical mining").Values[1] != 156 {
		t.Fatal("volume averaging changed event contributions")
	}
	window := fyne.CurrentApp().NewWindow("Restored mining volume")
	defer window.Close()
	if !newAnalyzerUI(window).miningVolume {
		t.Fatal("volume preference did not persist")
	}
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), "Units")
	if ui.mining.data.HitUnit != "units" || findLogisticsSeries(t, ui.mining, "Critical mining").Hits[0].Damage != 156 {
		t.Fatal("units toggle did not restore raw yield")
	}
}

func TestMiningOreTotalsColumnsSortingAndRange(t *testing.T) {
	ui, start := newMiningVolumeTestUI(t)
	if !strings.Contains(ui.summary.Text, "Mining 266 units / 2,561 m3") {
		t.Fatalf("incorrect footer: %s", ui.summary.Text)
	}
	rows, columns := ui.totals.Length()
	if columns != 11 || rows != 4 {
		t.Fatalf("mixed damage/ore table dimensions: %d/%d", rows, columns)
	}
	for _, series := range ui.tableSeries {
		if series.Name == "Loparite" {
			want := []string{"256", "2,560", "156", "1,560"}
			for i, value := range want {
				if got := miningTotalsCell(series, 7+i); got != value {
					t.Fatalf("mining column %d = %s, want %s", 7+i, got, value)
				}
			}
		}
	}
	for _, col := range []int{7, 8, 9, 10} {
		ui.totals.Select(widget.TableCellID{Row: 0, Col: col})
		if ui.tableSeries[0].Name != "Loparite" {
			t.Fatalf("column %d did not sort mining totals numerically", col)
		}
	}
	ui.tabs.SelectIndex(1)
	ui.window.Resize(fyne.NewSize(1920, 960))
	ui.totals.ScrollToLeading()
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_MINING_TOTALS_CAPTURE"))
	ui.applyGraphRange(start.Add(3*time.Second), start.Add(4*time.Second))
	_, columns = ui.totals.Length()
	if columns != 7 || len(ui.tableSeries) != 1 || ui.tableSeries[0].TotalOutgoing != 200 {
		t.Fatal("mining columns/rows must disappear outside the mined-event range while keeping damage")
	}
	ui.applyGraphRange(start, start.Add(4*time.Second))
	ui.miningFilters[filterOreName].set("Loparite", false)
	ui.saveMiningFilters()
	if ui.miningTotals.Units != 10 || ui.miningTotals.Volume != 1 {
		t.Fatal("ore filters did not affect mining totals")
	}
}

func TestUnknownOreVolumeDoesNotInventTotals(t *testing.T) {
	ui, start := newMiningVolumeTestUI(t)
	ui.snapshot.Logs[0].Mining = append(ui.snapshot.Logs[0].Mining, miningEvent{Second: start.Unix(), Kind: miningMined, Ore: "Future ore", Amount: 5, Critical: true})
	ui.rebuildView()
	if !strings.Contains(ui.summary.Text, "Mining 271 units / Unknown m3") {
		t.Fatalf("unknown volume hidden: %s", ui.summary.Text)
	}
	for _, row := range ui.tableSeries {
		if row.Name == "Future ore" && (miningTotalsCell(row, 8) != "Unknown" || miningTotalsCell(row, 10) != "Unknown") {
			t.Fatal("unknown ore volume presented as zero")
		}
	}
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), "Volume")
	mined := findLogisticsSeries(t, ui.mining, "Miner · Ore mined")
	if len(mined.Hits) != 3 {
		t.Fatal("unknown ore must not create fabricated volume points")
	}
}

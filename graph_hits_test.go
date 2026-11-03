package main

import (
	"context"
	"image/color"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func plottedHits(data plotData) map[string][]float64 {
	result := map[string][]float64{}
	for _, series := range data.Series {
		for _, hit := range series.Hits {
			result[series.Name] = append(result[series.Name], hit.Damage)
		}
	}
	return result
}

func TestIndividualHitsPreserveDamageAcrossBucketsGroupsAndFilters(t *testing.T) {
	ui, _ := newIncomingGroupingTestUI(t)
	if !ui.incoming.data.IndividualHits || !ui.outgoing.data.IndividualHits {
		t.Fatal("non-averaged graphs did not use individual-hit mode")
	}
	want := map[string][]float64{"Pilot A": {20, 30, 40, 50, 60, 70, 0}, "Pilot B": {10, 20}}
	if got := plottedHits(ui.incoming.data); !reflect.DeepEqual(got, want) {
		t.Fatalf("same-second hits were combined or idle samples added: %v", got)
	}
	if math.Abs(ui.incoming.data.Maximum-70*1.08) > 1e-10 {
		t.Fatal("point axis used bucket totals instead of individual damage")
	}
	setTestGraphBucket(ui, 5*time.Second)
	if got := plottedHits(ui.incoming.data); !reflect.DeepEqual(got, want) {
		t.Fatalf("graph interval changed individual hits: %v", got)
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPCType)
	if got := plottedHits(ui.incoming.data)["Sentry Gun"]; !reflect.DeepEqual(got, []float64{50, 20, 0}) {
		t.Fatalf("NPC grouping summed simultaneous hits: %v", got)
	}
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	ui.setDamageFilterOption(filterNPCNames, "npc-name:amarr sentry gun", true)
	if got := plottedHits(ui.incoming.data); !reflect.DeepEqual(got, map[string][]float64{"Sentry Gun": {50, 20, 0}}) {
		t.Fatalf("NPC filters leaked points: %v", got)
	}
	if filteredTotal(ui) != 100 || npcIncomingTotal(ui) != 70 || len(plottedHits(ui.outgoing.data)) != 2 {
		t.Fatal("point display changed totals or outgoing damage")
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPC)
	for index, source := range ui.snapshot.Logs[1].Sources {
		if source.NPC {
			ui.snapshot.Logs[1].Sources[index].Name = strings.ToLower(source.Name)
		}
	}
	ui.rebuildView()
	if got := plottedHits(ui.incoming.data); !reflect.DeepEqual(got, map[string][]float64{"Amarr Sentry Gun": {50, 20, 0}}) {
		t.Fatalf("case-insensitive NPC grouping lost raw hit points: %v", got)
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPCType)
	ui.setDamageFilterOption(filterCharacters, "Pilot B", false)
	if got := plottedHits(ui.incoming.data)["Sentry Gun"]; !reflect.DeepEqual(got, []float64{50, 0}) {
		t.Fatalf("character filtering leaked points: %v", got)
	}
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	if ui.incoming.data.IndividualHits || ui.outgoing.data.IndividualHits || ui.incoming.data.AverageWindow != dpsAverageWindow {
		t.Fatal("enabling averaging did not restore rate lines")
	}
	for _, series := range ui.incoming.data.Series {
		if len(series.Hits) > 0 {
			t.Fatal("averaged series retained individual-hit data")
		}
	}
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	if got := plottedHits(ui.incoming.data)["Sentry Gun"]; !reflect.DeepEqual(got, []float64{50, 0}) {
		t.Fatal("toggling averaging lost raw hit records")
	}
}

func TestIndividualHitsUseHistoricalShipsAtTheirOwnTimestamp(t *testing.T) {
	ui, start := newNPCFilterUI(t)
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 10 to Enemy - Laser - Hits\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 15 to Enemy - Laser - Hits\n" +
		"[ 2026.10.19 02:00:01 ] (combat) 20 to Enemy - Laser - Hits\n"))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}}
	ui.shipHistory = shipHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", TypeID: 1, TypeName: "Loki", FirstSeen: start, LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot A", TypeID: 2, TypeName: "Thrasher", FirstSeen: start.Add(time.Second), LastSeen: start.Add(2 * time.Second)},
	}}
	setTestGraphBucket(ui, 5*time.Second)
	ui.groupSelect.SetSelected("Ship history")
	ui.rebuildView()
	if got := plottedHits(ui.outgoing.data); !reflect.DeepEqual(got, map[string][]float64{"Loki": {10, 15}, "Thrasher": {20}}) {
		t.Fatalf("raw hit ship attribution: %v", got)
	}
	if len(log.Hits) != 3 || log.Hits[0].Second != log.Hits[1].Second {
		t.Fatal("parser combined individual hits within a second")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:1", true)
	if got := plottedHits(ui.outgoing.data); !reflect.DeepEqual(got, map[string][]float64{"Loki": {10, 15}}) {
		t.Fatalf("event-time ship filter leaked raw hits: %v", got)
	}
}

func TestIndividualHitRasterHasPointsWithoutLinesOrIdleSamples(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Individual damage hits")
	defer window.Close()
	chart := newDamageChart()
	window.SetPadded(false)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(1076, 300))
	window.Show()
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	red := color.NRGBA{255, 60, 60, 255}
	chart.setData(plotData{Start: start, End: start.Add(10 * time.Second), Bucket: 5 * time.Second, IndividualHits: true, Series: []plotSeries{{Name: "Pilot A", Color: red, Values: []float64{10000, 10000}, Hits: []plotHit{
		{Second: start.Unix() + 2, Damage: 20, Character: "Pilot A"},
		{Second: start.Unix() + 2, Damage: 60, Character: "Pilot A"},
		{Second: start.Unix() + 8, Damage: 40, Character: "Pilot A"},
		{Second: start.Unix() + 8, Damage: 0, Character: "Pilot A"},
	}}}})
	if math.Abs(chart.data.Maximum-64.8) > 1e-10 {
		t.Fatal("raw damage axis included aggregated rate values")
	}
	plot := chart.renderPlot(1001, 201)
	for _, hit := range chart.data.Series[0].Hits {
		x := int(hit.Second-start.Unix()) * 100
		y := int(math.Round(200 * (1 - chart.data.fraction(hit.Damage))))
		if got := plot.At(x, y); got != (color.RGBA{255, 60, 60, 255}) {
			t.Fatalf("hit missing at %d,%d: %v", x, y, got)
		}
	}
	for x := 350; x < 650; x++ {
		for y := 0; y < 201; y++ {
			pixel := color.RGBAModel.Convert(plot.At(x, y)).(color.RGBA)
			if pixel.R > 100 && pixel.G < 100 {
				t.Fatal("individual hits were connected by a line")
			}
		}
	}
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	chart.MouseMoved(testMouseEvent(chartPlotLeft+width*.2, 80, 0))
	if !strings.Contains(chart.hoverReadings[0].Text, "60 damage") || !strings.Contains(chart.hoverReadings[1].Text, "20 damage") || chart.hoverReadings[2].Visible() {
		t.Fatal("hover summed simultaneous hits or reported DPS")
	}
	chart.MouseMoved(testMouseEvent(chartPlotLeft+width*.5, 80, 0))
	for _, reading := range chart.hoverReadings {
		if reading.Visible() {
			t.Fatal("idle time produced a synthetic zero-damage hit")
		}
	}
	captureWindowPreview(t, window, os.Getenv("EVELA_HITS_CAPTURE"))
}

func TestLocalIndividualHitRecords(t *testing.T) {
	directory := os.Getenv("EVELA_TEST_LOGS")
	if directory == "" {
		t.Skip("set EVELA_TEST_LOGS to check real individual hits read-only")
	}
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, logLoadingCutoff(time.Now(), 30), nil)
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, log := range snapshot.Logs {
		var damage float64
		var count int
		for _, point := range log.Points {
			damage += point.Incoming + point.Outgoing
			count += point.IncomingHits + point.OutgoingHits
		}
		for _, hit := range log.Hits {
			damage -= hit.Amount
			count--
		}
		if math.Abs(damage) > .01 || count != 0 {
			t.Fatalf("raw hits disagree with totals for %s: damage=%f hits=%d", log.Character, damage, count)
		}
		total += len(log.Hits)
	}
	if total == 0 {
		t.Fatal("no individual hits found in local logs")
	}
	t.Logf("%d files; %d separate incoming/outgoing hit records agree with existing totals", snapshot.Files, total)
}

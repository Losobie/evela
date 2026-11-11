package main

import (
	"bytes"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"golang.org/x/text/encoding/unicode"
)

const miningTestLog = "Listener: Miner\n" +
	"[ 2024.12.24 06:52:34 ] (notify) Successfully compressed Bountiful Xenotime into 308 Compressed Bountiful Xenotime.\n" +
	"[ 2024.12.24 06:51:57 ] (mining) <color=0x77ffffff>You mined <font size=12><color=0xffaaaa00>154<color=0x77ffffff><font size=10> units of <color=0xffffffff><font size=12>Bountiful Xenotime<color=0x77ffffff><font size=10> with a lost residue of <font size=12><color=0xffaaaa00>0<color=0x77ffffff><font size=10> units\n" +
	"[ 2024.12.24 06:51:59 ] (mining) You mined 276 units of Prismatic Gneiss with a lost residue of 24 units\n" +
	"[ 2024.12.24 06:51:58 ] (mining) You mined 100 units of Xenotime with a lost residue of 10 units\n" +
	"[ 2024.12.24 06:52:35 ] (combat) You mined 999 units of Veldspar with a lost residue of 999 units\n" +
	"[ 2024.12.24 06:52:36 ] (notify) Successfully compressed Tritanium into 9 Compressed Tritanium.\n"

func TestMiningParsingAndStaticOreTypes(t *testing.T) {
	for _, utf16 := range []bool{false, true} {
		input := []byte(miningTestLog)
		if utf16 {
			var err error
			input, err = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(input)
			if err != nil {
				t.Fatal(err)
			}
		}
		log, err := parseLog(bytes.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		if len(log.Mining) != 8 || len(log.Points) != 0 || len(log.Logistics) != 0 {
			t.Fatalf("unexpected records: %+v", log.Mining)
		}
		first := log.Mining[0]
		if first.Ore != "Bountiful Xenotime" || first.Amount != 154 || first.Kind != miningMined || strings.Contains(first.Message, "<") {
			t.Fatalf("mined sample: %+v", first)
		}
		if log.Mining[1].Kind != miningWaste || log.Mining[1].Amount != 0 {
			t.Fatal("zero residue lost")
		}
		if event := log.Mining[6]; event.Kind != miningCompressed || event.Ore != first.Ore || event.Amount != 308 {
			t.Fatalf("compression: %+v", event)
		}
		// The parser retains unknown future ores; classification never drops data.
		if oreType(log.Mining[7].Ore) != "Unknown ore type" {
			t.Fatal("unknown ore guessed")
		}
		for i := 1; i < len(log.Mining); i++ {
			if log.Mining[i].Second < log.Mining[i-1].Second {
				t.Fatal("mining not sorted")
			}
		}
	}
	for name, want := range map[string]string{"Bountiful Xenotime": "Exceptional Moon Ore", "Xenotime": "Exceptional Moon Ore", "Compressed Bountiful Xenotime": "Exceptional Moon Ore", "Prismatic Gneiss": "Gneiss", "Dense Veldspar": "Veldspar", "Bitumens": "Ubiquitous Moon Ore", "Unknown resource": "Unknown ore type"} {
		if got := oreType(name); got != want {
			t.Errorf("oreType(%q)=%q, want %q", name, got, want)
		}
	}
	for _, message := range []string{"You mined -1 units of Veldspar", "You mined 1,23 units of Veldspar", "You mined 18446744073709551616 units of Veldspar", "You mined 5 units of Veldspar with a lost residue of invalid units", "You mined 5 units of Veldspar with a lost residue of 1,23 units"} {
		if got := parseMining(message, "(mining)", 1); len(got) != 0 {
			t.Errorf("accepted invalid message %q: %+v", message, got)
		}
	}
	if got := parseMining("Successfully compressed Xenotime into 308 Compressed Gneiss.", "(notify)", 1); len(got) != 0 {
		t.Fatal("accepted mismatched compressed ore")
	}
	if got := parseMining("You mined 1,234 units of Veldspar", "(mining)", 1); len(got) != 1 || got[0].Amount != 1234 {
		t.Fatal("legacy mining format or thousands separators lost")
	}
}

func newMiningTestUI(t *testing.T) *analyzerUI {
	t.Helper()
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Mining preview")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	enableLegacyTestFilters(ui)
	log, err := parseLog(strings.NewReader(miningTestLog))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{SourceFiles: map[string]cachedLog{"mining.txt": {Log: log}}}
	ui.snapshot.populateLogs()
	ui.graphAverageWindow = 0
	ui.selectQuickRange("All history")
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	return ui
}

func TestCriticalMiningGroupingAndTotals(t *testing.T) {
	ui := newMiningTestUI(t)
	log, err := parseLog(strings.NewReader(miningTestLog + "[ 2024.12.24 06:51:59 ] (mining) Critical mining success! You mined an additional 156 units of Loparite\n"))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{SourceFiles: map[string]cachedLog{"mining.txt": {Log: log}}}
	ui.snapshot.populateLogs()
	ui.selectQuickRange("All history")
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByCriticals)
	regular := findLogisticsSeries(t, ui.mining, "Regular mining")
	critical := findLogisticsSeries(t, ui.mining, "Critical mining")
	if len(regular.Hits) != 3 || len(critical.Hits) != 1 || critical.Hits[0].Damage != 156 || !strings.Contains(critical.Hits[0].Detail, "Critical mining success!") {
		t.Fatalf("incorrect regular/critical split: %+v / %+v", regular.Hits, critical.Hits)
	}
	if !strings.Contains(ui.summary.Text, "Mining 686 units") || ui.miningTotals.Units != 686 {
		t.Fatalf("critical bonus missing from totals: %v / %v", ui.summary.Text, ui.miningTotals.Units)
	}
	ui.initEventGraphGrouping()
	if ui.eventGraphGroupings[graphMining] != eventByCriticals {
		t.Fatal("critical grouping preference was not restored")
	}
	ui.miningFilters[filterMiningMessage].set(miningMined, false)
	ui.saveMiningFilters()
	if ui.miningTotals.Units != 0 {
		t.Fatalf("mined filter must exclude regular and critical ore: %v", ui.miningTotals.Units)
	}
}

func TestMiningSummaryTotal(t *testing.T) {
	ui := newMiningTestUI(t)
	if !strings.Contains(ui.summary.Text, "Mining 530 units") {
		t.Fatalf("mined total must exclude waste and compression: %s", ui.summary.Text)
	}
	if ui.miningTotals.Units != 530 {
		t.Fatalf("incorrect Totals tab mining total: %v", ui.miningTotals.Units)
	}
	start := time.Date(2024, 12, 24, 6, 51, 58, 0, time.UTC)
	ui.applyGraphRange(start, start.Add(time.Second))
	if !strings.Contains(ui.summary.Text, "Mining 100 units") {
		t.Fatalf("mining total must respect range boundaries: %s", ui.summary.Text)
	}
	if ui.miningTotals.Units != 100 {
		t.Fatalf("Totals tab mining total did not follow range: %v", ui.miningTotals.Units)
	}
	ui.applyGraphRange(start.Add(36*time.Second), start.Add(37*time.Second))
	if strings.Contains(ui.summary.Text, "Mining") {
		t.Fatalf("compression alone must not show a mining total: %s", ui.summary.Text)
	}
	if ui.miningTotals.Units != 0 {
		t.Fatalf("Totals tab retained stale mining total: %v", ui.miningTotals.Units)
	}
}

func TestMiningGraphFiltersGroupsPersistenceAndVisibility(t *testing.T) {
	ui := newMiningTestUI(t)
	assertGraphVisibility(t, ui, graphMining, false)
	if !ui.selected["Miner"] || len(ui.filterOptions[filterCharacters]) != 1 || len(ui.filterOptions[filterShipType]) != 1 || !strings.HasPrefix(ui.summary.Text, "1 selected") || len(ui.tableSeries) != 3 {
		t.Fatal("mining-only character lost or damage totals contaminated")
	}
	if !ui.snapshot.First.Equal(time.Date(2024, 12, 24, 6, 51, 57, 0, time.UTC)) || !ui.snapshot.Last.Equal(time.Date(2024, 12, 24, 6, 52, 36, 0, time.UTC)) {
		t.Fatal("mining did not establish archive range")
	}
	if len(ui.mining.data.Series) != 3 || ui.mining.data.HitUnit != "units" {
		t.Fatal("message kinds must stay separate with correct units")
	}
	series := findLogisticsSeries(t, ui.mining, "Miner · Ore mined")
	if len(series.Hits) != 3 || series.Hits[0].Damage != 154 || series.Hits[0].Character != "Miner" || !strings.Contains(series.Hits[0].Detail, "Bountiful Xenotime") {
		t.Fatal("raw amounts or hover details lost")
	}
	// Shared NPC filters do not affect mining; ship filters do.
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	if len(ui.mining.data.Series) != 3 {
		t.Fatal("NPC filter leaked into mining")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("ship filter ignored")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.setDamageFilterOption(filterCharacters, "Miner", false)
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("character filter ignored")
	}
	ui.setDamageFilterOption(filterCharacters, "Miner", true)
	// Exact grades are independent; compression uses its source ore grade.
	ui.miningFilters[filterOreName].Default = false
	ui.miningFilters[filterOreName].set("Bountiful Xenotime", true)
	ui.saveMiningFilters()
	if hits := findLogisticsSeries(t, ui.mining, "Miner · Ore mined").Hits; len(hits) != 1 || hits[0].Damage != 154 {
		t.Fatal("ore grade filter ignored")
	}
	if findLogisticsSeries(t, ui.mining, "Miner · Compressed").Hits[0].Damage != 308 {
		t.Fatal("compression did not share the ore grade")
	}
	ui.miningFilters[filterOreType].set("Exceptional Moon Ore", false)
	ui.saveMiningFilters()
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("ore type filter ignored")
	}
	ui.miningFilters[filterOreType].set("Exceptional Moon Ore", true)
	ui.miningFilters[filterMiningMessage].Default = false
	ui.miningFilters[filterMiningMessage].set(miningMined, true)
	ui.saveMiningFilters()
	ui.characterGroups = []characterGroup{{ID: "fleet", Name: "Mining fleet", Members: []string{"Miner"}}, {ID: "other", Name: "Other fleet", Members: []string{"Miner"}}}
	before := ui.outgoing.data
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByGroup)
	if len(ui.eventGroupingMenu(graphMining).Items) != 11 || len(ui.mining.data.Series) != 2 || !reflect.DeepEqual(before, ui.outgoing.data) {
		t.Fatal("grouping did not stay independent or lost overlapping memberships")
	}
	ui.setDamageFilterOption(filterCharacterGroup, "group:other", false)
	if len(ui.mining.data.Series) != 1 || ui.mining.data.Series[0].Name != "Mining fleet · Ore mined" {
		t.Fatal("group filter ignored")
	}
	ui.setAllDamageFilterOptions(filterSystem, false)
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("system filter ignored")
	}
	ui.setAllDamageFilterOptions(filterSystem, true)
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show Mining")
	assertGraphVisibility(t, ui, graphMining, true)
	if ui.mining.hoverGroup != ui.outgoing.hoverGroup || ui.mining.OnRangeSelected == nil || ui.mining.OnWheelZoom == nil {
		t.Fatal("mining navigation not connected")
	}
	// Show mining alone to check its controls and chart at the normal window size.
	for _, option := range graphVisibilityOptions {
		fyne.CurrentApp().Preferences().SetBool(graphVisibilityPreference+option.id, option.id == graphMining)
	}
	ui.applyGraphVisibility()
	ui.graphsScroll.ScrollToTop()
	for _, button := range ui.miningFilterButtons {
		if button.Size().Width < button.MinSize().Width || button.Size().Height < button.MinSize().Height {
			t.Fatal("mining filter clipped")
		}
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_MINING_CAPTURE"))
	// Search and All reuse the shared searchable checkbox popup.
	test.Tap(ui.miningFilterButtons[filterOreName])
	ui.filterPopup.search.SetText("Bountiful")
	if len(ui.filterPopup.visible) != 1 {
		t.Fatal("ore filter search failed")
	}
	ui.filterPopup.popup.Hide()
	other := fyne.CurrentApp().NewWindow("Restored mining")
	defer other.Close()
	restored := newAnalyzerUI(other)
	if restored.eventGraphGroupings[graphMining] != eventByGroup || restored.miningFilters[filterOreName].selected("Xenotime") || !restored.miningFilters[filterOreName].selected("Bountiful Xenotime") || restored.miningFilters[filterMiningMessage].selected(miningWaste) || !graphVisible(graphMining) {
		t.Fatal("mining settings not restored")
	}
}

func TestMiningAverageLookbackAndPartialBuckets(t *testing.T) {
	ui := newMiningTestUI(t)
	start := ui.snapshot.First
	ui.miningFilters[filterMiningMessage].Default = false
	ui.miningFilters[filterMiningMessage].set(miningMined, true)
	ui.graphAverageWindow = dpsAverageWindow
	ui.applyGraphRange(start.Add(time.Second), start.Add(13*time.Second))
	setTestGraphBucket(ui, 5*time.Second)
	series := findLogisticsSeries(t, ui.mining, "Miner · Ore mined")
	// First bucket: 154*5 + 100*5 + 276*4, divided by 10 seconds and 5 seconds.
	want := []float64{47.48, 49.92, 13.8}
	for i, value := range series.Values {
		if math.Abs(value-want[i]) > 1e-9 {
			t.Fatalf("average buckets: %v, want %v", series.Values, want)
		}
	}
	if len(series.Hits) != 2 || ui.mining.data.AverageWindow != 10*time.Second {
		t.Fatal("lookback event appeared as an in-range raw hit")
	}
}

func TestMiningGroupsByHistoricalShipAndOreType(t *testing.T) {
	ui := newMiningTestUI(t)
	start := ui.snapshot.First
	ui.shipHistory = shipHistory{1: {
		{CharacterID: 1, CharacterName: "Miner", TypeID: 1, TypeName: "Hulk", FirstSeen: start, LastSeen: start.Add(time.Second)},
		{CharacterID: 1, CharacterName: "Miner", TypeID: 2, TypeName: "Mackinaw", FirstSeen: start.Add(2 * time.Second), LastSeen: ui.snapshot.Last},
	}}
	other := &parsedLog{Character: "Other miner", Mining: []miningEvent{{Second: start.Unix(), Ore: "Xenotime", Kind: miningMined, Amount: 50}}}
	ui.snapshot.Logs = append(ui.snapshot.Logs, other)
	ui.snapshot.Characters = append(ui.snapshot.Characters, other.Character)
	ui.rebuildView()
	before := ui.outgoing.data
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByShip)
	if !reflect.DeepEqual(before, ui.outgoing.data) {
		t.Fatal("mining ship grouping changed another graph")
	}
	if hits := findLogisticsSeries(t, ui.mining, "Hulk · Ore mined").Hits; len(hits) != 2 || hits[0].Damage != 154 || hits[1].Damage != 100 {
		t.Fatal("mining did not use the ship at the event timestamp")
	}
	if hits := findLogisticsSeries(t, ui.mining, "Mackinaw · Ore mined").Hits; len(hits) != 1 || hits[0].Damage != 276 {
		t.Fatal("mining ship change lost")
	}
	if findLogisticsSeries(t, ui.mining, "Unknown ship · Ore mined").Hits[0].Character != other.Character {
		t.Fatal("mining without ship history did not stay unknown")
	}
	// The old hull retains the tail when averaging extends beyond a ship change.
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "30-second DPS average")
	if got := findLogisticsSeries(t, ui.mining, "Hulk · Ore mined").Values[3]; math.Abs(got-254.0/30) > 1e-9 {
		t.Fatalf("mining tail moved to the new hull: %v", got)
	}
	before = ui.outgoing.data
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByOreType)
	moon := findLogisticsSeries(t, ui.mining, "Exceptional Moon Ore · Ore mined")
	if len(moon.Hits) != 3 || math.Abs(moon.Values[3]-304.0/30) > 1e-9 {
		t.Fatal("ore type did not combine grades and characters")
	}
	if findLogisticsSeries(t, ui.mining, "Gneiss · Ore mined").Hits[0].Damage != 276 || findLogisticsSeries(t, ui.mining, "Exceptional Moon Ore · Compressed").Hits[0].Damage != 308 {
		t.Fatal("ore type mixed message kinds or ore families")
	}
	if !reflect.DeepEqual(before, ui.outgoing.data) {
		t.Fatal("mining grouping changed another graph")
	}
	window := fyne.CurrentApp().NewWindow("Restored ore grouping")
	defer window.Close()
	if restored := newAnalyzerUI(window); restored.eventGraphGroupings[graphMining] != eventByOreType {
		t.Fatal("ore type grouping not saved")
	}
	ui.miningFilters[filterOreType].set("Exceptional Moon Ore", false)
	ui.saveMiningFilters()
	for _, series := range ui.mining.data.Series {
		if strings.HasPrefix(series.Name, "Exceptional Moon Ore") {
			t.Fatal("ore grouping ignored the type filter")
		}
	}
}

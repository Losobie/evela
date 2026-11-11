package main

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestScopedFiltersAddRemovePersistAndIntersect(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Scoped filters")
	defer window.Close()
	ui := newAnalyzerUI(window)
	for scope, kinds := range ui.visibleFilters {
		for kind, visible := range kinds {
			if visible {
				t.Fatalf("new installation shows %s/%s", scope, kind)
			}
		}
	}
	input := "Listener: Pilot\n" + strings.Join([]string{
		"[ 2026.10.19 02:00:00 ] (combat) 100 to Enemy - Laser - Hits",
		"[ 2026.10.19 02:00:00 ] (combat) 200 to Enemy - Ogre I - Penetrates",
		"[ 2026.10.19 02:00:00 ] (combat) 300 from Enemy[C](Hulk) - Laser - Hits",
		"[ 2026.10.19 02:00:00 ] (combat) 400 from Enemy[C](Hulk) - Ogre I - Smashes",
		"[ 2026.10.19 02:00:00 ] (combat) 50 remote armor repaired to Enemy",
		"[ 2026.10.19 02:00:00 ] (combat) 20 remote shield boosted to Enemy",
		"[ 2026.10.19 02:00:00 ] (combat) 10 GJ energy neutralized Enemy - Neut",
		"[ 2026.10.19 02:00:00 ] (combat) Warp scramble attempt from Enemy to you!",
		"[ 2026.10.19 02:00:00 ] (mining) You mined 5 units of Veldspar with a lost residue of 2 units",
	}, "\n")
	log, err := parseLog(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot"}, Logs: []*parsedLog{log}, First: start, Last: start}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1200, 800))
	window.Show()
	total := func() (float64, float64) {
		var in, out float64
		for _, series := range ui.view.activeSeries() {
			in += series.TotalIncoming
			out += series.TotalOutgoing
		}
		return in, out
	}
	assertTotal := func(wantIn, wantOut float64) {
		t.Helper()
		in, out := total()
		if in != wantIn || out != wantOut {
			t.Fatalf("totals %v/%v want %v/%v", in, out, wantIn, wantOut)
		}
	}
	assertTotal(700, 300)
	chooseGraphMenuItem(t, ui.addFilterMenu(graphOutgoing), filterWeaponType)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Ogre I", true)
	assertTotal(700, 200)
	if ui.visibleFilters[graphIncoming][filterWeaponType] {
		t.Fatal("outgoing visibility leaked to incoming")
	}
	for _, item := range ui.addFilterMenu(graphOutgoing).Items {
		if item.Label == filterWeaponType && !item.Checked {
			t.Fatal("Add filter does not check the visible filter")
		}
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, false)
	assertTotal(700, 300)
	if ui.scopedFilterButtons[graphOutgoing][filterWeaponType] != nil {
		t.Fatal("removed filter button is still visible")
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, true)
	assertTotal(700, 300)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Ogre I", true)
	assertTotal(700, 200)
	ui.setFilterVisible(graphIncoming, filterCharacters, true)
	ui.setAllScopeFilters(graphIncoming, filterCharacters, false)
	assertTotal(0, 200)
	ui.setFilterVisible(graphIncoming, filterCharacters, false)
	assertTotal(700, 200)
	ui.setFilterVisible(globalFilterScope, filterShipType, true)
	ui.setAllScopeFilters(globalFilterScope, filterShipType, false)
	assertTotal(0, 0)
	if len(ui.outgoingLogistics.data.Series) != 0 || len(ui.ewar.data.Series) != 0 || len(ui.mining.data.Series) != 0 {
		t.Fatal("global filter failed to intersect event graphs")
	}
	ui.setFilterVisible(globalFilterScope, filterShipType, false)
	assertTotal(700, 200)
	ui.setFilterVisible(graphOutgoingLogistics, filterLogisticsType, true)
	ui.setAllScopeFilters(graphOutgoingLogistics, filterLogisticsType, false)
	if len(ui.outgoingLogistics.data.Series) != 0 {
		t.Fatal("local logistics filter ignored")
	}
	ui.setFilterVisible(graphOutgoingLogistics, filterLogisticsType, false)
	if len(ui.outgoingLogistics.data.Series) != 2 {
		t.Fatal("hidden logistics filter still active")
	}
	ui.setFilterVisible(graphMining, filterMiningMessage, true)
	ui.setAllScopeFilters(graphMining, filterMiningMessage, false)
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("local mining filter ignored")
	}
	ui.setFilterVisible(graphMining, filterMiningMessage, false)
	if len(ui.mining.data.Series) != 2 {
		t.Fatal("hidden mining filter still active")
	}
	ui.setFilterVisible(graphIncomingEWar, filterEffectType, true)
	ui.setAllScopeFilters(graphIncomingEWar, filterEffectType, false)
	ui.setScopeFilter(graphIncomingEWar, filterEffectType, "Warp scramble", true)
	if len(ui.ewar.data.Series) != 1 || ui.ewar.data.Series[0].Name != "Warp scramble" {
		t.Fatal("E-War effect filter ignored")
	}
	assertTotal(700, 200)
	// Both visibility and retained selections survive a new UI instance.
	restoredWindow := app.NewWindow("Restored filters")
	defer restoredWindow.Close()
	restored := newAnalyzerUI(restoredWindow)
	restored.snapshot = ui.snapshot
	restored.graphAverageWindow = 0
	restored.applyGraphRange(start, start.Add(2*time.Second))
	restored.rebuildView()
	if !reflect.DeepEqual(restored.visibleFilters, ui.visibleFilters) || restored.view.activeSeries()[0].TotalOutgoing != 200 || len(restored.mining.data.Series) != 2 {
		t.Fatal("scoped filter persistence failed")
	}
	// Captures both filter bars and independent per-graph filters for visual QA.
	captureWindowPreview(t, window, os.Getenv("EVELA_SCOPED_FILTER_CAPTURE"))
}

func TestGraphFilterAndGroupingCatalogs(t *testing.T) {
	for _, graph := range graphVisibilityOptions {
		kinds := availableFilters(graph.id)
		for _, kind := range sharedDamageFilterKinds {
			if !slices.Contains(kinds, kind) {
				t.Fatalf("%s missing %s", graph.id, kind)
			}
		}
		damage := graph.id == graphIncoming || graph.id == graphOutgoing
		if slices.Contains(kinds, filterWeaponType) != damage || slices.Contains(kinds, filterWeaponStrength) != damage {
			t.Fatal("weapon filters offered for unrelated data")
		}
		if slices.Contains(kinds, filterNPCNames) != (graph.id == graphIncoming) {
			t.Fatal("NPC filter offered for unrelated data")
		}
		if slices.Contains(kinds, filterLogisticsType) != (graph.id == graphIncomingLogistics || graph.id == graphOutgoingLogistics) {
			t.Fatal("repair Type filter offered for unrelated data")
		}
	}
	if slices.Contains(availableFilters(globalFilterScope), filterWeaponType) {
		t.Fatal("damage-only filter offered globally")
	}
	for _, id := range []string{graphMining, graphIncomingEWar, graphIncomingLogistics, graphOutgoingLogistics} {
		modes := eventGroupingOptions(id)
		if !slices.Contains(modes, filterSystem) || !slices.Contains(modes, eventByClass) {
			t.Fatalf("%s missing meaningful history grouping", id)
		}
		if slices.Contains(modes, filterWeaponType) || slices.Contains(modes, incomingByNPC) {
			t.Fatal("unrelated grouping offered")
		}
	}
	if !slices.Contains(eventGroupingOptions(graphMining), eventByOreName) || slices.Contains(eventGroupingOptions(graphMining), filterMiningMessage) {
		t.Fatal("mining must support ore names without redundant message grouping")
	}
}

func TestClearAllFiltersRestoresMiningAndPersists(t *testing.T) {
	ui := newMiningTestUI(t)
	for scope := range ui.visibleFilters {
		for _, kind := range availableFilters(scope) {
			ui.visibleFilters[scope][kind] = true
			selection := ui.storedScopeFilter(scope, kind)
			selection.Default = false
			selection.Values = map[string]bool{"stale option": true}
		}
	}
	ui.selected["Miner"] = false
	ui.rebuildFilteredView()
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("test restrictions did not hide mining")
	}
	start, end := ui.view.Start, ui.view.End
	ui.eventGraphGroupings[graphMining] = eventByOreName
	menu := ui.graphOptionsMenu()
	last := len(menu.Items) - 1
	if menu.Items[last].Label != "Clear all filters" || !menu.Items[last-1].IsSeparator || menu.Items[last-2].Label != "Show Mining" {
		t.Fatal("Clear all filters needs its own section below graph visibility")
	}
	menu.Items[last].Action()
	if len(ui.mining.data.Series) == 0 || ui.miningTotal.Text != "Mining total: 530 units" || !ui.selected["Miner"] {
		t.Fatal("clearing filters did not restore mining and character selection")
	}
	if !ui.view.Start.Equal(start) || !ui.view.End.Equal(end) || ui.eventGraphGroupings[graphMining] != eventByOreName {
		t.Fatal("clearing filters changed the range or grouping")
	}
	checkCleared := func(ui *analyzerUI) {
		t.Helper()
		for scope := range ui.visibleFilters {
			for _, kind := range availableFilters(scope) {
				selection := ui.storedScopeFilter(scope, kind)
				if ui.visibleFilters[scope][kind] || selection.restricted() || !selection.selected("new option") || len(selection.Values) != 0 {
					t.Fatalf("%s/%s was not removed and reset", scope, kind)
				}
			}
		}
	}
	checkCleared(ui)
	window := fyne.CurrentApp().NewWindow("Restored cleared filters")
	defer window.Close()
	checkCleared(newAnalyzerUI(window))
	ui.setFilterVisible(graphMining, filterOreName, true)
	if ui.miningTotal.Text != "Mining total: 530 units" {
		t.Fatal("readding a cleared ore filter restored old exclusions")
	}
}

func TestRemovingFiltersResetsGlobalAndGraphSelections(t *testing.T) {
	ui := newMiningTestUI(t)
	for scope := range ui.visibleFilters {
		for _, kind := range availableFilters(scope) {
			ui.setFilterVisible(scope, kind, true)
			ui.setAllScopeFilters(scope, kind, false)
			ui.setScopeFilter(scope, kind, "old option", true)
			ui.setFilterVisible(scope, kind, false)
			ui.setFilterVisible(scope, kind, true)
			selection := ui.storedScopeFilter(scope, kind)
			if selection.restricted() || !selection.selected("new option") || len(selection.Values) != 0 {
				t.Fatalf("readding %s/%s restored exclusions", scope, kind)
			}
		}
	}
	if !ui.selected["Miner"] || ui.miningTotal.Text != "Mining total: 530 units" {
		t.Fatal("removing and readding filters lost the selected miner or yield")
	}
}

func TestHiddenLocalSystemFilterMigratesNames(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("System migration")
	defer window.Close()
	ui := newAnalyzerUI(window)
	ui.storedScopeFilter(graphMining, filterSystem).set("system-name:jita", false)
	ui.migrateSystemFilterNames(locationResolver{systems: map[string]int64{"jita": 30000142}})
	if ui.storedScopeFilter(graphMining, filterSystem).selected("system:30000142") || ui.visibleFilters[graphMining][filterSystem] {
		t.Fatal("migration lost hidden local selection or activated it")
	}
}

func TestFilterBarsWrapWhileKeepingActionsAligned(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	for _, width := range []float32{2400, 1200, 1600, 1200} {
		ui.window.Resize(fyne.NewSize(width, 900))
		// The headless canvas does not process queued layout invalidations.
		ui.window.Content().Refresh()
		ui.window.Canvas().Capture()
		for _, scope := range []string{globalFilterScope, graphOutgoing, graphIncoming} {
			bar := ui.scopedFilterBars[scope]
			add, right := bar.Objects[0], bar.Objects[len(bar.Objects)-1]
			if add.Position().X != 0 || add.Position().Y != 0 || right.Position().Y != 0 || right.Position().X+right.Size().Width > bar.Size().Width+1 {
				t.Fatalf("%s actions not aligned at width %v: add %v right %v size %v", scope, width, add.Position(), right.Position(), bar.Size())
			}
			wrapped := false
			for _, object := range bar.Objects[1 : len(bar.Objects)-1] {
				pos, size := object.Position(), object.Size()
				if pos.X < 0 || pos.X+size.Width > bar.Size().Width+1 || pos.Y+size.Height > bar.Size().Height+1 {
					t.Fatalf("%s chip clipped at width %v: %v %v bar %v", scope, width, pos, size, bar.Size())
				}
				if pos.Y > 0 {
					wrapped = true
				} else if pos.X+size.Width > right.Position().X {
					t.Fatal("chip overlaps right-hand controls")
				}
			}
			if scope == graphIncoming && width == 1200 && !wrapped {
				t.Fatal("incoming filters did not wrap")
			}
		}
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_FILTER_WRAP_CAPTURE"))
}

func TestAddFilterCheckboxesStayOpenAndDismissOutside(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	for _, scope := range []string{globalFilterScope, graphOutgoing, graphIncoming} {
		// Also cover the initially empty state, where the check column must exist.
		for _, kind := range availableFilters(scope) {
			ui.visibleFilters[scope][kind] = false
		}
		ui.refreshScopedFilterBars()
		if scope != globalFilterScope {
			ui.graphsScroll.ScrollToOffset(fyne.NewPos(0, ui.graphCards[scope].Position().Y))
		}
		test.Tap(ui.scopedFilterBars[scope].Objects[0].(fyne.Tappable))
		popup := ui.graphMenuPopup
		position := popup.Position()
		for index, kind := range availableFilters(scope) {
			row := popup.Items[index]
			assertLiveVisibilityCheckmark(t, row, false)
			test.Tap(row.(fyne.Tappable))
			if !ui.visibleFilters[scope][kind] || !popup.Visible() || popup.Position() != position || len(ui.window.Canvas().Overlays().List()) != 1 {
				t.Fatalf("%s/%s checkbox closed or moved menu", scope, kind)
			}
			assertLiveVisibilityCheckmark(t, row, true)
		}
		// Keyboard toggles retain the active row and update live checkbox state.
		kind := availableFilters(scope)[len(availableFilters(scope))-1]
		for _, want := range []bool{false, true, false} {
			popup.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
			if ui.visibleFilters[scope][kind] != want || !popup.Visible() {
				t.Fatal("keyboard toggle lost the active checkbox or closed the menu")
			}
			assertLiveVisibilityCheckmark(t, popup.Items[len(popup.Items)-1], want)
		}
		if scope == graphIncoming {
			captureWindowPreview(t, ui.window, os.Getenv("EVELA_FILTER_CHECKBOX_CAPTURE"))
		}
		test.TapCanvas(ui.window.Canvas(), fyne.NewPos(2, 2))
		if popup.Visible() || ui.window.Canvas().Overlays().Top() != nil {
			t.Fatal("clicking away did not dismiss Add filter")
		}
	}
}

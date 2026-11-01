package main

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func chooseGraphMenuItem(t *testing.T, menu *fyne.Menu, label string) {
	t.Helper()
	for _, item := range menu.Items {
		if item.Label == label {
			item.Action()
			return
		}
	}
	t.Fatalf("missing %q in %q", label, menu.Label)
}

func newIncomingGroupingTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	ui, start := newDamageFilterTestUI(t)
	inputs := []string{
		"Listener: Pilot A\n" +
			"[ 2026.10.19 01:59:59 ] (combat) 100 from Alpha[ONE](Raven) - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 20 from Alpha[ONE](Raven) - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 30 from Alpha[ONE](Drake) - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 40 from Bravo[TWO](Raven) - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 50 from Amarr Sentry Gun - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 60 from Guristas Blackbird - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 70 from Unknown attacker - Laser - Hits\n" +
			"[ 2026.10.19 02:00:01 ] (combat) 0 from Amarr Sentry Gun - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 90 to Enemy - Laser - Hits\n",
		"Listener: Pilot B\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 10 from Alpha[ONE](Raven) - Missile - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 20 from Amarr Sentry Gun - Hits\n" +
			"[ 2026.10.19 02:00:00 ] (combat) 10 to Enemy - Laser - Hits\n",
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A", "Pilot B"}, First: start.Add(-time.Second), Last: start.Add(time.Second)}
	for _, input := range inputs {
		log, err := parseLog(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		ui.snapshot.Logs = append(ui.snapshot.Logs, log)
	}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	return ui, start
}

func incomingRates(ui *analyzerUI, bin int) map[string]float64 {
	rates := map[string]float64{}
	for _, series := range ui.incoming.data.Series {
		rates[series.Name] = series.Values[bin]
	}
	return rates
}

func TestIncomingGroupingModesAreIndependentAndPreserveDamage(t *testing.T) {
	ui, _ := newIncomingGroupingTestUI(t)
	outgoing, summary := ui.outgoing.data, ui.summary.Text
	for _, tc := range []struct {
		mode string
		want map[string]float64
	}{
		{incomingByCharacter, map[string]float64{"Pilot A": 270, "Pilot B": 30}},
		{incomingByCharacterGroup, map[string]float64{"Fleet": 300, "Scouts": 270}},
		{incomingByAttacker, map[string]float64{"Alpha[ONE](Raven)": 30, "Alpha[ONE](Drake)": 30, "Bravo[TWO](Raven)": 40, "Other / unresolved": 200}},
		{incomingByShip, map[string]float64{"Raven": 70, "Drake": 30, "Other / unresolved": 200}},
		{incomingByNPC, map[string]float64{"Amarr Sentry Gun": 70, "Guristas Blackbird": 60, "Other / unresolved": 170}},
		{incomingByNPCType, map[string]float64{"Sentry Gun": 70, "Irregular Frigate": 60, "Other / unresolved": 170}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			chooseGraphMenuItem(t, ui.incomingGroupingMenu(), tc.mode)
			if got := incomingRates(ui, 0); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rates=%v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(ui.outgoing.data, outgoing) || ui.summary.Text != summary || filteredTotal(ui) != 100 || npcIncomingTotal(ui) != 300 {
				t.Fatal("incoming grouping changed outgoing data or raw totals")
			}
		})
	}
	chooseGraphMenuItem(t, ui.graphFormatMenu(), "Ship type (ship history)")
	if ui.incomingGrouping != incomingByNPCType || !reflect.DeepEqual(incomingRates(ui, 0), map[string]float64{"Sentry Gun": 70, "Irregular Frigate": 60, "Other / unresolved": 170}) {
		t.Fatal("outgoing grouping overrode the incoming selection")
	}
}

func TestIncomingSourceGroupingFiltersAverageAndRangeHistory(t *testing.T) {
	ui, start := newIncomingGroupingTestUI(t)
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByAttacker)
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	if got := incomingRates(ui, 0); !reflect.DeepEqual(got, map[string]float64{"Alpha[ONE](Raven)": 13, "Alpha[ONE](Drake)": 3, "Bravo[TWO](Raven)": 4, "Other / unresolved": 20}) {
		t.Fatalf("averaging lost event attribution/lookback: %v", got)
	}
	ui.applyGraphRange(start, start.Add(6*time.Second))
	ui.bucketSelect.SetSelected("5 seconds")
	if got := incomingRates(ui, 0); math.Abs(got["Alpha[ONE](Raven)"]-13) > 1e-10 {
		t.Fatalf("coarse bucket average: %v", got)
	}
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	ui.setDamageFilterOption(filterNPCNames, "npc-name:amarr sentry gun", true)
	if got := incomingRates(ui, 0); !reflect.DeepEqual(got, map[string]float64{"Other / unresolved": 7}) || npcIncomingTotal(ui) != 70 || filteredTotal(ui) != 100 {
		t.Fatalf("NPC filter leaked source data: %v", got)
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPCType)
	if got := incomingRates(ui, 0); !reflect.DeepEqual(got, map[string]float64{"Sentry Gun": 7}) {
		t.Fatalf("NPC type grouping: %v", got)
	}
	ui.setDamageFilterOption(filterCharacters, "Pilot B", false)
	if got := incomingRates(ui, 0); !reflect.DeepEqual(got, map[string]float64{"Sentry Gun": 5}) {
		t.Fatalf("character selection leaked source data: %v", got)
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:2", true) // Only Pilot A's later ship, outside this range.
	if len(ui.incoming.data.Series) != 0 {
		t.Fatal("shared event-time ship filter leaked incoming sources")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.panGraphRange(true)
	ui.undoGraphRange()
	if ui.incomingGrouping != incomingByNPCType || !ui.view.Start.Equal(start) {
		t.Fatal("range undo changed incoming grouping")
	}
	if len(ui.snapshot.Logs[0].IncomingSources) != 8 {
		t.Fatal("filtering modified source archive")
	}
}

func TestIncomingGroupingMenusAndPreferences(t *testing.T) {
	ui, _ := newIncomingGroupingTestUI(t)
	if ui.graphFormatButton.Text != "Group By" || ui.incomingGroupButton.Text != "Group By" || ui.graphOptionsButton.Text != "Options" {
		t.Fatal("incorrect dropdown labels")
	}
	for _, item := range ui.graphFormatMenu().Items {
		if strings.HasPrefix(item.Label, "By ") || item.Label == "10-second DPS average" || item.Label == "Target invulnerability indicators" {
			t.Fatal("Group By contains old labels or toggles")
		}
	}
	var labels []string
	for _, item := range ui.incomingGroupingMenu().Items {
		labels = append(labels, item.Label)
	}
	if !reflect.DeepEqual(labels, []string{"Character", "Character Group", "Attacker", "Ship", "NPC", "NPC Type", "Weapon Type", "Weapon Strength", eventByShip, eventByClass, filterSystem}) {
		t.Fatalf("incoming choices: %v", labels)
	}
	if len(ui.graphOptionsMenu().Items) != 4+len(graphVisibilityOptions) {
		t.Fatal("Options must contain the three format toggles, a separator and every graph visibility toggle")
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPC)
	chooseGraphMenuItem(t, ui.graphFormatMenu(), "Ship class (ship history)")
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Target invulnerability indicators")
	ui.window.Resize(fyne.NewSize(1200, 800))
	test.Tap(ui.incomingGroupButton)
	popup := ui.graphMenuPopup
	bounds := ui.window.Canvas().Size()
	position, size := popup.Position(), popup.Size()
	if position.X < 0 || position.Y < 0 || position.X+size.Width > bounds.Width || position.Y+size.Height > bounds.Height {
		t.Fatalf("incoming Group By popup is outside window: position=%v size=%v canvas=%v", position, size, bounds)
	}
	last := popup.Items[len(popup.Items)-1]
	lastPosition := fyne.CurrentApp().Driver().AbsolutePositionForObject(last)
	if lastPosition.Y+last.Size().Height > position.Y+size.Height {
		t.Fatal("last incoming grouping choice is clipped")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_INCOMING_GROUP_CAPTURE"))
	popup.Hide()
	window := fyne.CurrentApp().NewWindow("Restored independent grouping")
	defer window.Close()
	restored := newAnalyzerUI(window)
	if restored.incomingGrouping != incomingByNPC || !restored.shipClassGrouping() || restored.graphAverageWindow == 0 || !restored.graphInvulnerabilityMarkers {
		t.Fatal("independent grouping/options did not persist")
	}
	for _, item := range restored.incomingGroupingMenu().Items {
		if item.Checked != (item.Label == incomingByNPC) {
			t.Fatal("incoming menu checkmark disagrees with saved grouping")
		}
	}
	for _, tc := range []struct {
		saved string
		want  string
	}{
		{"obsolete", incomingByCharacter},
		{"Attacking Character", incomingByCharacter},
		{"Attacking Ship", incomingByAttacker},
		{"Attacking Ship Type", incomingByShip},
	} {
		t.Run(tc.saved, func(t *testing.T) {
			fyne.CurrentApp().Preferences().SetString(incomingGroupingPreference, tc.saved)
			other := fyne.CurrentApp().NewWindow("Restored incoming preference")
			defer other.Close()
			if restored := newAnalyzerUI(other); restored.incomingGrouping != tc.want {
				t.Fatalf("restored grouping=%q, want %q", restored.incomingGrouping, tc.want)
			}
		})
	}
}

func TestAttackerMetadataDoesNotInventPlayerOrShipIdentity(t *testing.T) {
	player := incomingDamageSource("Alpha[ONE](Raven) - Missile - Hits")
	if player.Character != "Alpha" || player.Hull != "Raven" {
		t.Fatalf("missing attacker details: %+v", player)
	}
	structure := incomingDamageSource("Moon - Mining[CORP](Tatara) - Standup Heavy Missile - Hits")
	if structure.Character != "" || structure.Hull != "Tatara" {
		t.Fatalf("structure was identified as a pilot: %+v", structure)
	}
	for _, text := range []string{"Unknown attacker - Laser - Hits", "Amarr Sentry Gun - Hits"} {
		source := incomingDamageSource(text)
		if source.Character != "" || source.Hull != "" {
			t.Fatalf("invented player hull: %+v", source)
		}
	}
}

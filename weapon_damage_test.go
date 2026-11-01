package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDamageWeaponParsing(t *testing.T) {
	cases := []struct {
		message, weapon, strength string
		incoming                  bool
	}{
		{"466 to Blood Raiders Stronghold - Ogre I - Penetrates", "Ogre I", "Penetrates", false},
		{"980 to Lin Kou 02[CACX](Hulk) - Heavy Beam Laser II - Penetrates", "Heavy Beam Laser II", "Penetrates", false},
		{"1399 to Dance King 0[CACX](Hulk) - Heavy Beam Laser II - Smashes", "Heavy Beam Laser II", "Smashes", false},
		{"376 to Guristas Pirates Stronghold - Ogre I - Hits", "Ogre I", "Hits", false},
		{"612 from Pilot - Name[C](Hulk) - Heavy Beam Laser II - Glances Off", "Heavy Beam Laser II", "Glances Off", true},
		{"50 from Amarr Sentry Gun - Hits", "", "Hits", true},
		{"Blood Raiders Apocalypse misses you completely", "", "Misses", true},
		{"Enemy misses you completely - Laser", "Laser", "Misses", true},
		{"Your Ogre I misses Enemy completely", "Ogre I", "Misses", false},
		{"You miss Enemy completely - Heavy Beam Laser II", "Heavy Beam Laser II", "Misses", false},
	}
	for _, c := range cases {
		t.Run(c.message, func(t *testing.T) {
			log, err := parseLog(strings.NewReader("Listener: Pilot\n[ 2026.10.19 02:00:00 ] (combat) " + c.message))
			if err != nil {
				t.Fatal(err)
			}
			if len(log.Hits) != 1 {
				t.Fatalf("hits: %+v", log.Hits)
			}
			hit := log.Hits[0]
			if hit.WeaponType != c.weapon || hit.WeaponStrength != c.strength || hit.Incoming != c.incoming {
				t.Fatalf("hit: %+v", hit)
			}
			if c.strength == "Misses" && len(log.Points) != 0 {
				t.Fatal("miss counted as landed hit")
			}
		})
	}
}

func TestWeaponFiltersAndGrouping(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	messages := []string{
		"466 to Blood Raiders Stronghold - Ogre I - Penetrates",
		"980 to Lin Kou 02[CACX](Hulk) - Heavy Beam Laser II - Penetrates",
		"1399 from Enemy[C](Hulk) - Heavy Beam Laser II - Smashes",
		"376 from Enemy[C](Hulk) - Ogre I - Hits",
		"Enemy[C](Hulk) misses you completely",
		"Your Ogre I misses Enemy completely",
	}
	var input strings.Builder
	input.WriteString("Listener: Pilot A\n")
	for _, message := range messages {
		fmt.Fprintf(&input, "[ 2026.10.19 02:00:00 ] (combat) %s\n", message)
	}
	log, err := parseLog(strings.NewReader(input.String()))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start, Last: start}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	if len(ui.filterOptions[filterWeaponType]) != 3 || len(ui.filterOptions[filterWeaponStrength]) != 4 {
		t.Fatalf("options: %+v", ui.filterOptions)
	}
	chooseGraphMenuItem(t, ui.graphFormatMenu(), filterWeaponType)
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), filterWeaponStrength)
	if len(ui.tableSeries) != 3 {
		t.Fatalf("groups: %+v", ui.tableSeries)
	}
	find := func(name string) characterSeries {
		for _, series := range ui.tableSeries {
			if series.Name == name {
				return series
			}
		}
		t.Fatalf("missing %s", name)
		return characterSeries{}
	}
	if s := find("Ogre I"); s.TotalOutgoing != 466 || s.TotalIncoming != 376 || s.OutgoingHits != 1 || s.IncomingHits != 1 {
		t.Fatalf("ogre: %+v", s)
	}
	// Intersect both weapon fields before aggregating hits in the same second.
	ui.setAllDamageFilterOptions(filterWeaponType, false)
	ui.setDamageFilterOption(filterWeaponType, "name:Heavy Beam Laser II", true)
	ui.setAllDamageFilterOptions(filterWeaponStrength, false)
	ui.setDamageFilterOption(filterWeaponStrength, "name:Smashes", true)
	if len(ui.tableSeries) != 1 || ui.tableSeries[0].TotalIncoming != 1399 || ui.tableSeries[0].TotalOutgoing != 0 {
		t.Fatalf("filtered: %+v", ui.tableSeries)
	}
	ui.setAllDamageFilterOptions(filterWeaponType, true)
	ui.setAllDamageFilterOptions(filterWeaponStrength, false)
	ui.setDamageFilterOption(filterWeaponStrength, "name:Misses", true)
	if len(ui.view.activeSeries()) != 1 {
		t.Fatal("miss-only character disappeared")
	}
	for _, series := range ui.tableSeries {
		if series.TotalIncoming != 0 || series.TotalOutgoing != 0 || series.IncomingHits != 0 || series.OutgoingHits != 0 {
			t.Fatalf("miss totals: %+v", series)
		}
	}
	if len(ui.analysisSnapshot().Logs[0].Hits) != 2 {
		t.Fatal("missing filtered misses")
	}
	// Averages retain their original weapon and conserve raw totals.
	ui.setAllDamageFilterOptions(filterWeaponStrength, true)
	ui.graphAverageWindow = 10 * time.Second
	ui.renderSelection()
	if s := find("Ogre I"); s.TotalOutgoing != 466 || s.TotalIncoming != 376 {
		t.Fatalf("averaged totals: %+v", s)
	}
}

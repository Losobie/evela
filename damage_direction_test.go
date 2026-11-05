package main

import (
	"strings"
	"testing"
	"time"
)

func TestDamageGraphsExcludeOppositeDirectionSeries(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	incoming := "[ 2026.10.19 02:00:00 ] (combat) 27 from Sean Apollo[.N-M.](Vulture) - 'Vehemence' Compact Medium EMP Smartbomb - Hits\n"
	outgoing := "[ 2026.10.19 02:00:00 ] (combat) 912 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates\n"
	for _, input := range []string{incoming, outgoing, incoming + outgoing} {
		log, err := parseLog(strings.NewReader("Listener: Pilot A\n" + input))
		if err != nil {
			t.Fatal(err)
		}
		ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start, Last: start}
		ui.applyGraphRange(start, start.Add(time.Minute))
		ui.rebuildView()
		for _, average := range []time.Duration{0, 10 * time.Second, 30 * time.Second} {
			ui.graphAverageWindow = average
			for _, mode := range []struct{ outgoing, incoming string }{
				{"Characters", incomingByCharacter},
				{characterGroupsMode, incomingByCharacterGroup},
				{"Ship history", eventByShip},
				{"Ship class history", eventByClass},
				{filterSystem, filterSystem},
				{filterWeaponType, filterWeaponType},
				{filterWeaponStrength, filterWeaponStrength},
				{filterTarget, incomingByCharacter},
				{filterTargetType, incomingByCharacter},
			} {
				ui.groupSelect.Selected, ui.incomingGrouping = mode.outgoing, mode.incoming
				ui.renderSelection()
				count := 1
				if mode.outgoing == characterGroupsMode {
					count = 2 // Pilot A belongs to Fleet and Scouts.
				}
				wantOut, wantIn := 0, 0
				if strings.Contains(input, " to ") {
					wantOut = count
				}
				if strings.Contains(input, " from ") {
					wantIn = count
				}
				if len(ui.outgoing.data.Series) != wantOut || len(ui.incoming.data.Series) != wantIn {
					t.Fatalf("opposite-direction series for %s/%s, average %s, input %q", mode.outgoing, mode.incoming, average, input)
				}
				if mode.outgoing == filterWeaponType {
					for _, series := range ui.outgoing.data.Series {
						if series.Name != "425mm Railgun II" {
							t.Fatalf("incoming weapon leaked into outgoing graph: %s", series.Name)
						}
					}
					for _, series := range ui.incoming.data.Series {
						if series.Name != "'Vehemence' Compact Medium EMP Smartbomb" {
							t.Fatalf("outgoing weapon leaked into incoming graph: %s", series.Name)
						}
					}
				}
			}
		}
	}
}

func TestDamageGraphsRetainZeroHitsAndMissesInTheirDirection(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	for _, tc := range []struct {
		message  string
		incoming bool
	}{
		{"0 to Enemy - Ogre I - Hits", false},
		{"Your Ogre I misses Enemy completely", false},
		{"0 from Enemy - Ogre I - Hits", true},
		{"Enemy misses you completely - Ogre I", true},
	} {
		log, err := parseLog(strings.NewReader("Listener: Pilot A\n[ 2026.10.19 02:00:00 ] (combat) " + tc.message))
		if err != nil {
			t.Fatal(err)
		}
		ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start, Last: start}
		ui.applyGraphRange(start, start.Add(time.Minute))
		ui.rebuildView()
		for _, average := range []time.Duration{0, 10 * time.Second, 30 * time.Second} {
			ui.graphAverageWindow = average
			for _, mode := range []struct{ outgoing, incoming string }{{"Characters", incomingByCharacter}, {characterGroupsMode, incomingByCharacterGroup}, {filterWeaponType, filterWeaponType}, {filterWeaponStrength, filterWeaponStrength}} {
				ui.groupSelect.Selected, ui.incomingGrouping = mode.outgoing, mode.incoming
				ui.renderSelection()
				if (len(ui.incoming.data.Series) > 0) != tc.incoming || (len(ui.outgoing.data.Series) > 0) == tc.incoming {
					t.Fatalf("zero-valued event in wrong graph for %s, average %s, message %q", mode.outgoing, average, tc.message)
				}
			}
		}
	}
}

func TestDamageGraphsRetainVisibleAverageTails(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n" +
		"[ 2026.10.19 01:59:59 ] (combat) 100 to Enemy - Ogre I - Hits\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 27 from Enemy - Smartbomb - Hits\n"))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start.Add(-time.Second), Last: start}
	ui.applyGraphRange(start, start.Add(time.Minute))
	for _, average := range []time.Duration{0, 10 * time.Second, 30 * time.Second} {
		ui.graphAverageWindow = average
		for _, mode := range []string{"Characters", characterGroupsMode, "Ship history", filterSystem, filterWeaponType} {
			ui.groupSelect.Selected = mode
			ui.renderSelection()
			if (len(ui.outgoing.data.Series) > 0) != (average > 0) {
				t.Fatalf("incorrect pre-range activity for %s, average %s", mode, average)
			}
			for _, series := range ui.outgoing.data.Series {
				if series.Values[0] != 100/average.Seconds() {
					t.Fatalf("average tail changed: %s = %v", series.Name, series.Values[0])
				}
			}
		}
	}
}

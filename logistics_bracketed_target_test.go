package main

import (
	"strings"
	"testing"
)

const bracketedLogisticsExample = "(combat) 299 remote capacitor transmitted to Basilisk [THIRD] [.AE..] [Clento Loso] - - Large Inductive Compact Remote Capacitor Transmitter"

func TestBracketedLogisticsTargets(t *testing.T) {
	for _, tc := range []struct {
		detail, name, hull, alliance, corporation string
	}{
		{"Basilisk [THIRD] [.AE..] [Clento Loso] - - Large Inductive Compact Remote Capacitor Transmitter", "Clento Loso", "Basilisk", "THIRD", ".AE.."},
		{"Basilisk [THIRD] [.AE..] [Clento Loso] - Large Remote Shield Booster II", "Clento Loso", "Basilisk", "THIRD", ".AE.."},
		{"Basilisk [THIRD] [.AE..] [Clento Loso]", "Clento Loso", "Basilisk", "THIRD", ".AE.."},
		{"Osprey Navy Issue [] [CORP] [Pilot - Name] - - Remote Shield Booster II", "Pilot - Name", "Osprey Navy Issue", "", "CORP"},
		{"Basilisk [] [] [Clento Loso]", "Clento Loso", "Basilisk", "", ""},
	} {
		t.Run(tc.detail, func(t *testing.T) {
			for _, verb := range []string{"capacitor transmitted", "shield boosted", "armor repaired"} {
				event, ok := parseLogistics("299 remote "+verb+" to "+tc.detail, 1)
				if !ok || event.Incoming || event.Amount != 299 || event.Target != tc.name || event.TargetType != tc.hull || event.TargetAllianceTicker != tc.alliance || event.TargetCorporationTicker != tc.corporation || event.TargetCorporation != "" || event.TargetLocation != "" {
					t.Fatalf("incorrect %s target: %+v", verb, event)
				}
				if name, hull := loggedTarget(tc.detail); name != tc.name || hull != tc.hull {
					t.Fatalf("target lookup returned %q / %q", name, hull)
				}
			}
		})
	}
}

func TestBracketedLogisticsTargetFiltersAndGrouping(t *testing.T) {
	ui, start := newTargetTestUI(t)
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n[ " + start.Format(logTimeLayout) + " ] " + bracketedLogisticsExample))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot.Logs = []*parsedLog{log}
	ui.rebuildView()
	for _, tc := range []struct{ mode, key, series string }{
		{filterTarget, "Clento Loso", "Clento Loso · GJ"},
		{filterTargetType, "Basilisk", "Basilisk · GJ"},
	} {
		chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), tc.mode)
		ui.setAllScopeFilters(graphOutgoingLogistics, tc.mode, false)
		if len(ui.outgoingLogistics.data.Series) != 0 {
			t.Fatal("cleared target filter retained logistics")
		}
		ui.setScopeFilter(graphOutgoingLogistics, tc.mode, "name:"+tc.key, true)
		series := ui.outgoingLogistics.data.Series
		if len(series) != 1 || series[0].Name != tc.series || len(series[0].Hits) != 1 || series[0].Hits[0].Damage != 299 {
			t.Fatalf("%s filter / group mismatch: %+v", tc.mode, series)
		}
	}
}

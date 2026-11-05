package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/widget"
)

const customsOfficeRepair = "(combat) 472 remote shield boosted to Customs Office (R3-K7K IX) [Way So Mad] - Large Remote Shield Booster II"

func TestCustomsOfficeLogisticsTargetMetadata(t *testing.T) {
	for _, tc := range []struct {
		detail string
		want   combatEntity
	}{
		{"Customs Office (R3-K7K IX) [Way So Mad] - Large Remote Shield Booster II", combatEntity{Name: "Customs Office", Type: "Customs Office", Location: "R3-K7K IX", Corporation: "Way So Mad"}},
		{"Customs Office (R3-K7K IX) [Way So Mad]", combatEntity{Name: "Customs Office", Type: "Customs Office", Location: "R3-K7K IX", Corporation: "Way So Mad"}},
		{"Customs Office (R3-K7K IX)", combatEntity{Name: "Customs Office", Type: "Customs Office", Location: "R3-K7K IX"}},
		{"Customs Office Gantry (R3-K7K IX) [Corp][TICK](Customs Office Gantry) - Booster", combatEntity{Name: "Customs Office Gantry", Type: "Customs Office Gantry", Location: "R3-K7K IX", Corporation: "Corp", CorporationTicker: "TICK"}},
		{"Pilot - Name[TICK](Hulk) - Large Remote Shield Booster II", combatEntity{Name: "Pilot - Name", Type: "Hulk", CorporationTicker: "TICK"}},
		{skyhookTargetExample + " - Large Remote Shield Booster II", combatEntity{Name: "Orbital Skyhook", Type: "Orbital Skyhook", Location: "R3-K7K IX", Corporation: "Unfinished Symphony", CorporationTicker: "MOVMT"}},
	} {
		t.Run(tc.detail, func(t *testing.T) {
			event, ok := parseLogistics("472 remote shield boosted to "+tc.detail, 1)
			got := combatEntity{Name: event.Target, Type: event.TargetType, Location: event.TargetLocation, Corporation: event.TargetCorporation, CorporationTicker: event.TargetCorporationTicker}
			if !ok || event.Amount != 472 || event.Incoming || got != tc.want {
				t.Fatalf("got %+v, want %+v; event %+v", got, tc.want, event)
			}
		})
	}
	// The fixed target type must also reach filters and grouping.
	ui, start := newTargetTestUI(t)
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n[ " + start.Format(logTimeLayout) + " ] " + customsOfficeRepair))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot.Logs = []*parsedLog{log}
	ui.rebuildView()
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), filterTargetType)
	if len(ui.outgoingLogistics.data.Series) != 1 || ui.outgoingLogistics.data.Series[0].Name != "Customs Office · HP" {
		t.Fatalf("customs office group missing: %+v", ui.outgoingLogistics.data.Series)
	}
	ui.setAllScopeFilters(graphOutgoingLogistics, filterTargetType, false)
	ui.setScopeFilter(graphOutgoingLogistics, filterTargetType, "name:Customs Office", true)
	if len(ui.outgoingLogistics.data.Series) != 1 || ui.outgoingLogistics.data.Series[0].Hits[0].Damage != 472 {
		t.Fatal("customs office target type filter did not retain the repair")
	}
}

func TestEventDetailsExtractedFields(t *testing.T) {
	viewer := &eventsViewer{}
	for _, tc := range []struct {
		message string
		want    []string
		absent  []string
	}{
		{customsOfficeRepair, []string{"Activity: Logistics", "Direction: Outgoing", "Logistics type: Shield repair", "Amount: 472", "Target: Customs Office\n", "Target type: Customs Office", "Target location: R3-K7K IX", "Target corporation: Way So Mad"}, []string{"Target corporation ticker:", "Weapon strength:", "Target type: R3-K7K IX"}},
		{bracketedLogisticsExample, []string{"Amount: 299", "Target: Clento Loso\n", "Target type: Basilisk", "Target corporation ticker: .AE..", "Target alliance ticker: THIRD"}, []string{"Target corporation:", "Target location:"}},
		{unalliedLogisticsExample, []string{"Amount: 0", "Target: Alpha Arterius\n", "Target type: Stork", "Target corporation ticker: 53C0N"}, []string{"Target alliance ticker:", "Target corporation:", "Target location:"}},
		{"(combat) 466 to Blood Raiders Stronghold - Ogre I - Penetrates", []string{"Amount: 466", "Target: Blood Raiders Stronghold\n", "Target type: ♦ Stronghold"}, nil},
		{"(combat) " + skyhookDeflectionExample, []string{"Activity: Damage", "Amount: 60", "Weapon type: Vespa II", "Weapon strength: Glances Off", "Deflection amount: 60", "Target corporation ticker: MOVMT"}, nil},
		{"(combat) Blood Raiders Apocalypse misses you completely", []string{"Direction: Incoming", "Amount: 0", "Weapon strength: Misses", "Attacker: Blood Raiders Apocalypse"}, []string{"Deflection amount:", "Target type:"}},
		{"(combat) Amarr Sentry Gun misses you completely", []string{"Attacker NPC: true", "Attacker type ID: 1194", "Attacker group ID: 99"}, nil},
		{"(combat) 908 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates", []string{"Amount: 908", "Target: Grijawlva", "Target type: Phoenix Navy Issue", "Target corporation ticker: SNOOO"}, []string{"Target location:", "Target corporation:"}},
		{"(combat) 10 from Pilot (System) [Corp][TICK](Hulk) - Ogre I - Hits", []string{"Attacker character: Pilot", "Attacker type: Hulk", "Attacker location: System", "Attacker corporation: Corp", "Attacker corporation ticker: TICK"}, nil},
		{"(combat) 50 remote armor repaired by Friend[C](Guardian)", []string{"Direction: Incoming", "Logistics type: Armor repair", "Amount: 50"}, []string{"Target:"}},
		{"(combat) Warp scramble attempt from Enemy to you!", []string{"Activity: E-War", "Effect: Warp scramble"}, []string{"Amount:"}},
		{"(combat) 50 GJ energy neutralized Enemy - Neutralizer", []string{"Effect: Energy neutralizer", "Capacitor drained (GJ): 50"}, nil},
		{"(mining) You mined 12 units of Veldspar with a lost residue of 3 units.", []string{"Activity: Mining", "Message type: Ore mined", "Ore: Veldspar", "Amount: 12", "Message type: Waste", "Amount: 3"}, nil},
		{"(notify) Jumping from Mamet to Hoshoun", []string{"Activity: Travel", "From system: Mamet", "To system: Hoshoun"}, nil},
		{"(chat) 50 remote shield boosted to Pilot[C](Hulk)", []string{"Channel: chat", "Activity fields: None extracted"}, []string{"Activity: Logistics", "Target:"}},
		{"(combat) 1 to A <literal> &amp; B[T](Hulk) - Ogre I - Hits", []string{"Target: A <literal> &amp; B"}, nil},
	} {
		t.Run(tc.message, func(t *testing.T) {
			text := formatExtractedFields(viewer.eventFields(eventRow{logEvent: logEvent{Second: 1, Message: tc.message}}))
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in:\n%s", want, text)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(text, absent) {
					t.Errorf("unexpected %q in:\n%s", absent, text)
				}
			}
		})
	}
}

func TestEventDetailsDialogAndContext(t *testing.T) {
	ui, start := newEventsTestUI(t)
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n" +
		"[ " + start.Format(logTimeLayout) + " ] " + customsOfficeRepair + "\n" +
		"[ " + start.Format(logTimeLayout) + " ] (combat) " + skyhookDeflectionExample + "\n" +
		"[ " + start.Add(time.Second).Format(logTimeLayout) + " ] (notify) Target is invulnerable.\n"))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Logs: []*parsedLog{log}, Characters: []string{"Pilot A"}}
	ui.shipHistory = shipHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", TypeName: "Basilisk", FirstSeen: start, LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot A", TypeName: "Stork", FirstSeen: start.Add(time.Second), LastSeen: start.Add(time.Second)},
	}}
	ui.locationHistory = locationHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", SystemName: "Jita", FirstSeen: start, LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot A", SystemName: "Amarr", FirstSeen: start.Add(time.Second), LastSeen: start.Add(time.Second)},
	}}
	ui.rebuildView()
	ui.tabs.SelectIndex(2)
	viewer := ui.events
	if len(viewer.rows) != 3 {
		t.Fatalf("fixture lost events: %+v", viewer.rows)
	}
	for i, want := range []string{"Current ship: Basilisk\nCurrent system: Jita\n\nExtracted fields", "Current ship: Basilisk\nCurrent system: Jita\n\nExtracted fields", "Current ship: Stork\nCurrent system: Amarr\n\nExtracted fields"} {
		if text := viewer.eventDetails(viewer.rows[i]); !strings.Contains(text, want) {
			t.Fatalf("event %d has incorrect historical context or field order: %s", i, text)
		}
	}
	outside := eventRow{logEvent: logEvent{Second: start.Add(-time.Second).Unix(), Message: "(chat) Before tracking"}, Character: "Pilot A"}
	if text := viewer.eventDetails(outside); !strings.Contains(text, "Current ship: Unknown ship\nCurrent system: Unknown system\n\nExtracted fields") {
		t.Fatalf("event outside history acquired a current ship / system: %s", text)
	}
	if text := viewer.eventDetails(viewer.rows[0]); !strings.Contains(text, "Target corporation: Way So Mad") || strings.Contains(text, "Weapon strength:") {
		t.Fatalf("same-second logistics details were misattributed: %s", text)
	}
	if text := viewer.eventDetails(viewer.rows[1]); !strings.Contains(text, "Deflection amount: 60") || strings.Contains(text, "Logistics type:") {
		t.Fatalf("same-second damage details were misattributed: %s", text)
	}
	if text := viewer.eventDetails(viewer.rows[2]); !strings.Contains(text, "Activity: Structure invulnerability") || !strings.Contains(text, "Target type: Orbital Skyhook") {
		t.Fatalf("contextual structure fields missing: %s", text)
	}
	viewer.table.OnSelected(widget.TableCellID{Row: 1, Col: 2})
	if ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("event details dialog did not open")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_EVENT_DETAILS_CAPTURE"))
}

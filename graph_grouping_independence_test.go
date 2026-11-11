package main

import (
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestGroupingNeverChangesOtherGraphs(t *testing.T) {
	for _, average := range []time.Duration{0, 10 * time.Second} {
		t.Run(average.String(), func(t *testing.T) {
			ui, _ := newMiningVolumeTestUI(t)
			support, err := parseLog(strings.NewReader("Listener: Miner\n" +
				"[ 2026.11.11 02:00:01 ] (combat) 300 from Enemy[C](Hulk) - Laser - Hits\n" +
				"[ 2026.11.11 02:00:01 ] (combat) 50 remote armor repaired to Friend\n" +
				"[ 2026.11.11 02:00:01 ] (combat) 60 remote shield boosted by Friend\n" +
				"[ 2026.11.11 02:00:01 ] (combat) 10 GJ energy neutralized Enemy - Neut\n"))
			if err != nil {
				t.Fatal(err)
			}
			ui.snapshot.SourceFiles["support.txt"] = cachedLog{Log: support}
			ui.snapshot.populateLogs()
			ui.selectQuickRange("All history")
			for _, group := range []characterGroup{{Name: "Miners", Members: []string{"Miner"}}, {Name: "Absent", Members: []string{"Other Pilot"}}} {
				if _, err := ui.saveCharacterGroup(group); err != nil {
					t.Fatal(err)
				}
			}
			ui.graphAverageWindow = average
			ui.tabs.SelectIndex(2)
			reset := func() {
				ui.groupSelect.SetSelected("Characters")
				ui.incomingGrouping = incomingByCharacter
				for _, id := range []string{graphIncomingLogistics, graphOutgoingLogistics, graphIncomingEWar, graphMining} {
					ui.eventGraphGroupings[id] = eventGroupingOptions(id)[0]
				}
				ui.miningVolume = false
				ui.renderSelection()
			}
			plots := func() map[string]plotData {
				return map[string]plotData{graphOutgoing: ui.outgoing.data, graphIncoming: ui.incoming.data,
					graphOutgoingLogistics: ui.outgoingLogistics.data, graphIncomingLogistics: ui.incomingLogistics.data,
					graphIncomingEWar: ui.ewar.data, graphMining: ui.mining.data}
			}
			reset()
			baseline := plots()
			for id, data := range baseline {
				if len(data.Series) == 0 {
					t.Fatalf("fixture has no %s data", id)
				}
			}
			selection := maps.Clone(ui.selected)
			summary, totals := ui.summary.Text, ui.miningTotals
			events := append([]eventRow(nil), ui.events.rows...)
			if len(events) == 0 {
				t.Fatal("fixture has no Events rows")
			}
			for _, graph := range graphVisibilityOptions {
				var menu *fyne.Menu
				switch graph.id {
				case graphOutgoing:
					menu = ui.graphFormatMenu()
					for _, name := range []string{"Miners", "Absent"} {
						name := name
						menu.Items = append(menu.Items, fyne.NewMenuItem("Group: "+name, func() { ui.groupSelect.SetSelected("Group: " + name) }))
					}
				case graphIncoming:
					menu = ui.incomingGroupingMenu()
				default:
					menu = ui.eventGroupingMenu(graph.id)
				}
				for _, item := range menu.Items {
					if item.IsSeparator {
						continue
					}
					reset()
					item.Action()
					for id, data := range plots() {
						if id != graph.id && !reflect.DeepEqual(data, baseline[id]) {
							t.Fatalf("%s grouping %q changed %s", graph.id, item.Label, id)
						}
					}
					if !maps.Equal(selection, ui.selected) || summary != ui.summary.Text || totals != ui.miningTotals || !reflect.DeepEqual(events, ui.events.rows) {
						t.Fatalf("%s grouping %q changed global selection, summary, mining totals or Events", graph.id, item.Label)
					}
					if graph.id == graphOutgoing && item.Label == "Group: Absent" && (len(ui.outgoing.data.Series) != 0 || len(ui.mining.data.Series) == 0) {
						t.Fatal("empty outgoing group must hide only outgoing damage")
					}
				}
			}
		})
	}
}

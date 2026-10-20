package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestCharacterFiltersFollowDamageWindow(t *testing.T) {
	for _, grouping := range []string{"Characters", "Ship history"} {
		t.Run(grouping, func(t *testing.T) {
			application := test.NewTempApp(t)
			window := application.NewWindow("Character filters")
			defer window.Close()
			ui := newAnalyzerUI(window)
			start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
			ui.snapshot = &logSnapshot{Characters: []string{"Outgoing", "Incoming", "Outside", "Earlier", "Zero hit", "Empty"}, First: start.Add(-time.Second), Last: start.Add(10 * time.Second), Logs: []*parsedLog{
				{Character: "Outgoing", Points: []damagePoint{{Second: start.Unix(), Outgoing: 10, OutgoingHits: 1}}},
				{Character: "Incoming", Points: []damagePoint{{Second: start.Unix() + 5, Incoming: 20, IncomingHits: 1}}},
				{Character: "Outside", Points: []damagePoint{{Second: start.Unix() + 10, Outgoing: 30, OutgoingHits: 1}}},
				{Character: "Earlier", Points: []damagePoint{{Second: start.Unix() - 1, Incoming: 40, IncomingHits: 1}}},
				{Character: "Zero hit", Points: []damagePoint{{Second: start.Unix() + 3, OutgoingHits: 1}}},
				{Character: "Empty"},
			}}
			ui.selected = map[string]bool{"Outgoing": false, "Incoming": true, "Outside": false, "Earlier": true, "Zero hit": true, "Empty": false}
			ui.applyGraphRange(start, start.Add(10*time.Second))
			applyTestShipRecords(ui, []shipRecord{
				{CharacterName: "Outgoing", TypeID: 1, TypeName: "Loki", CheckedAt: time.Now()},
				{CharacterName: "Incoming", TypeID: 1, TypeName: "Loki", CheckedAt: time.Now()},
				{CharacterName: "Outside", TypeID: 1, TypeName: "Loki", CheckedAt: time.Now()},
				{CharacterName: "Earlier", TypeID: 2, TypeName: "Rifter", CheckedAt: time.Now()},
				{CharacterName: "Zero hit", TypeID: 3, TypeName: "Tengu", CheckedAt: time.Now()},
			})
			ui.groupSelect.SetSelected(grouping)
			window.SetContent(ui.content())
			window.Resize(fyne.NewSize(1440, 960))
			window.Show()
			assertChecks := func(want map[string]bool) {
				t.Helper()
				if got := characterFilterChecks(ui.characters); !reflect.DeepEqual(got, want) {
					t.Fatalf("visible filters = %v, want %v", got, want)
				}
			}
			assertChecks(map[string]bool{"Outgoing": false, "Incoming": true, "Zero hit": true})
			if !strings.HasPrefix(ui.summary.Text, "2 selected") {
				t.Fatalf("out-of-range selections counted in summary: %s", ui.summary.Text)
			}
			if len(ui.tableSeries) != 2 || len(ui.outgoing.data.Series) != 2 {
				t.Fatal("inactive characters or ships remained in totals or plots")
			}
			if grouping == "Ship history" {
				var headers []string
				walkFilterObjects(ui.characters, func(object fyne.CanvasObject) {
					if label, ok := object.(*widget.Label); ok && label.TextStyle.Bold {
						headers = append(headers, label.Text)
					}
				})
				if !reflect.DeepEqual(headers, []string{"Loki (2)", "Tengu (1)"}) {
					t.Fatalf("inactive members affected ship headers: %v", headers)
				}
				// Selecting Loki must not select its out-of-range member.
				walkFilterObjects(ui.characters.Objects[0], func(object fyne.CanvasObject) {
					if button, ok := object.(*widget.Button); ok && button.Text == "Select" {
						test.Tap(button)
					}
				})
				if ui.selected["Outside"] || !ui.selected["Outgoing"] {
					t.Fatal("ship selection changed hidden characters or skipped visible members")
				}
			}
			ui.characterSearch.SetText("Outside")
			assertChecks(map[string]bool{})
			ui.characterSearch.SetText("Incoming")
			assertChecks(map[string]bool{"Incoming": true})
			ui.characterSearch.SetText("")
			ui.selectRangeCharacters(false)
			assertChecks(map[string]bool{"Outgoing": false, "Incoming": false, "Zero hit": false})
			if !ui.selected["Earlier"] || ui.selected["Outside"] || ui.selected["Empty"] {
				t.Fatal("None changed selections outside the current window")
			}
			ui.selectRangeCharacters(true)
			assertChecks(map[string]bool{"Outgoing": true, "Incoming": true, "Zero hit": true})
			if !ui.selected["Earlier"] || ui.selected["Outside"] || ui.selected["Empty"] {
				t.Fatal("All changed selections outside the current window")
			}
			ui.applyGraphRange(start.Add(-5*time.Second), start.Add(15*time.Second))
			wide := map[string]bool{"Outgoing": true, "Incoming": true, "Zero hit": true, "Earlier": true, "Outside": false}
			assertChecks(wide)
			ui.undoGraphRange()
			assertChecks(map[string]bool{"Outgoing": true, "Incoming": true, "Zero hit": true})
			ui.redoGraphRange()
			assertChecks(wide)
			ui.applyGraphRange(start.Add(20*time.Second), start.Add(30*time.Second))
			assertChecks(map[string]bool{})
			if len(ui.outgoing.data.Series) != 0 || len(ui.tableSeries) != 0 || !strings.HasPrefix(ui.summary.Text, "0 selected") {
				t.Fatal("empty window still has plotted characters or totals")
			}
			if label, ok := ui.characters.Objects[0].(*widget.Label); !ok || label.Text != "No damage data in this time range" {
				t.Fatal("empty window does not explain the missing filters")
			}
			ui.undoGraphRange()
			assertChecks(wide)
			ui.snapshot.Logs[5].Points = append(ui.snapshot.Logs[5].Points, damagePoint{Second: start.Unix() + 4, Incoming: 50, IncomingHits: 1})
			ui.updateRange() // A refresh discovers damage for a previously inactive character.
			wide["Empty"] = false
			assertChecks(wide)
		})
	}
}

func characterFilterChecks(root fyne.CanvasObject) map[string]bool {
	checks := map[string]bool{}
	walkFilterObjects(root, func(object fyne.CanvasObject) {
		if check, ok := object.(*widget.Check); ok {
			checks[check.Text] = check.Checked
		}
	})
	return checks
}

func walkFilterObjects(root fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	visit(root)
	if container, ok := root.(*fyne.Container); ok {
		for _, child := range container.Objects {
			walkFilterObjects(child, visit)
		}
	}
}

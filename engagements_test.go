package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func engagementTestSnapshot(t *testing.T) (*logSnapshot, time.Time) {
	t.Helper()
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	parse := func(character string, messages map[int]string) *parsedLog {
		var text strings.Builder
		text.WriteString("Listener: " + character + "\n")
		// Deliberately reverse the input: parsing guarantees sorted streams.
		for second := 4000; second >= 0; second-- {
			if message, ok := messages[second]; ok {
				text.WriteString("[ " + start.Add(time.Duration(second)*time.Second).Format(logTimeLayout) + " ] " + message + "\n")
			}
		}
		log, err := parseLog(strings.NewReader(text.String()))
		if err != nil {
			t.Fatal(err)
		}
		return log
	}
	a := parse("Pilot A", map[int]string{
		0:    "(notify) Jumping from Mamet to Hoshoun",
		1:    "(combat) 908 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates",
		60:   "(combat) Blood Raiders Apocalypse misses you completely",
		300:  "(chat) Chat is not activity",
		600:  "(notify) Jumping from Hoshoun to Mai",
		659:  "(combat) 20 remote armor repaired to Ally[CORP](Hulk)",
		1258: "(mining) You mined 12 units of Veldspar.",
		1500: "(chat) Chat cannot bridge an idle gap",
		1858: "(combat) 100 from Enemy[CORP](Raven) - Missile - Hits",
		4000: "(notify) Jumping from Mai to Mamet",
	})
	b := parse("Pilot B", map[int]string{
		700:  "(combat) Warp scramble attempt from Enemy to you!",
		3100: "(combat) 50 remote shield boosted by Ally",
	})
	return &logSnapshot{Logs: []*parsedLog{b, a}, Characters: []string{"Pilot A", "Pilot B"}, First: start.Add(time.Second), Last: start.Add(3100 * time.Second)}, start
}

func TestEngagementsActivityGapsAndSummaries(t *testing.T) {
	snapshot, start := engagementTestSnapshot(t)
	locations := newLocationResolver(nil, snapshot.locationSessions())
	rows, err := buildEngagements(context.Background(), snapshot, locations)
	if err != nil {
		t.Fatal(err)
	}
	// A 599-second gap stays together across activity kinds. Exactly 600
	// seconds splits, despite intervening chat and later movement messages.
	if len(rows) != 3 {
		t.Fatalf("got %d engagements: %+v", len(rows), rows)
	}
	for i, bounds := range [][2]int{{3100, 3101}, {1858, 1859}, {1, 1259}} {
		if !rows[i].Start.Equal(start.Add(time.Duration(bounds[0])*time.Second)) || !rows[i].End.Equal(start.Add(time.Duration(bounds[1])*time.Second)) {
			t.Fatalf("engagement %d bounds = %+v", i, rows[i])
		}
	}
	for _, text := range []string{"Hoshoun", "Mai", "damage dealt", "damage received", "E-War", "outgoing logistics", "mining", "Pilot A", "Pilot B", "Phoenix Navy Issue", "Warp scramble", "Veldspar"} {
		if !strings.Contains(rows[2].Summary, text) {
			t.Errorf("summary missing %q: %s", text, rows[2].Summary)
		}
	}
	if !strings.Contains(rows[0].Summary, "Unknown system") || !strings.Contains(rows[0].Summary, "incoming logistics") {
		t.Fatalf("logistics-only engagement = %+v", rows[0])
	}
	// Reversing the logs must not change ordering or summaries for ties.
	snapshot.Logs[0], snapshot.Logs[1] = snapshot.Logs[1], snapshot.Logs[0]
	again, _ := buildEngagements(context.Background(), snapshot, locations)
	for i := range rows {
		if rows[i] != again[i] {
			t.Fatalf("log order changed engagement %d", i)
		}
	}
}

func TestEngagementsEmptyMissesAndCancellation(t *testing.T) {
	rows, err := buildEngagements(context.Background(), &logSnapshot{Logs: []*parsedLog{{Events: []logEvent{{Second: 1, Message: "chat"}}}}}, locationResolver{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("non-activity produced engagements: %v, %v", rows, err)
	}
	snapshot := &logSnapshot{Logs: []*parsedLog{{Character: "Pilot", Hits: []damageHit{{Second: 1, Incoming: true, Source: -1, WeaponStrength: "Misses"}}}}}
	rows, err = buildEngagements(context.Background(), snapshot, locationResolver{})
	if err != nil || len(rows) != 1 || rows[0].End.Sub(rows[0].Start) != time.Second || !strings.Contains(rows[0].Summary, "damage received") {
		t.Fatalf("miss-only engagement lost: %v, %v", rows, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildEngagements(ctx, snapshot, locationResolver{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled build returned %v", err)
	}
}

func waitForEngagements(t *testing.T, ui *analyzerUI) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ready := false
		fyne.DoAndWait(func() { ready = !ui.engagements.loading && !ui.engagements.dirty })
		if ready {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("engagement build did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEngagementsLazyLoadingNavigationAndRefresh(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Engagements")
	ui := newAnalyzerUI(window)
	t.Cleanup(func() { ui.engagements.cancelBuild(); window.Close() })
	ui.snapshot, _ = engagementTestSnapshot(t)
	ui.rangeSelect.SetSelected("Last 10 minutes")
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	viewer := ui.engagements
	if viewer.table != nil || viewer.loading || len(viewer.rows) != 0 {
		t.Fatal("hidden Engagements eagerly initialized or processed the archive")
	}
	ui.tabs.SelectIndex(3)
	waitForEngagements(t, ui)
	if len(viewer.rows) != 3 {
		t.Fatalf("list followed the graph's narrow range: %+v", viewer.rows)
	}
	captureWindowPreview(t, window, os.Getenv("EVELA_ENGAGEMENTS_CAPTURE"))
	generation := viewer.generation
	row := viewer.rows[2]
	before := graphHistoryState(ui)
	cell := viewer.table.CreateCell().(*engagementCell)
	viewer.table.UpdateCell(widget.TableCellID{Row: 3, Col: 2}, cell)
	cell.Tapped(&fyne.PointEvent{})
	if ui.tabs.SelectedIndex() != 3 || !strings.Contains(viewer.selection.Text, "Veldspar") {
		t.Fatal("single-click navigated instead of showing the summary")
	}
	cell.DoubleTapped(&fyne.PointEvent{})
	if ui.tabs.SelectedIndex() != 0 || !ui.view.Start.Equal(row.Start) || !ui.view.End.Equal(row.End) || ui.rangeSelect.Selected != "Custom" {
		t.Fatal("double-click did not open the engagement's complete range in Graphs")
	}
	ui.undoGraphRange()
	if graphHistoryState(ui) != before {
		t.Fatal("engagement navigation was not undoable")
	}
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.tabs.SelectIndex(3)
	if viewer.generation != generation || len(viewer.rows) != 3 || viewer.loading {
		t.Fatal("range / filter navigation rebuilt or filtered the engagement cache")
	}
	// Recycled header cells must never retain a row's double-click callback.
	viewer.table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	cell.DoubleTapped(&fyne.PointEvent{})
	if ui.tabs.SelectedIndex() != 3 {
		t.Fatal("header reused an obsolete engagement callback")
	}
	// Location updates also refresh summaries when the log snapshot is unchanged.
	ui.locationHistory = locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot B", SystemName: "Jita", SystemID: 30000142, FirstSeen: viewer.rows[0].Start, LastSeen: viewer.rows[0].End}}}
	viewer.invalidate()
	ui.refreshEngagements()
	waitForEngagements(t, ui)
	if !strings.Contains(viewer.rows[0].Summary, "Jita") {
		t.Fatal("location-history refresh left a stale system summary")
	}
	ui.tabs.SelectIndex(0)
	ui.snapshot = &logSnapshot{}
	ui.rebuildView()
	if viewer.snapshot != nil || viewer.loading {
		t.Fatal("hidden refresh retained an obsolete archive or started work")
	}
	ui.tabs.SelectIndex(3)
	waitForEngagements(t, ui)
	if len(viewer.rows) != 0 || !strings.Contains(viewer.status.Text, "No combat") {
		t.Fatal("new logs did not replace the cached list")
	}
}

func TestEngagementBuildCancellationRejectsStaleResults(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Engagement cancellation")
	ui := newAnalyzerUI(window)
	t.Cleanup(window.Close)
	window.SetContent(ui.content())
	ui.snapshot, _ = engagementTestSnapshot(t)
	// Start and leave in the same UI turn, before a worker can publish.
	fyne.DoAndWait(func() {
		ui.tabs.SelectIndex(3)
		ui.tabs.SelectIndex(0)
	})
	if ui.engagements.loading || ui.engagements.cancel != nil {
		t.Fatal("leaving the tab did not cancel unfinished work")
	}
	ui.snapshot = &logSnapshot{}
	ui.tabs.SelectIndex(3)
	waitForEngagements(t, ui)
	if len(ui.engagements.rows) != 0 {
		t.Fatal("a stale worker replaced the latest archive")
	}
}

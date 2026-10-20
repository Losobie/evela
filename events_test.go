package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/encoding/unicode"
)

const eventsLog = "Listener: Pilot A\n" +
	"[ 2026.10.19 02:00:02 ] (notify) Jumping from Mamet to Hoshoun\n" +
	"[ 2026.10.19 01:59:59 ] (info) Before range\n" +
	"[ 2026.10.19 02:00:00 ] (combat) <color=0xffffffff><b>10</b> from Enemy - Laser - Hits\n" +
	"[ 2026.10.19 02:00:01 ] (combat) Enemy misses you completely - Laser\n" +
	"[ 2026.10.19 02:00:01 ] (combat) 20 remote armor repaired to Ally\n" +
	"[ 2026.10.19 02:00:03 ] (chat) Hello &amp; goodbye [literal] .*\n" +
	"[ 2026.10.19 02:00:04 ] Message without a channel\n" +
	"[ 2026.10.19 02:00:10 ] (info) At exclusive end\n" +
	"[ 2026.99.19 02:00:05 ] (combat) Invalid timestamp\n" +
	"Not an event\n"

func TestParseAllLogEventsAcrossEncodings(t *testing.T) {
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			data := []byte(eventsLog)
			if encoding == "utf16le" {
				data, _ = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			if encoding == "utf16be" {
				data, _ = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			log, err := parseLog(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(log.Events) != 8 {
				t.Fatalf("expected every valid timestamped message, got %d", len(log.Events))
			}
			if log.Events[1].Message != "(combat) 10 from Enemy - Laser - Hits" || log.Events[5].Message != "(chat) Hello & goodbye [literal] .*" {
				t.Fatalf("message/channel formatting was lost: %+v", log.Events)
			}
			if log.Events[2].Message != "(combat) Enemy misses you completely - Laser" || log.Events[3].Message != "(combat) 20 remote armor repaired to Ally" {
				t.Fatal("same-second messages were combined or reordered")
			}
			for i := 1; i < len(log.Events); i++ {
				if log.Events[i].Second < log.Events[i-1].Second {
					t.Fatal("events were not sorted chronologically")
				}
			}
			if len(log.Points) != 1 || log.Points[0].Incoming != 10 || log.Points[0].IncomingHits != 1 {
				t.Fatal("retaining all events changed damage parsing")
			}
		})
	}
}

func newEventsTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	ui, start := newDamageFilterTestUI(t)
	log, err := parseLog(strings.NewReader(eventsLog))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start.Add(-time.Second), Last: start.Add(10 * time.Second)}
	ui.applyGraphRange(start, start.Add(10*time.Second))
	return ui, start
}

func TestEventsTabRangeSortingAndMessageSearch(t *testing.T) {
	ui, start := newEventsTestUI(t)
	var tabs []string
	for _, tab := range ui.tabs.Items {
		tabs = append(tabs, tab.Text)
	}
	if !reflect.DeepEqual(tabs, []string{"Damage graphs", "Damage totals", "Events", "ESI & ships"}) {
		t.Fatalf("incorrect tab order: %v", tabs)
	}
	viewer := ui.events
	rows, columns := viewer.table.Length()
	if len(viewer.rows) != 6 || rows != 7 || columns != 3 || viewer.rows[0].Second != start.Unix() || viewer.rows[5].Second != start.Unix()+4 {
		t.Fatalf("incorrect time bounds/table dimensions: %+v", viewer.rows)
	}
	viewer.table.OnSelected(widget.TableCellID{Row: 0, Col: 0})
	if !viewer.descending || viewer.rows[0].Second != start.Unix()+4 {
		t.Fatal("Timestamp did not switch to newest-first")
	}
	viewer.table.OnSelected(widget.TableCellID{Row: 0, Col: 0})
	if viewer.descending || viewer.rows[0].Second != start.Unix() {
		t.Fatal("Timestamp did not switch back to oldest-first")
	}
	label := viewer.table.CreateCell().(*widget.Label)
	viewer.table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, label)
	if label.Text != "2026-10-19 02:00:00" {
		t.Fatalf("incorrect EVE time: %s", label.Text)
	}
	viewer.table.UpdateCell(widget.TableCellID{Row: 1, Col: 1}, label)
	if label.Text != "Pilot A" {
		t.Fatalf("character column did not identify the log listener: %s", label.Text)
	}
	viewer.table.UpdateCell(widget.TableCellID{Row: 0, Col: 1}, label)
	if label.Text != "Character" {
		t.Fatalf("incorrect character heading: %s", label.Text)
	}
	viewer.table.UpdateCell(widget.TableCellID{Row: 1, Col: 2}, label)
	if label.Text != "(combat) 10 from Enemy - Laser - Hits" {
		t.Fatal("message column was not the rest of the log line")
	}
	viewer.search.SetText("JUMPING")
	if len(viewer.rows) != 1 || !strings.Contains(viewer.rows[0].Message, "Hoshoun") {
		t.Fatal("plain message search did not ignore case")
	}
	viewer.search.SetText("2026-10-19")
	if len(viewer.rows) != 0 {
		t.Fatal("message search included timestamps")
	}
	viewer.search.SetText(".*")
	if len(viewer.rows) != 1 {
		t.Fatal("plain search treated regex metacharacters as a pattern")
	}
	test.Tap(viewer.regex)
	if len(viewer.rows) != 6 {
		t.Fatal("Regex checkbox did not enable expressions")
	}
	viewer.search.SetText(`^\(combat\).* (misses|repaired)\b`)
	if len(viewer.rows) != 2 {
		t.Fatalf("regex alternation/anchors failed: %+v", viewer.rows)
	}
	viewer.search.SetText("JUMPING")
	if len(viewer.rows) != 0 {
		t.Fatal("regex matching should be case sensitive without a flag")
	}
	viewer.search.SetText("(?i)jumping.*hoshoun")
	if len(viewer.rows) != 1 {
		t.Fatal("inline case-insensitive regex did not match")
	}
	viewer.search.SetText("[")
	if viewer.search.Validate() == nil || len(viewer.rows) != 0 || !strings.Contains(viewer.status.Text, "Invalid regular expression") {
		t.Fatal("invalid regex did not show an error and clear results")
	}
	test.Tap(viewer.regex)
	if viewer.search.Validate() != nil || len(viewer.rows) != 1 {
		t.Fatal("disabling regex did not restore literal matching")
	}
	viewer.search.SetText("")
	viewer.table.OnSelected(widget.TableCellID{Row: 0, Col: 0})
	ui.setAllDamageFilterOptions(filterCharacters, false)
	ui.setAllDamageFilterOptions(filterShipType, false)
	if len(viewer.rows) != 6 {
		t.Fatal("damage filters hid non-damage log events")
	}
	ui.applyGraphRange(start.Add(time.Second), start.Add(3*time.Second))
	if len(viewer.rows) != 3 || !viewer.descending {
		t.Fatal("Events did not follow the time range while keeping sort order")
	}
	ui.undoGraphRange()
	if len(viewer.rows) != 6 {
		t.Fatal("range undo did not restore Events")
	}
}

func TestEventsRefreshPreferencesAndPreview(t *testing.T) {
	ui, start := newEventsTestUI(t)
	viewer := ui.events
	viewer.regex.SetChecked(true)
	viewer.search.SetText(`(?i)jumping|hello`)
	viewer.table.OnSelected(widget.TableCellID{Row: 0, Col: 0})
	log := *ui.snapshot.Logs[0]
	log.Events = append(append([]logEvent(nil), log.Events...), logEvent{Second: start.Unix() + 5, Message: "(notify) HELLO from a new event"})
	// Place the new entry before the event at the exclusive upper bound.
	log.Events[len(log.Events)-1], log.Events[len(log.Events)-2] = log.Events[len(log.Events)-2], log.Events[len(log.Events)-1]
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{&log}}
	ui.updateRange()
	if len(viewer.rows) != 3 || !viewer.regex.Checked || viewer.search.Text != `(?i)jumping|hello` || !viewer.descending || viewer.rows[0].Second != start.Unix()+5 {
		t.Fatal("refresh lost Events search/sort or failed to include a new event")
	}
	ui.tabs.SelectIndex(2)
	ui.window.Resize(fyne.NewSize(1440, 960))
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_EVENTS_CAPTURE"))
	viewer.table.OnSelected(widget.TableCellID{Row: 1, Col: 2})
	if ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("selecting a message did not expose the full text")
	}
	window := fyne.CurrentApp().NewWindow("Saved regex setting")
	defer window.Close()
	if restored := newAnalyzerUI(window); !restored.events.regex.Checked {
		t.Fatal("Regex preference did not persist enabled state")
	}
	viewer.regex.SetChecked(false)
	other := fyne.CurrentApp().NewWindow("Saved literal setting")
	defer other.Close()
	if restored := newAnalyzerUI(other); restored.events.regex.Checked {
		t.Fatal("Regex preference did not persist disabled state")
	}
}

func TestEventDuplicateExportsRetainDistinctMessages(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"original.txt", "copy.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(eventsLog), 0600); err != nil {
			t.Fatal(err)
		}
	}
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Duplicates != 1 || len(snapshot.Logs) != 1 || len(snapshot.Logs[0].Events) != 8 {
		t.Fatal("duplicate exports duplicated event rows")
	}
	if err := os.WriteFile(filepath.Join(directory, "another.txt"), []byte(strings.Replace(eventsLog, "Pilot A", "Pilot B", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, log := range snapshot.Logs {
		count += len(log.Events)
	}
	if count != 16 {
		t.Fatal("events from distinct logs were incorrectly deduplicated")
	}
	application := test.NewTempApp(t)
	window := application.NewWindow("Events from multiple characters")
	defer window.Close()
	viewer := newEventsViewer(window)
	start := time.Date(2026, 10, 19, 1, 59, 0, 0, time.UTC)
	viewer.setRange(snapshot, start, start.Add(time.Hour))
	viewer.search.SetText("Jumping")
	characters := map[string]bool{}
	for row, event := range viewer.rows {
		characters[event.Character] = true
		label := viewer.table.CreateCell().(*widget.Label)
		viewer.table.UpdateCell(widget.TableCellID{Row: row + 1, Col: 1}, label)
		if label.Text != event.Character {
			t.Fatal("character column did not retain the owning log during sorting/search")
		}
	}
	if !reflect.DeepEqual(characters, map[string]bool{"Pilot A": true, "Pilot B": true}) {
		t.Fatalf("identical messages lost character association: %v", characters)
	}
	viewer.search.SetText("Pilot B")
	if len(viewer.rows) != 0 {
		t.Fatal("message search included the Character column")
	}
}

func TestLocalEvents(t *testing.T) {
	directory := os.Getenv("EVELA_TEST_LOGS")
	if directory == "" {
		t.Skip("set EVELA_TEST_LOGS to verify real Events data read-only")
	}
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, logLoadingCutoff(time.Now(), 30), nil)
	if err != nil {
		t.Fatal(err)
	}
	start, end := presetRange("Last 24 hours", snapshot)
	var total, expected int
	for _, log := range snapshot.Logs {
		total += len(log.Events)
		for _, event := range log.Events {
			if event.Second >= start.Unix() && event.Second < end.Unix() {
				expected++
			}
		}
	}
	application := test.NewTempApp(t)
	window := application.NewWindow("Real Events")
	defer window.Close()
	viewer := newEventsViewer(window)
	viewer.setRange(snapshot, start, end)
	if expected == 0 || len(viewer.rows) != expected {
		t.Fatalf("real Events range: got %d rows, want %d", len(viewer.rows), expected)
	}
	viewer.regex.SetChecked(true)
	viewer.search.SetText(`^\(combat\)`)
	for _, event := range viewer.rows {
		if !strings.HasPrefix(event.Message, "(combat)") {
			t.Fatal("regex leaked a non-combat message")
		}
	}
	t.Logf("%d files; %d retained events; %d events in latest 24 hours; %d combat messages match regex", snapshot.Files, total, expected, len(viewer.rows))
}

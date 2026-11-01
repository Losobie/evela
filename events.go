package main

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const eventsRegexPreference = "events.search.regex"

type logEvent struct {
	Second  int64
	Message string
}

type eventRow struct {
	logEvent
	Character string
}

type eventsViewer struct {
	window      fyne.Window
	content     fyne.CanvasObject
	search      *widget.Entry
	regex       *widget.Check
	status      *widget.Label
	table       *widget.Table
	snapshot    *logSnapshot
	start, end  time.Time
	rangeEvents []eventRow
	rows        []eventRow
	descending  bool
	characters  map[string]bool // nil includes every character; an empty map includes none.
}

func newEventsViewer(window fyne.Window) *eventsViewer {
	viewer := &eventsViewer{window: window, search: widget.NewEntry(), status: widget.NewLabel("")}
	viewer.search.SetPlaceHolder("Search messages…")
	viewer.status.Truncation = fyne.TextTruncateEllipsis
	viewer.regex = widget.NewCheck("Regex", nil)
	viewer.regex.SetChecked(fyne.CurrentApp().Preferences().Bool(eventsRegexPreference))
	viewer.search.Validator = func(query string) error {
		if viewer.regex.Checked {
			_, err := regexp.Compile(query)
			return err
		}
		return nil
	}
	viewer.search.OnChanged = func(string) {
		viewer.applySearch()
		viewer.table.ScrollToTop()
	}
	viewer.regex.OnChanged = func(checked bool) {
		fyne.CurrentApp().Preferences().SetBool(eventsRegexPreference, checked)
		viewer.search.Validate()
		viewer.applySearch()
		viewer.table.ScrollToTop()
	}
	viewer.table = widget.NewTable(
		func() (int, int) { return len(viewer.rows) + 1, 3 },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			label.TextStyle.Bold = id.Row == 0
			if id.Row == 0 {
				text := []string{"Timestamp", "Character", "Message"}[id.Col]
				if id.Col == 0 {
					text = "Timestamp ↑"
					if viewer.descending {
						text = "Timestamp ↓"
					}
				}
				label.SetText(text)
				return
			}
			event := viewer.rows[id.Row-1]
			switch id.Col {
			case 0:
				label.SetText(time.Unix(event.Second, 0).UTC().Format("2006-01-02 15:04:05"))
			case 1:
				label.SetText(event.Character)
			case 2:
				label.SetText(event.Message)
			}
		})
	viewer.table.StickyRowCount = 1
	viewer.table.SetColumnWidth(0, 200)
	viewer.table.SetColumnWidth(1, 190)
	viewer.table.SetColumnWidth(2, 1000)
	viewer.table.OnSelected = func(id widget.TableCellID) {
		viewer.table.UnselectAll()
		if id.Row == 0 {
			if id.Col == 0 {
				viewer.descending = !viewer.descending
				viewer.applySearch()
				viewer.table.ScrollToTop()
			}
			return
		}
		event := viewer.rows[id.Row-1]
		text := widget.NewLabel(time.Unix(event.Second, 0).UTC().Format("2006-01-02 15:04:05") + " EVE time\nCharacter: " + event.Character + "\n\n" + event.Message)
		text.Wrapping, text.Selectable = fyne.TextWrapWord, true
		details := dialog.NewCustom("Log event", "Close", container.NewVScroll(text), viewer.window)
		details.Resize(fyne.NewSize(900, 320))
		details.Show()
	}
	search := container.NewBorder(nil, nil, widget.NewLabel("Message search"), viewer.regex, viewer.search)
	note := widget.NewLabel("Characters and Character Group filters · EVE time · Click Timestamp to reverse order; select a row to read its full message.")
	note.Truncation = fyne.TextTruncateEllipsis
	viewer.content = container.NewBorder(container.NewVBox(search, viewer.status), note, nil, nil, viewer.table)
	viewer.applySearch()
	return viewer
}

func (viewer *eventsViewer) setRange(snapshot *logSnapshot, start, end time.Time, characters map[string]bool) {
	sameRange := viewer.snapshot == snapshot && viewer.start.Equal(start) && viewer.end.Equal(end)
	sameCharacters := (viewer.characters == nil) == (characters == nil) && maps.Equal(viewer.characters, characters)
	if sameRange && sameCharacters {
		return
	}
	changedRange := !viewer.start.Equal(start) || !viewer.end.Equal(end)
	viewer.characters = maps.Clone(characters)
	viewer.snapshot, viewer.start, viewer.end = snapshot, start, end
	if !sameRange {
		clear(viewer.rangeEvents)
		viewer.rangeEvents = viewer.rangeEvents[:0]
		first, last := start.Unix(), end.Unix()
		for _, log := range snapshot.Logs {
			// Parsed events are sorted once, so broad archives need only a binary
			// search and a copy of records belonging to the visible time window.
			left := sort.Search(len(log.Events), func(i int) bool { return log.Events[i].Second >= first })
			right := sort.Search(len(log.Events), func(i int) bool { return log.Events[i].Second >= last })
			if left < right {
				for _, event := range log.Events[left:right] {
					viewer.rangeEvents = append(viewer.rangeEvents, eventRow{logEvent: event, Character: log.Character})
				}
			}
		}
		sort.SliceStable(viewer.rangeEvents, func(i, j int) bool { return viewer.rangeEvents[i].Second < viewer.rangeEvents[j].Second })
	}
	viewer.applySearch()
	if changedRange || !sameCharacters {
		viewer.table.ScrollToTop()
	}
}

func (viewer *eventsViewer) applySearch() {
	query := viewer.search.Text
	var pattern *regexp.Regexp
	if viewer.regex.Checked {
		var err error
		pattern, err = regexp.Compile(query)
		if err != nil {
			clear(viewer.rows)
			viewer.rows = viewer.rows[:0]
			viewer.status.SetText("Invalid regular expression: " + err.Error())
			viewer.table.Refresh()
			return
		}
	}
	clear(viewer.rows)
	viewer.rows = viewer.rows[:0]
	literal := strings.ToLower(query)
	eligible := 0
	for _, event := range viewer.rangeEvents {
		if viewer.characters != nil && !viewer.characters[event.Character] {
			continue
		}
		eligible++
		matches := query == ""
		if !matches {
			if pattern != nil {
				matches = pattern.MatchString(event.Message)
			} else {
				matches = strings.Contains(strings.ToLower(event.Message), literal)
			}
		}
		if matches {
			viewer.rows = append(viewer.rows, event)
		}
	}
	if viewer.descending {
		// Reverse timestamp groups without reversing equal-time event ordering.
		for left, right := 0, len(viewer.rows)-1; left < right; left, right = left+1, right-1 {
			viewer.rows[left], viewer.rows[right] = viewer.rows[right], viewer.rows[left]
		}
		for first := 0; first < len(viewer.rows); {
			end := first + 1
			for end < len(viewer.rows) && viewer.rows[end].Second == viewer.rows[first].Second {
				end++
			}
			for left, right := first, end-1; left < right; left, right = left+1, right-1 {
				viewer.rows[left], viewer.rows[right] = viewer.rows[right], viewer.rows[left]
			}
			first = end
		}
	}
	status := fmt.Sprintf("%d of %d events", len(viewer.rows), eligible)
	if len(viewer.rangeEvents) == 0 {
		status = "No events in this time range"
	} else if eligible == 0 {
		status = "No events for the selected characters and character groups"
	} else if len(viewer.rows) == 0 {
		status = "No matching messages · " + status
	}
	if viewer.regex.Checked {
		status += " · Regex is case sensitive; use (?i) to ignore case"
	}
	viewer.status.SetText(status)
	viewer.table.Refresh()
}

func (ui *analyzerUI) refreshEvents() {
	if ui.events == nil {
		return
	}
	if ui.tabs != nil && ui.tabs.Selected().Content != ui.events.content {
		ui.eventsDirty = true
		// A hidden viewer must not keep an obsolete full archive alive after
		// folder changes or a new import. Its displayed rows refresh on entry.
		if ui.events.snapshot != ui.snapshot {
			ui.events.snapshot = nil
		}
		return
	}
	ui.eventsDirty = false
	members := ui.graphMetadata().members
	selected := map[string]bool{}
	for _, log := range ui.snapshot.Logs {
		selected[log.Character] = ui.damageFilterSelected(filterCharacters, log.Character) &&
			characterGroupsSelected(ui.globalFilter(filterCharacterGroup), members[characterKey(log.Character)])
	}
	ui.events.setRange(ui.snapshot, ui.view.Start, ui.view.End, selected)
}

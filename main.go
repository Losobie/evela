package main

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type analyzerUI struct {
	window                                  fyne.Window
	snapshot                                *logSnapshot
	view                                    damageView
	analysis                                *logSnapshot
	selected                                map[string]bool
	path                                    *widget.Entry
	start, end                              *dateTimePicker
	rangeSelect, bucketSelect               *widget.Select
	status, summary, directoryInfo          *widget.Label
	progress                                *widget.ProgressBar
	refresh                                 *widget.Button
	auto                                    *widget.Check
	characters                              *fyne.Container
	characterSearch                         *widget.Entry
	incoming, outgoing                      *damageChart
	ewar                                    *damageChart
	ewarMetric                              *widget.Select
	outgoingLogistics                       *damageChart
	incomingLogistics                       *damageChart
	outgoingLogisticsMetric                 *widget.Select
	incomingLogisticsMetric                 *widget.Select
	graphsScroll                            *container.Scroll
	graphStack                              *fyne.Container
	graphCards                              map[string]*widget.Card
	graphsEmpty                             fyne.CanvasObject
	totals                                  *widget.Table
	events                                  *eventsViewer
	loadRequests                            chan []string
	stop                                    chan struct{}
	settingRange                            bool
	ships                                   map[string]shipRecord
	shipHistory                             shipHistory
	locationHistory                         locationHistory
	logLocations                            logLocationHistory
	logLocationUpdates                      chan logLocationHistory
	locationLookup                          locationResolver
	groupSelect                             *widget.Select
	characterGroups                         []characterGroup
	characterGroupEditor                    *characterGroupEditor
	tableSeries                             []characterSeries
	totalsSortColumn                        int
	totalsSortDescending                    bool
	clientID                                *widget.Entry
	esiStatus                               *widget.Label
	esiConnections                          *fyne.Container
	shipAuto                                *widget.Check
	shipRefresh, connect, cancelLogin       *widget.Button
	esiActions                              chan esiAction
	cancelESILogin                          context.CancelFunc
	tabs                                    *container.AppTabs
	callbackPort, accessToken, refreshToken *widget.Entry
	callbackURL                             *widget.Label
	esiDialogStatus                         *widget.Label
	tokenConnections                        *fyne.Container
	esiDialog                               *dialog.CustomDialog
	esiSettingsTabs                         *container.AppTabs
	esiSettingsDirty, settingESI            bool
	graphUndo, graphRedo                    []graphRangeChange
	logDirectory                            string
	logLookbackDays                         int
	logLoadingDialog                        *dialog.CustomDialog
	graphAverage10s                         bool
	graphInvulnerabilityMarkers             bool
	graphFormatLabel                        *widget.Label
	graphFormatButton                       *widget.Button
	graphOptionsButton                      *widget.Button
	incomingGroupButton                     *widget.Button
	incomingGrouping                        string
	graphMenuPopup                          *widget.PopUpMenu
	damageFilters                           map[string]*damageFilterSelection
	filterOptions                           map[string][]damageFilterOption
	filterButtons                           map[string]*widget.Button
	filterPopup                             *damageFilterPopup
}

func main() {
	application := app.NewWithID("io.evela.damage-analyzer")
	application.SetIcon(fyne.NewStaticResource("icon.svg", applicationIcon))
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Evela · EVE Online damage analyzer")
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.SetOnClosed(func() { close(ui.stop) })
	go ui.watch()
	go ui.watchESI()
	ui.requestLoad()
	application.Lifecycle().SetOnStarted(ui.initGraphMouseNavigation)
	window.ShowAndRun()
}

func newAnalyzerUI(window fyne.Window) *analyzerUI {
	ui := &analyzerUI{window: window, snapshot: &logSnapshot{}, selected: make(map[string]bool), loadRequests: make(chan []string, 1), stop: make(chan struct{})}
	ui.path = widget.NewEntry()
	ui.path.SetText(strings.Join(savedLogFolders(), "\n"))
	ui.logLookbackDays = savedLogLookbackDays()
	ui.loadCharacterGroups()
	ui.start, ui.end = newDateTimePicker(), newDateTimePicker()
	ui.end.Validator = func(value string) error {
		if _, err := parseRangeTime(ui.start.Text); err != nil {
			_, err = parseRangeTime(value)
			return err
		}
		_, _, err := parseRange(ui.start.Text, value)
		return err
	}
	ui.status = widget.NewLabel("Ready to read game logs")
	ui.status.Wrapping = fyne.TextWrapWord
	ui.status.Truncation = fyne.TextTruncateEllipsis
	ui.directoryInfo = widget.NewLabel("")
	ui.directoryInfo.Wrapping = fyne.TextWrapWord
	ui.directoryInfo.Truncation = fyne.TextTruncateEllipsis
	ui.summary = widget.NewLabel("Waiting for logs…")
	ui.summary.TextStyle.Bold = true
	ui.summary.Truncation = fyne.TextTruncateEllipsis
	ui.progress = widget.NewProgressBar()
	ui.progress.Hide()
	ui.refresh = widget.NewButtonWithIcon("Refresh logs", theme.ViewRefreshIcon(), ui.requestLoad)
	ui.auto = widget.NewCheck("Auto refresh · 10s", nil)
	ui.auto.SetChecked(true)
	ui.rangeSelect = widget.NewSelect([]string{"Last 10 minutes", "Last 30 minutes", "Last hour", "Last 24 hours", "Last 7 days", "All history", "Custom"}, func(_ string) { ui.clearGraphRangeHistory(); ui.updateRange() })
	markCustom := func(string) {
		if !ui.settingRange {
			ui.clearGraphRangeHistory()
			ui.rangeSelect.Selected = "Custom"
			ui.rangeSelect.Refresh()
			ui.end.Validate() // A changed start can invalidate or repair the end.
			ui.rebuildView()
		}
	}
	ui.start.OnChanged, ui.end.OnChanged = markCustom, markCustom
	ui.bucketSelect = widget.NewSelect([]string{"Auto", "1 second", "5 seconds", "10 seconds", "1 minute", "5 minutes"}, func(_ string) { ui.rebuildView() })
	ui.characters = container.NewVBox()
	ui.characterSearch = widget.NewEntry()
	ui.characterSearch.SetPlaceHolder("Find a character…")
	ui.characterSearch.OnChanged = func(string) { ui.rebuildCharacters() }
	ui.incoming, ui.outgoing = newDamageChart(), newDamageChart()
	ui.incoming.OnRangeSelected, ui.outgoing.OnRangeSelected = ui.applyGraphRange, ui.applyGraphRange
	ui.incoming.OnWheelZoom, ui.outgoing.OnWheelZoom = ui.applyGraphWheelRange, ui.applyGraphWheelRange
	ui.initESI()
	ui.initGraphFormat()
	ui.initDamageFilters()
	ui.initEWar()
	ui.initLogistics()
	ui.totals = widget.NewTable(
		func() (int, int) { return len(ui.tableSeries) + 1, 7 },
		func() fyne.CanvasObject { return newTotalsCell() },
		func(id widget.TableCellID, object fyne.CanvasObject) {
			cell := object.(*totalsCell)
			label := cell.label
			cell.icon.Hide()
			label.TextStyle = fyne.TextStyle{Bold: id.Row == 0}
			label.Truncation = fyne.TextTruncateEllipsis
			label.Alignment = fyne.TextAlignTrailing
			if id.Col <= 1 {
				label.Alignment = fyne.TextAlignLeading
			}
			if id.Row == 0 {
				label.Alignment = fyne.TextAlignLeading
				first, second := "Character", "Current ship"
				if ui.shipGrouping() {
					first, second = "Ship type", "Members"
				}
				if ui.shipClassGrouping() {
					first = "Ship class"
				}
				if ui.characterGroupGrouping() {
					first, second = "Character group", "Members"
				}
				heading := []string{first, second, "Outgoing damage", "Incoming damage", "Avg out DPS", "Avg in DPS", "Hits out / in"}[id.Col]
				if id.Col == ui.totalsSortColumn {
					cell.icon.SetResource(theme.MoveUpIcon())
					if ui.totalsSortDescending {
						cell.icon.SetResource(theme.MoveDownIcon())
					}
					cell.icon.Show()
				}
				label.SetText(heading)
				cell.Refresh()
				return
			}
			if id.Row > len(ui.tableSeries) {
				return
			}
			series := ui.tableSeries[id.Row-1]
			duration := ui.view.End.Sub(ui.view.Start).Seconds()
			if duration <= 0 {
				duration = 1
			}
			ship := shipLabel(series.Name, ui.ships)
			if ui.anyGrouping() {
				ship = fmt.Sprintf("%d characters", len(series.Members))
			}
			label.SetText([]string{series.Name, ship, fmt.Sprintf("%.0f", series.TotalOutgoing), fmt.Sprintf("%.0f", series.TotalIncoming), fmt.Sprintf("%.2f", series.TotalOutgoing/duration), fmt.Sprintf("%.2f", series.TotalIncoming/duration), fmt.Sprintf("%d / %d", series.OutgoingHits, series.IncomingHits)}[id.Col])
			cell.Refresh()
		})
	ui.totals.StickyRowCount = 1
	ui.totals.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			ui.sortTotalsByColumn(id.Col)
			// Allow repeated clicks on the same heading to reverse the order.
			ui.totals.UnselectAll()
		}
	}
	for col, width := range []float32{190, 145, 175, 175, 130, 130, 140} {
		ui.totals.SetColumnWidth(col, width)
	}
	ui.events = newEventsViewer(window)
	ui.bucketSelect.SetSelected("Auto")
	ui.loadGraphRangePreference()
	ui.initGraphRangeShortcuts()
	window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Settings",
		fyne.NewMenuItem("Log loading…", ui.openLogLoadingSettings),
		fyne.NewMenuItem("Character groups…", ui.openCharacterGroupSettings),
		fyne.NewMenuItem("ESI configuration…", ui.openESISettings))))
	return ui
}

func (ui *analyzerUI) content() fyne.CanvasObject {
	title := canvas.NewText("EVELA", color.NRGBA{68, 209, 223, 255})
	title.TextStyle.Bold = true
	title.TextSize = 28
	// Keep the progress slot even when its bar is hidden, so refreshing never
	// changes the height available to the graphs.
	progressSlot := container.NewGridWrap(fyne.NewSize(140, ui.progress.MinSize().Height), ui.progress)
	details := widget.NewButton("Details…", func() { ui.showRefreshDetails("Log refresh", ui.status.Text+"\n\n"+ui.directoryInfo.Text) })
	heading := container.NewHBox(title, widget.NewLabel("COMBAT LOG ANALYZER"), layout.NewSpacer(), progressSlot, ui.auto, ui.refresh, details)
	filters := container.NewGridWithColumns(4,
		container.NewBorder(widget.NewLabel("Time range"), nil, nil, nil, ui.rangeSelect),
		container.NewBorder(widget.NewLabel("Start · EVE time"), nil, nil, nil, ui.start),
		container.NewBorder(widget.NewLabel("End · EVE time · exclusive"), nil, nil, nil, ui.end),
		container.NewBorder(widget.NewLabel("Graph interval"), nil, nil, nil, ui.bucketSelect))
	controls := container.NewVBox(heading, filters, widget.NewSeparator())
	filterBar := ui.damageFilterBar()
	ui.graphCards = map[string]*widget.Card{}
	ui.graphStack = container.NewVBox(
		ui.newGraphCard(graphOutgoing, "Outgoing", ui.graphPanContent(ui.outgoing)),
		ui.newGraphCard(graphOutgoingLogistics, "Outgoing logistics", container.NewBorder(logisticsControls(ui.outgoingLogisticsMetric), nil, nil, nil, ui.graphPanContent(ui.outgoingLogistics))),
		ui.newGraphCard(graphIncoming, "Incoming", container.NewBorder(ui.incomingDamageFilterBar(), nil, nil, nil, ui.graphPanContent(ui.incoming))),
		ui.newGraphCard(graphIncomingLogistics, "Incoming logistics", container.NewBorder(logisticsControls(ui.incomingLogisticsMetric), nil, nil, nil, ui.graphPanContent(ui.incomingLogistics))),
		ui.newGraphCard(graphIncomingEWar, "Incoming E-War", container.NewBorder(ui.ewarControls(), nil, nil, nil, ui.graphPanContent(ui.ewar))))
	ui.graphsScroll = container.NewVScroll(ui.graphStack)
	ui.graphsEmpty = newGraphsEmptyState()
	ui.applyGraphVisibility()
	scrollHint := widget.NewLabel("Use the scrollbar or wheel over graph headings to scroll · Wheel over a plot zooms all graphs")
	scrollHint.Truncation = fyne.TextTruncateEllipsis
	graphContent := container.NewBorder(container.NewVBox(filterBar, scrollHint), nil, nil, nil, ui.graphsScroll)
	totalsNote := widget.NewLabel("Current filters and time range · average DPS includes idle time\nClick a heading to sort; click again to reverse. Hits sort by outgoing, then incoming.")
	totalsNote.Wrapping = fyne.TextWrapWord
	totals := container.NewBorder(totalsNote, nil, nil, nil, ui.totals)
	tabs := container.NewAppTabs(container.NewTabItem("Graphs", graphContent), container.NewTabItem("Totals", totals), container.NewTabItem("Events", ui.events.content), container.NewTabItem("ESI & ships", ui.esiContent()))
	ui.tabs = tabs
	footer := container.NewVBox(widget.NewSeparator(), ui.summary)
	return container.NewPadded(container.NewBorder(controls, footer, nil, nil, tabs))
}

func (ui *analyzerUI) requestLoad() {
	folders, err := parseLogFolders(ui.path.Text)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	select {
	case <-ui.loadRequests:
	default:
	}
	ui.loadRequests <- folders
}

// The worker owns the file cache. All widgets and view state stay on Fyne's UI thread.
func (ui *analyzerUI) watch() {
	index := &multiLogIndex{}
	defer index.close()
	var directories []string
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ui.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	for {
		select {
		case <-ui.stop:
			return
		case directories = <-ui.loadRequests:
		case <-ticker.C:
			enabled := false
			fyne.DoAndWait(func() { enabled = ui.auto.Checked })
			if !enabled || len(directories) == 0 {
				continue
			}
		}
		var lookbackDays int
		fyne.DoAndWait(func() {
			lookbackDays = ui.logLookbackDays
			ui.refresh.Disable()
			ui.progress.SetValue(0)
			ui.progress.Show()
			ui.status.SetText("Reading logs… unchanged files use the memory cache.")
		})
		snapshot, err := index.refresh(ctx, directories, logLoadingCutoff(time.Now(), lookbackDays), func(done, total int) {
			fyne.Do(func() {
				ui.progress.SetValue(float64(done) / float64(total))
				ui.status.SetText(fmt.Sprintf("Refreshed %d / %d log folders…", done, total))
			})
		})
		if ctx.Err() != nil {
			return
		}
		fyne.DoAndWait(func() {
			ui.refresh.Enable()
			ui.progress.Hide()
			if err != nil {
				ui.status.SetText("Could not load folders: " + err.Error())
				return
			}
			currentFolders, _ := parseLogFolders(ui.path.Text)
			if lookbackDays != ui.logLookbackDays || !slices.Equal(directories, currentFolders) {
				// Saving settings queues a new load. Do not display a completed
				// scan from the previous limit while that request is pending.
				return
			}
			ui.snapshot = snapshot
			ui.rememberLogLocations(snapshot.locationSessions())
			directoryKey := strings.Join(directories, "\n")
			if ui.logDirectory != directoryKey {
				ui.clearGraphRangeHistory()
				ui.logDirectory = directoryKey
			}
			for _, name := range snapshot.Characters {
				if _, known := ui.selected[name]; !known {
					ui.selected[name] = ui.damageFilterSelected(filterCharacters, name)
				}
			}
			ui.updateRange()
			ui.directoryInfo.SetText(fmt.Sprintf("%d log folders · %d log files loaded · %d too old · %d without filename dates · %d characters · %d duplicate exports skipped · %d files without a Listener header", len(directories), snapshot.Files, snapshot.SkippedOld, snapshot.SkippedUndated, len(snapshot.Characters), snapshot.Duplicates, snapshot.Unassigned))
			status := "Updated " + time.Now().Format("15:04:05") + " · Logs stay on this computer."
			if snapshot.Files == 0 {
				if snapshot.SkippedOld+snapshot.SkippedUndated > 0 {
					status += " No logs matched the loading limit. Change Settings → Log loading to include older files."
				} else {
					status += " No .txt game logs found in the configured folders."
				}
			} else if snapshot.Last.IsZero() {
				status += " No supported English combat messages found."
			} else {
				status += " Latest combat: " + snapshot.Last.Format("2006-01-02 15:04:05") + " EVE time."
			}
			status += snapshot.warningDetails()
			ui.status.SetText(status)
		})
	}
}

func (ui *analyzerUI) updateRange() {
	custom := ui.rangeSelect.Selected == "Custom"
	if custom {
		ui.start.Enable()
		ui.end.Enable()
	} else {
		start, end := presetRange(ui.rangeSelect.Selected, ui.snapshot)
		ui.settingRange = true
		ui.start.SetText(start.Format("2006-01-02 15:04:05"))
		ui.end.SetText(end.Format("2006-01-02 15:04:05"))
		ui.settingRange = false
	}
	ui.end.Validate()
	ui.rebuildView()
}

func (ui *analyzerUI) rebuildView() {
	if ui.bucketSelect == nil || ui.totals == nil {
		return
	}
	start, end, err := parseRange(strings.TrimSpace(ui.start.Text), strings.TrimSpace(ui.end.Text))
	if err != nil {
		return
	}
	bucket := map[string]time.Duration{"1 second": time.Second, "5 seconds": 5 * time.Second, "10 seconds": 10 * time.Second, "1 minute": time.Minute, "5 minutes": 5 * time.Minute}[ui.bucketSelect.Selected]
	ui.locationLookup = ui.resolvedLocations()
	ui.migrateSystemFilterNames(ui.locationLookup)
	ui.analysis = ui.filteredDamageSnapshot()
	ui.view = buildView(ui.analysisSnapshot(), start, end, bucket)
	ui.refreshDamageFilterOptions()
	ui.saveGraphRangePreference()
	ui.renderSelection()
	ui.rebuildCharacters()
	ui.events.setRange(ui.snapshot, start, end)
}

func (ui *analyzerUI) rebuildCharacters() {
	var rows []fyne.CanvasObject
	active := ui.view.activeSeries()
	query := strings.ToLower(strings.TrimSpace(ui.characterSearch.Text))
	if ui.anyGrouping() {
		rows = ui.groupedCharacterList(query)
	} else {
		for _, series := range active {
			name := series.Name
			if !strings.Contains(strings.ToLower(name+" "+shipLabel(name, ui.ships)), query) {
				continue
			}
			rows = append(rows, ui.characterRow(name, "", shipLabel(name, ui.ships)))
		}
	}
	if len(rows) == 0 {
		message := "No matching characters"
		if len(ui.snapshot.Characters) == 0 {
			message = "Characters appear after import"
		} else if len(active) == 0 {
			message = "No damage data in this time range"
		}
		empty := widget.NewLabel(message)
		empty.Wrapping = fyne.TextWrapWord
		rows = append(rows, empty)
	}
	// Swap the complete list in one step. Emptying a scrolled list first would
	// temporarily shrink its content and reset the user's scroll position.
	ui.characters.Objects = rows
	ui.characters.Refresh()
	if ui.characterGroupEditor != nil {
		ui.characterGroupEditor.refreshLists()
	}
}

func (ui *analyzerUI) selectRangeCharacters(selected bool) {
	for _, series := range ui.view.activeSeries() {
		if ui.characterMatchesGroupFilter(series.Name) {
			ui.selected[series.Name] = selected
		}
	}
	ui.rebuildCharacters()
	ui.renderSelection()
}

func (ui *analyzerUI) renderSelection() {
	in := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket}
	if ui.graphAverage10s {
		in.AverageWindow = dpsAverageWindow
	}
	out := in
	selected := ui.filteredCharacterSelection()
	if ui.graphInvulnerabilityMarkers {
		out.Markers = ui.view.structureMarkers(ui.analysisSnapshot(), selected)
	}
	var incoming, outgoing float64
	var hitsIn, hitsOut int
	contributing := map[string]bool{}
	active := ui.view.activeSeries()
	for _, series := range active {
		if !selected[series.Name] {
			continue
		}
		contributing[series.Name] = true
		incoming += series.TotalIncoming
		outgoing += series.TotalOutgoing
		hitsIn += series.IncomingHits
		hitsOut += series.OutgoingHits
	}
	var filtered []characterSeries
	for _, series := range active {
		if selected[series.Name] {
			filtered = append(filtered, series)
		}
	}
	ui.tableSeries = append([]characterSeries(nil), filtered...)
	graphSeries := active
	if ui.graphAverage10s && !ui.shipGrouping() {
		graphSeries = ui.view.tenSecondAverage(ui.analysisSnapshot())
	}
	if ui.shipGrouping() {
		ui.tableSeries = ui.groupedShipSeries(selected, false)
		graphSeries = ui.groupedShipSeries(ui.selected, ui.graphAverage10s)
	} else if ui.characterGroupGrouping() {
		ui.tableSeries = ui.groupCustomCharacterSeries(active, selected)
		graphSeries = ui.groupCustomCharacterSeries(graphSeries, selected)
	}
	for _, series := range graphSeries {
		if !ui.anyGrouping() && !selected[series.Name] {
			continue
		}
		ink := characterColor(series.Name)
		out.Series = append(out.Series, plotSeries{Name: series.Name, Color: ink, Values: series.Outgoing, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
	}
	for _, series := range ui.incomingGraphSeries(selected) {
		in.Series = append(in.Series, plotSeries{Name: series.Name, Color: characterColor(series.Name), Values: series.Incoming, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
	}
	if !ui.graphAverage10s {
		ui.attachIndividualHits(&in, &out, selected)
	}
	ui.incoming.setData(in)
	ui.outgoing.setData(out)
	ui.ewar.setData(ui.incomingEWarPlot(selected))
	ui.outgoingLogistics.setData(ui.logisticsPlot(selected, false))
	ui.incomingLogistics.setData(ui.logisticsPlot(selected, true))
	ui.sortDamageTotals()
	ui.totals.Refresh()
	var ewarCount int
	var drained float64
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.EWar {
			if event.Second >= ui.view.Start.Unix() && event.Second < ui.view.End.Unix() {
				contributing[log.Character] = true
				ewarCount++
				drained += event.Amount
			}
		}
		for _, event := range log.Logistics {
			if event.Second >= ui.view.Start.Unix() && event.Second < ui.view.End.Unix() {
				contributing[log.Character] = true
			}
		}
	}
	summary := fmt.Sprintf("%d selected   |   Outgoing %s damage / %d hits   |   Incoming %s damage / %d hits", len(contributing), shortNumber(outgoing), hitsOut, shortNumber(incoming), hitsIn)
	if ewarCount > 0 {
		summary += fmt.Sprintf("   |   Incoming E-War %d events / %s GJ", ewarCount, shortNumber(drained))
	}
	ui.summary.SetText(summary)
	ui.updateGraphFormatLabel()
	ui.refreshDamageFilterControls()
}

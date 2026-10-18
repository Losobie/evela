package main

import (
	"context"
	"fmt"
	"image/color"
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
	window                                   fyne.Window
	snapshot                                 *logSnapshot
	view                                     damageView
	selected                                 map[string]bool
	path, start, end                         *widget.Entry
	rangeSelect, bucketSelect                *widget.Select
	status, interval, summary, directoryInfo *widget.Label
	progress                                 *widget.ProgressBar
	refresh                                  *widget.Button
	auto                                     *widget.Check
	characters                               *fyne.Container
	characterSearch                          *widget.Entry
	incoming, outgoing                       *damageChart
	totals                                   *widget.Table
	loadRequests                             chan string
	stop                                     chan struct{}
	settingRange                             bool
	ships                                    map[string]shipRecord
	shipHistory                              shipHistory
	groupSelect                              *widget.Select
	tableSeries                              []characterSeries
	clientID                                 *widget.Entry
	esiStatus                                *widget.Label
	esiConnections                           *fyne.Container
	shipAuto                                 *widget.Check
	shipRefresh, connect, cancelLogin        *widget.Button
	esiActions                               chan esiAction
	cancelESILogin                           context.CancelFunc
	tabs                                     *container.AppTabs
	callbackPort, accessToken, refreshToken  *widget.Entry
	callbackURL                              *widget.Label
	esiDialogStatus                          *widget.Label
	tokenConnections                         *fyne.Container
	esiDialog                                *dialog.CustomDialog
	esiSettingsTabs                          *container.AppTabs
	esiSettingsDirty, settingESI             bool
	esiAuthMode                              *widget.Select
	clientSecret                             *widget.Entry
	esiApplicationForm                       *widget.Form
	graphUndo, graphRedo                     []graphRangeChange
	logDirectory                             string
	logLookbackDays                          int
	logLoadingDialog                         *dialog.CustomDialog
	graphLogarithmic                         bool
	graphAverage10s                          bool
	graphFormatLabel                         *widget.Label
	graphFormatButton                        *widget.Button
}

func main() {
	application := app.NewWithID("io.evela.damage-analyzer")
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
	ui := &analyzerUI{window: window, snapshot: &logSnapshot{}, selected: make(map[string]bool), loadRequests: make(chan string, 1), stop: make(chan struct{})}
	ui.path = widget.NewEntry()
	ui.path.SetText(defaultLogDirectory())
	ui.logLookbackDays = savedLogLookbackDays()
	ui.start, ui.end = widget.NewEntry(), widget.NewEntry()
	ui.start.SetPlaceHolder("2026-10-17 00:00:00")
	ui.end.SetPlaceHolder("2026-10-18 00:00:00")
	ui.status = widget.NewLabel("Ready to read game logs")
	ui.status.Wrapping = fyne.TextWrapWord
	ui.status.Truncation = fyne.TextTruncateEllipsis
	ui.directoryInfo = widget.NewLabel("")
	ui.directoryInfo.Wrapping = fyne.TextWrapWord
	ui.directoryInfo.Truncation = fyne.TextTruncateEllipsis
	ui.summary = widget.NewLabel("Waiting for logs…")
	ui.summary.TextStyle.Bold = true
	ui.summary.Truncation = fyne.TextTruncateEllipsis
	ui.interval = widget.NewLabel("Damage per second · timestamps in UTC")
	ui.progress = widget.NewProgressBar()
	ui.progress.Hide()
	ui.refresh = widget.NewButtonWithIcon("Refresh logs", theme.ViewRefreshIcon(), ui.requestLoad)
	ui.auto = widget.NewCheck("Auto refresh · 10s", nil)
	ui.auto.SetChecked(true)
	ui.rangeSelect = widget.NewSelect([]string{"Latest combat day", "Last 30 seconds", "Last 1 minute", "Last 5 minutes", "Last 10 minutes", "Last 30 minutes", "Last hour", "Last 24 hours", "Last 7 days", "All history", "Custom UTC range"}, func(_ string) { ui.clearGraphRangeHistory(); ui.updateRange() })
	markCustom := func(string) {
		if !ui.settingRange {
			ui.clearGraphRangeHistory()
			ui.rangeSelect.Selected = "Custom UTC range"
			ui.rangeSelect.Refresh()
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
	ui.totals = widget.NewTable(
		func() (int, int) { return len(ui.tableSeries) + 1, 7 },
		func() fyne.CanvasObject { return widget.NewLabel("Character name") },
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			label.TextStyle = fyne.TextStyle{Bold: id.Row == 0}
			label.Alignment = fyne.TextAlignTrailing
			if id.Col <= 1 {
				label.Alignment = fyne.TextAlignLeading
			}
			if id.Row == 0 {
				first, second := "Character", "Current ship"
				if ui.shipGrouping() {
					first, second = "Ship type", "Members"
				}
				if ui.shipClassGrouping() {
					first = "Ship class"
				}
				label.SetText([]string{first, second, "Outgoing damage", "Incoming damage", "Avg out DPS", "Avg in DPS", "Hits out / in"}[id.Col])
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
			if ui.shipGrouping() {
				ship = fmt.Sprintf("%d characters", len(series.Members))
			}
			label.SetText([]string{series.Name, ship, fmt.Sprintf("%.0f", series.TotalOutgoing), fmt.Sprintf("%.0f", series.TotalIncoming), fmt.Sprintf("%.2f", series.TotalOutgoing/duration), fmt.Sprintf("%.2f", series.TotalIncoming/duration), fmt.Sprintf("%d / %d", series.OutgoingHits, series.IncomingHits)}[id.Col])
		})
	for col, width := range []float32{210, 155, 150, 150, 120, 120, 130} {
		ui.totals.SetColumnWidth(col, width)
	}
	ui.bucketSelect.SetSelected("Auto")
	ui.rangeSelect.SetSelected("Latest combat day")
	ui.initGraphRangeShortcuts()
	window.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Settings",
		fyne.NewMenuItem("Log loading…", ui.openLogLoadingSettings),
		fyne.NewMenuItem("ESI configuration…", ui.openESISettings))))
	return ui
}

func (ui *analyzerUI) content() fyne.CanvasObject {
	title := canvas.NewText("EVELA", color.NRGBA{68, 209, 223, 255})
	title.TextStyle.Bold = true
	title.TextSize = 28
	heading := container.NewHBox(title, widget.NewLabel("COMBAT LOG ANALYZER"), layout.NewSpacer(), ui.auto, ui.refresh)
	// Keep the progress slot even when its bar is hidden, so refreshing never
	// changes the height available to the graphs.
	progressSlot := container.NewGridWrap(fyne.NewSize(140, ui.progress.MinSize().Height), ui.progress)
	refreshStatus := container.NewBorder(nil, nil, nil, container.NewHBox(progressSlot,
		widget.NewButton("Details…", func() { ui.showRefreshDetails("Log refresh", ui.status.Text+"\n\n"+ui.directoryInfo.Text) })), ui.status)
	browse := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			if folder != nil {
				ui.path.SetText(folder.Path())
				ui.requestLoad()
			}
		}, ui.window).Show()
	})
	pathRow := container.NewBorder(nil, nil, widget.NewLabel("Log folder"), container.NewHBox(browse, widget.NewButton("Open folder", ui.requestLoad)), ui.path)
	apply := widget.NewButton("Apply UTC range", func() {
		if _, _, err := parseRange(strings.TrimSpace(ui.start.Text), strings.TrimSpace(ui.end.Text)); err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		ui.rangeSelect.SetSelected("Custom UTC range")
		ui.rebuildView()
	})
	filters := container.NewGridWithColumns(4,
		container.NewBorder(widget.NewLabel("Time range"), nil, nil, nil, ui.rangeSelect),
		container.NewBorder(widget.NewLabel("Start · UTC"), nil, nil, nil, ui.start),
		container.NewBorder(widget.NewLabel("End · UTC · exclusive"), nil, nil, nil, ui.end),
		container.NewBorder(widget.NewLabel("Graph interval"), nil, nil, nil, ui.bucketSelect))
	controls := container.NewVBox(heading, refreshStatus, ui.directoryInfo, pathRow, filters, container.NewHBox(apply, layout.NewSpacer(), ui.interval), widget.NewSeparator())
	all := widget.NewButton("All", func() { ui.selectRangeCharacters(true) })
	none := widget.NewButton("None", func() { ui.selectRangeCharacters(false) })
	groupNote := widget.NewLabel("Ship groups use history at each damage event's UTC time. Unobserved times stay unknown.")
	groupNote.Wrapping = fyne.TextWrapWord
	sidebar := container.NewBorder(container.NewVBox(widget.NewLabelWithStyle("GROUP / LEGEND", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), ui.groupSelect, groupNote, ui.characterSearch, container.NewGridWithColumns(2, all, none)), nil, nil, nil, container.NewVScroll(ui.characters))
	charts := container.NewGridWithRows(2,
		widget.NewCard("Outgoing damage", "DPS dealt by selected characters or ship groups", ui.graphPanContent(ui.outgoing)),
		widget.NewCard("Incoming damage", "DPS received by selected characters or ship groups", ui.graphPanContent(ui.incoming)))
	graphControls := container.NewBorder(nil, nil, nil, ui.graphFormatButton, ui.graphFormatLabel)
	graphContent := container.NewBorder(graphControls, nil, nil, nil, charts)
	totals := container.NewBorder(widget.NewLabel("All characters in this date range · average DPS includes idle time"), nil, nil, nil, ui.totals)
	tabs := container.NewAppTabs(container.NewTabItem("Damage graphs", graphContent), container.NewTabItem("Damage totals", totals), container.NewTabItem("ESI & ships", ui.esiContent()))
	ui.tabs = tabs
	split := container.NewHSplit(sidebar, tabs)
	split.SetOffset(0.21)
	footer := container.NewVBox(widget.NewSeparator(), ui.summary)
	return container.NewPadded(container.NewBorder(controls, footer, nil, nil, split))
}

func (ui *analyzerUI) requestLoad() {
	directory := strings.TrimSpace(ui.path.Text)
	if directory == "" {
		dialog.ShowError(fmt.Errorf("choose the EVE logs/gamelogs folder"), ui.window)
		return
	}
	select {
	case ui.loadRequests <- directory:
	default:
	}
}

// The worker owns the file cache. All widgets and view state stay on Fyne's UI thread.
func (ui *analyzerUI) watch() {
	index := &logIndex{}
	var directory string
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
		case directory = <-ui.loadRequests:
		case <-ticker.C:
			enabled := false
			fyne.DoAndWait(func() { enabled = ui.auto.Checked })
			if !enabled || directory == "" {
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
		snapshot, err := index.refresh(ctx, directory, logLoadingCutoff(time.Now(), lookbackDays), func(done, total int) {
			fyne.Do(func() {
				ui.progress.SetValue(float64(done) / float64(total))
				ui.status.SetText(fmt.Sprintf("Indexing %d / %d log files…", done, total))
			})
		})
		if ctx.Err() != nil {
			return
		}
		fyne.DoAndWait(func() {
			ui.refresh.Enable()
			ui.progress.Hide()
			if err != nil {
				ui.status.SetText("Could not load folder: " + err.Error())
				return
			}
			if lookbackDays != ui.logLookbackDays {
				// Saving settings queues a new load. Do not display a completed
				// scan from the previous limit while that request is pending.
				return
			}
			ui.snapshot = snapshot
			if ui.logDirectory != directory {
				ui.clearGraphRangeHistory()
				ui.logDirectory = directory
			}
			for _, name := range snapshot.Characters {
				if _, known := ui.selected[name]; !known {
					ui.selected[name] = true
				}
			}
			ui.updateRange()
			ui.directoryInfo.SetText(fmt.Sprintf("%d log files loaded · %d too old · %d without filename dates · %d characters · %d duplicate exports skipped · %d files without a Listener header", snapshot.Files, snapshot.SkippedOld, snapshot.SkippedUndated, len(snapshot.Characters), snapshot.Duplicates, snapshot.Unassigned))
			status := "Updated " + time.Now().Format("15:04:05") + " · Logs stay on this computer."
			if snapshot.Files == 0 {
				if snapshot.SkippedOld+snapshot.SkippedUndated > 0 {
					status += " No logs matched the loading limit. Change Settings → Log loading to include older files."
				} else {
					status += " No .txt game logs found in this folder."
				}
			} else if snapshot.Last.IsZero() {
				status += " No supported English damage messages found."
			} else {
				status += " Latest combat: " + snapshot.Last.Format("2006-01-02 15:04:05") + " UTC."
			}
			if len(snapshot.Warnings) > 0 {
				status += fmt.Sprintf(" %d files could not be read. %s", len(snapshot.Warnings), snapshot.Warnings[0])
			}
			ui.status.SetText(status)
		})
	}
}

func (ui *analyzerUI) updateRange() {
	custom := ui.rangeSelect.Selected == "Custom UTC range"
	if custom {
		ui.start.Enable()
		ui.end.Enable()
	} else {
		start, end := presetRange(ui.rangeSelect.Selected, ui.snapshot)
		ui.settingRange = true
		ui.start.SetText(start.Format("2006-01-02 15:04:05"))
		ui.end.SetText(end.Format("2006-01-02 15:04:05"))
		ui.settingRange = false
		// Entries remain editable so Apply can switch straight to a custom range.
	}
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
	ui.view = buildView(ui.snapshot, start, end, bucket)
	ui.interval.SetText(fmt.Sprintf("DPS · %s buckets · UTC · max 1,200 buckets", ui.view.Bucket))
	ui.renderSelection()
	ui.rebuildCharacters()
}

func (ui *analyzerUI) rebuildCharacters() {
	var rows []fyne.CanvasObject
	active := ui.view.activeSeries()
	query := strings.ToLower(strings.TrimSpace(ui.characterSearch.Text))
	if ui.shipGrouping() {
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
}

func (ui *analyzerUI) selectRangeCharacters(selected bool) {
	for _, series := range ui.view.activeSeries() {
		ui.selected[series.Name] = selected
	}
	ui.rebuildCharacters()
	ui.renderSelection()
}

func (ui *analyzerUI) renderSelection() {
	in := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket, Logarithmic: ui.graphLogarithmic}
	if ui.graphAverage10s {
		in.AverageWindow = dpsAverageWindow
	}
	out := in
	var incoming, outgoing float64
	var hitsIn, hitsOut, count int
	active := ui.view.activeSeries()
	for _, series := range active {
		if !ui.selected[series.Name] {
			continue
		}
		count++
		incoming += series.TotalIncoming
		outgoing += series.TotalOutgoing
		hitsIn += series.IncomingHits
		hitsOut += series.OutgoingHits
	}
	ui.tableSeries = active
	graphSeries := active
	if ui.graphAverage10s && !ui.shipGrouping() {
		graphSeries = ui.view.tenSecondAverage(ui.snapshot)
	}
	if ui.shipGrouping() {
		ui.tableSeries = ui.groupedShipSeries(nil, false)
		graphSeries = ui.groupedShipSeries(ui.selected, ui.graphAverage10s)
	}
	for _, series := range graphSeries {
		if !ui.shipGrouping() && !ui.selected[series.Name] {
			continue
		}
		ink := characterColor(series.Name)
		in.Series = append(in.Series, plotSeries{Name: series.Name, Color: ink, Values: series.Incoming, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
		out.Series = append(out.Series, plotSeries{Name: series.Name, Color: ink, Values: series.Outgoing, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
	}
	ui.incoming.setData(in)
	ui.outgoing.setData(out)
	ui.totals.Refresh()
	ui.summary.SetText(fmt.Sprintf("%d selected   |   Outgoing %s damage / %d hits   |   Incoming %s damage / %d hits", count, shortNumber(outgoing), hitsOut, shortNumber(incoming), hitsIn))
	ui.updateGraphFormatLabel()
}

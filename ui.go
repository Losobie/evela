package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type analyzerUI struct {
	metadata            *graphMetadata
	eventsDirty         bool
	scopedOptions       map[string]map[string][]damageFilterOption
	scopedFilters       map[string]map[string]*damageFilterSelection
	visibleFilters      map[string]map[string]bool
	scopedFilterBars    map[string]*fyne.Container
	scopedFilterButtons map[string]map[string]*widget.Button

	window                            fyne.Window
	snapshot                          *logSnapshot
	view                              damageView
	analysis                          *logSnapshot
	selected                          map[string]bool
	path                              *widget.Entry
	rangeStart, rangeEnd              time.Time
	rangePreset                       string
	rangeLabel                        *widget.Label
	quickSelect                       *widget.Button
	dayDialog                         *dialog.CustomDialog
	dayCalendar                       *widget.Calendar
	status, summary, directoryInfo    *widget.Label
	releaseLink                       *widget.Hyperlink
	progress                          *widget.ProgressBar
	refresh                           *widget.Button
	auto                              *widget.Check
	characters                        *fyne.Container
	characterSearch                   *widget.Entry
	incoming, outgoing                *damageChart
	ewar                              *damageChart
	ewarMetric                        *widget.Select
	outgoingLogistics                 *damageChart
	incomingLogistics                 *damageChart
	mining                            *damageChart
	miningFilters                     map[string]*damageFilterSelection
	miningFilterButtons               map[string]*widget.Button
	logisticsTypes                    map[string]*damageFilterSelection
	logisticsTypeButtons              map[string]*widget.Button
	eventGraphGroupings               map[string]string
	graphsScroll                      *container.Scroll
	graphStack                        *fyne.Container
	graphCards                        map[string]*widget.Card
	graphsEmpty                       fyne.CanvasObject
	totals                            *widget.Table
	events                            *eventsViewer
	engagements                       *engagementsViewer
	loadRequests                      chan []string
	stop                              chan struct{}
	ships                             map[string]shipRecord
	shipHistory                       shipHistory
	locationHistory                   locationHistory
	logLocations                      logLocationHistory
	logLocationUpdates                chan logLocationHistory
	locationLookup                    locationResolver
	groupSelect                       *widget.Select
	characterGroups                   []characterGroup
	characterGroupEditor              *characterGroupEditor
	tableSeries                       []characterSeries
	totalsSortColumn                  int
	totalsSortDescending              bool
	clientID                          *widget.Entry
	esiStatus                         *widget.Label
	esiConnections                    *fyne.Container
	shipAuto                          *widget.Check
	shipRefresh, connect, cancelLogin *widget.Button
	esiActions                        chan esiAction
	esiLogins                         chan esiLoginRequest
	cancelESILogin                    context.CancelFunc
	cancelESIRefresh                  context.CancelFunc
	tabs                              *container.AppTabs
	callbackPort                      *widget.Entry
	callbackURL                       *widget.Label
	esiDialogStatus                   *widget.Label
	tokenConnections                  *fyne.Container
	esiDialog                         *dialog.CustomDialog
	esiSettingsTabs                   *container.AppTabs
	esiSettingsDirty, settingESI      bool
	graphUndo, graphRedo              []graphRangeChange
	logDirectory                      string
	logLookbackDays                   int
	logLoadingDialog                  *dialog.CustomDialog
	graphAverageWindow                time.Duration
	graphInvulnerabilityMarkers       bool
	graphFormatButton                 *widget.Button
	graphOptionsButton                *widget.Button
	incomingGroupButton               *widget.Button
	incomingGrouping                  string
	graphMenuPopup                    *widget.PopUpMenu
	damageFilters                     map[string]*damageFilterSelection
	filterOptions                     map[string][]damageFilterOption
	filterButtons                     map[string]*widget.Button
	filterPopup                       *damageFilterPopup
}

func newAnalyzerUI(window fyne.Window) *analyzerUI {
	ui := &analyzerUI{window: window, snapshot: &logSnapshot{}, selected: make(map[string]bool), loadRequests: make(chan []string, 1), stop: make(chan struct{})}
	ui.path = widget.NewEntry()
	ui.path.SetText(strings.Join(savedLogFolders(), "\n"))
	ui.logLookbackDays = savedLogLookbackDays()
	ui.loadCharacterGroups()
	ui.rangeLabel = widget.NewLabel("")
	ui.rangeLabel.Selectable = true
	ui.rangeLabel.Truncation = fyne.TextTruncateEllipsis
	ui.quickSelect = ui.toggleMenuButton("Quick Select", ui.quickRangeMenu, nil)
	ui.status = widget.NewLabel("Ready to read game logs")
	ui.status.Wrapping = fyne.TextWrapWord
	ui.status.Truncation = fyne.TextTruncateEllipsis
	ui.directoryInfo = widget.NewLabel("")
	ui.directoryInfo.Wrapping = fyne.TextWrapWord
	ui.directoryInfo.Truncation = fyne.TextTruncateEllipsis
	ui.summary = widget.NewLabel("Waiting for logs…")
	ui.summary.TextStyle.Bold = true
	ui.summary.Truncation = fyne.TextTruncateEllipsis
	ui.releaseLink = widget.NewHyperlink("New release available", nil)
	ui.releaseLink.Hide()
	ui.progress = widget.NewProgressBar()
	ui.progress.Hide()
	ui.refresh = widget.NewButtonWithIcon("Refresh logs", theme.ViewRefreshIcon(), ui.requestLoad)
	ui.auto = widget.NewCheck("Auto refresh · 10s", nil)
	ui.auto.SetChecked(true)
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
	ui.initMining()
	ui.initScopedFilters()
	linkChartHovers(ui.outgoing, ui.outgoingLogistics, ui.incoming, ui.incomingLogistics, ui.ewar, ui.mining)
	ui.initEventGraphGrouping()
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
				if ui.damageAttributeGrouping() || ui.systemGrouping() {
					first, second = ui.groupSelect.Selected, "Members"
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
	ui.events.contextAt = func(character string, second int64) (string, string) {
		attributes := ui.graphMetadata().attributes(character, second)
		return attributes.hull.Label, attributes.system.Label
	}
	ui.engagements = newEngagementsViewer(ui.openEngagement)
	ui.loadGraphRangePreference()
	ui.initGraphRangeShortcuts()
	for _, chart := range []*damageChart{ui.outgoing, ui.incoming, ui.outgoingLogistics, ui.incomingLogistics, ui.ewar, ui.mining} {
		chart.OnPanRange = ui.applyGraphPanRange
	}
	window.SetMainMenu(ui.mainMenu())
	return ui
}

func (ui *analyzerUI) content() fyne.CanvasObject {
	// Keep the progress slot even when its bar is hidden, so refreshing never
	// changes the height available to the graphs.
	progressSlot := container.NewGridWrap(fyne.NewSize(140, ui.progress.MinSize().Height), ui.progress)
	details := widget.NewButton("Details…", func() { ui.showRefreshDetails("Log refresh", ui.status.Text+"\n\n"+ui.directoryInfo.Text) })
	refreshControls := container.NewHBox(progressSlot, ui.auto, ui.refresh, details)
	heading := container.NewBorder(nil, nil, ui.quickSelect, refreshControls, ui.rangeLabel)
	controls := container.NewVBox(heading, widget.NewSeparator())
	filterBar := ui.damageFilterBar()
	ui.graphCards = map[string]*widget.Card{}
	ui.graphStack = container.NewVBox(
		ui.newGraphCard(graphOutgoing, "Outgoing damage", container.NewBorder(ui.scopedFilterBar(graphOutgoing, ui.graphFormatButton), nil, nil, nil, ui.outgoing)),
		ui.newGraphCard(graphOutgoingLogistics, "Outgoing logistics", container.NewBorder(ui.logisticsControls(graphOutgoingLogistics), nil, nil, nil, ui.outgoingLogistics)),
		ui.newGraphCard(graphIncoming, "Incoming damage", container.NewBorder(ui.incomingDamageFilterBar(), nil, nil, nil, ui.incoming)),
		ui.newGraphCard(graphIncomingLogistics, "Incoming logistics", container.NewBorder(ui.logisticsControls(graphIncomingLogistics), nil, nil, nil, ui.incomingLogistics)),
		ui.newGraphCard(graphIncomingEWar, "Incoming E-War", container.NewBorder(ui.ewarControls(), nil, nil, nil, ui.ewar)),
		ui.newGraphCard(graphMining, "Mining", container.NewBorder(ui.miningControls(), nil, nil, nil, ui.mining)))
	ui.graphsScroll = container.NewVScroll(ui.graphStack)
	ui.graphsEmpty = newGraphsEmptyState()
	ui.applyGraphVisibility()
	graphContent := container.NewBorder(filterBar, nil, nil, nil, ui.graphsScroll)
	totalsNote := widget.NewLabel("Current filters and time range · average DPS includes idle time\nClick a heading to sort; click again to reverse. Hits sort by outgoing, then incoming.")
	totalsNote.Wrapping = fyne.TextWrapWord
	totals := container.NewBorder(totalsNote, nil, nil, nil, ui.totals)
	tabs := container.NewAppTabs(container.NewTabItem("Graphs", graphContent), container.NewTabItem("Totals", totals), container.NewTabItem("Events", ui.events.content), container.NewTabItem("Engagements", ui.engagements.content), container.NewTabItem("ESI & ships", ui.esiContent()))
	ui.tabs = tabs
	tabs.OnSelected = func(item *container.TabItem) {
		if item.Content == ui.events.content {
			ui.refreshEvents()
		}
		if item.Content == ui.engagements.content {
			ui.refreshEngagements()
		}
	}
	tabs.OnUnselected = func(item *container.TabItem) {
		if item.Content == ui.engagements.content {
			ui.engagements.cancelBuild()
		}
	}
	footer := container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, nil, ui.releaseLink, ui.summary))
	return container.NewPadded(container.NewBorder(controls, footer, nil, nil, tabs))
}

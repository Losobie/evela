package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type esiAction struct {
	Kind, ClientID string
	CharacterID    int64
	CallbackPort   int
	Scopes         []string
}

func (ui *analyzerUI) initESI() {
	ui.ships = map[string]shipRecord{}
	ui.esiActions = make(chan esiAction, 8)
	ui.esiLogins = make(chan esiLoginRequest, 1)
	ui.logLocationUpdates = make(chan logLocationHistory, 1)
	ui.clientID = widget.NewEntry()
	ui.clientID.SetPlaceHolder(defaultESIClientID + " (default)")
	ui.initESISettings()
	ui.esiStatus = widget.NewLabel("Set up EVE SSO to look up current ships.")
	ui.esiStatus.Wrapping = fyne.TextWrapWord
	ui.esiStatus.Truncation = fyne.TextTruncateEllipsis
	ui.initESITable()
	ui.shipAuto = widget.NewCheck("Refresh every 30 seconds", nil)
	ui.shipAuto.SetChecked(true)
	ui.connect = widget.NewButton("Sign in with EVE Online", ui.openESILogin)
	ui.loginSubmit = widget.NewButton("Sign in", func() { ui.requestESIConfiguration("connect") })
	ui.cancelLogin = widget.NewButton("Cancel sign-in", func() {
		if ui.cancelESILogin != nil {
			ui.cancelESILogin()
		}
	})
	ui.cancelLogin.Hide()
	ui.initESILoginScopes()
	ui.groupSelect = widget.NewSelect(ui.characterGroupingOptions(), func(string) {
		ui.saveGraphFormatPreferences()
		ui.rebuildCharacters()
		ui.renderSelection()
	})
	ui.groupSelect.Selected = "Characters"
}

func (ui *analyzerUI) esiContent() fyne.CanvasObject {
	controls := container.NewBorder(nil, nil, nil, container.NewHBox(ui.shipAuto, widget.NewButton("Ship & Location History", ui.openShipLocationHistory)), container.NewHBox(widget.NewButton("Configure ESI", ui.openESISettings), ui.connect))
	return container.NewPadded(container.NewBorder(controls, nil, nil, nil, ui.esiTableContent()))
}

func (ui *analyzerUI) requestESI(action esiAction) bool {
	select {
	case ui.esiActions <- action:
		return true
	default:
		ui.setESIStatus("ESI is busy; wait for the current operation to finish.")
		return false
	}
}

func (ui *analyzerUI) watchESI() {
	directory, err := os.UserConfigDir()
	if err != nil {
		fyne.Do(func() { ui.setESIStatus("Find ESI settings folder: " + err.Error()) })
		reportRuntimeError("Find ESI settings folder", err)
		return
	}
	service := newESIService(filepath.Join(directory, "Evela", "esi.dat"))
	ui.watchESIService(service, ui.openESIBrowser)
}

func (ui *analyzerUI) watchESIService(service *esiService, openBrowser func(string) error) {
	loadErr := service.load()
	fyne.DoAndWait(func() {
		if !ui.esiSettingsDirty {
			ui.loadESIConfiguration(service.config.ClientID, service.config.CallbackPort)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ui.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	loginResults := make(chan esiLoginResult, 1)
	loginDone := make(chan struct{})
	go func() {
		defer close(loginDone)
		ui.watchESILogins(ctx, service, loginResults, openBrowser)
	}()
	defer func() {
		cancel()
		<-loginDone
		// A completed sign-in may have interrupted a refresh just as the app
		// closed. Persist it before the storage owner exits.
		select {
		case result := <-loginResults:
			close(result.taken)
			if err := service.acceptLogin(result.action.ClientID, result.action.CallbackPort, result.account); err != nil {
				fyne.LogError("Save completed EVE sign-in", err)
			}
		default:
		}
	}()
	publish := func(message string) {
		records := service.records()
		history := service.config.ShipHistory.clone()
		locations := service.config.LocationHistory.clone()
		logLocations := service.config.LogLocations.clone()
		fyne.DoAndWait(func() {
			ui.shipHistory = history
			ui.locationHistory = locations
			if ui.logLocations == nil {
				ui.logLocations = logLocationHistory{}
			}
			ui.logLocations.merge(logLocations)
			ui.engagements.invalidate()
			ui.applyShipRecords(records)
			ui.setESIStatus(message)
		})
	}
	message := "Connect characters to record ship history for damage grouping."
	if loadErr != nil {
		reportRuntimeError("Load ESI settings", loadErr)
		message = loadErr.Error()
	}
	publish(message)
	refresh := func() {
		if len(service.config.Accounts) == 0 && len(service.config.ShipHistory) == 0 && len(service.config.LocationHistory) == 0 {
			publish("No characters connected. Sign in with EVE Online first.")
			return
		}
		refreshCtx, finish := context.WithTimeout(ctx, 25*time.Second)
		fyne.DoAndWait(func() {
			ui.cancelESIRefresh = finish
			ui.setESIStatus("Querying current ships and locations from ESI…")
		})
		if len(loginResults) > 0 {
			finish()
		}
		err := service.refreshShips(refreshCtx)
		interrupted := refreshCtx.Err() == context.Canceled
		finish()
		fyne.DoAndWait(func() { ui.cancelESIRefresh = nil })
		if ctx.Err() != nil {
			return
		}
		if interrupted {
			return
		}
		message := "Ships and locations updated " + time.Now().UTC().Format("15:04:05") + " · " + strconv.Itoa(len(service.config.Accounts)) + " connected characters."
		if err != nil {
			reportRuntimeError("Refresh ESI ships and locations", err)
			message = err.Error()
		}
		publish(message)
	}
	if len(service.config.Accounts) > 0 || len(service.config.ShipHistory) > 0 || len(service.config.LocationHistory) > 0 {
		refresh()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	logHistoryDirty := false
	saveLogLocations := func() {
		if !logHistoryDirty {
			return
		}
		if err := service.save(); err != nil {
			reportRuntimeError("Save log location history", err)
			publish("Save log location history: " + err.Error())
		} else {
			logHistoryDirty = false
			publish("Log location history saved.")
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-loginResults:
			close(result.taken)
			if err := service.acceptLogin(result.action.ClientID, result.action.CallbackPort, result.account); err != nil {
				reportRuntimeError("Save EVE sign-in", err)
				publish("EVE sign-in: " + err.Error())
			} else {
				publish("Connected " + result.account.Ship.CharacterName + ".")
				refresh()
			}
		case locations := <-ui.logLocationUpdates:
			if service.config.LogLocations == nil {
				service.config.LogLocations = logLocationHistory{}
			}
			if service.config.LogLocations.merge(locations) {
				logHistoryDirty = true
			}
			saveLogLocations()
		case <-ticker.C:
			saveLogLocations()
			enabled := false
			fyne.DoAndWait(func() { enabled = ui.shipAuto.Checked })
			if enabled && len(service.config.Accounts) > 0 {
				refresh()
			} else {
				records := service.records()
				fyne.Do(func() { ui.applyShipRecords(records) })
			}
		case action := <-ui.esiActions:
			switch action.Kind {
			case "configure":
				if err := service.configure(action.ClientID, action.CallbackPort); err != nil {
					reportRuntimeError("Save ESI configuration", err)
					publish(err.Error())
					fyne.Do(func() { dialog.ShowError(err, ui.window) })
					continue
				}
				publish("ESI configuration saved. Register " + esiCallbackURL(service.config.CallbackPort) + " in your EVE application.")
			case "refresh":
				refresh()
			case "disconnect":
				account := service.config.Accounts[action.CharacterID]
				delete(service.config.Accounts, action.CharacterID)
				if err := service.save(); err != nil {
					reportRuntimeError("Save ESI disconnect", err)
					if account != nil {
						service.config.Accounts[action.CharacterID] = account
					}
					publish("Could not save disconnect: " + err.Error())
				} else {
					publish("Character disconnected from Evela.")
				}
			}
		}
	}
}

func (ui *analyzerUI) applyShipRecords(records []shipRecord) {
	ui.ships = map[string]shipRecord{}
	locations := ui.resolvedLocations()
	ui.esiRows = nil
	for _, record := range records {
		ui.ships[characterKey(record.CharacterName)] = record
		text := record.TypeName
		if text == "" {
			text = "Ship unavailable"
		}
		checked := "Not checked"
		if !record.CheckedAt.IsZero() {
			checked = record.CheckedAt.UTC().Format("2006-01-02 15:04:05")
		}
		locationText := locationLabel(record.Location)
		if location, known := locations.atName(record.CharacterName, time.Now()); location.Source == "Game log" {
			locationText = "Unknown system · conflicting game logs"
			if known {
				locationText = location.SystemName
				current := record.Location
				sameSystem := current.SystemID > 0 && current.SystemID == location.SystemID || current.SystemName != "" && strings.EqualFold(current.SystemName, location.SystemName)
				if sameSystem && current.Error == "" && time.Since(current.CheckedAt) < locationHistoryMaxGap {
					current.SystemName = location.SystemName
					locationText = locationLabel(current)
				}
			}
		}
		ui.esiRows = append(ui.esiRows, [5]string{record.CharacterName, checked, text, locationText, characterESIStatus(record)})
	}
	ui.esiConnections.Refresh()
	ui.rebuildView()
}

func (ui *analyzerUI) shipGrouping() bool {
	return ui.groupSelect != nil && (ui.groupSelect.Selected == "Ship history" || ui.shipClassGrouping())
}

func (ui *analyzerUI) shipClassGrouping() bool {
	return ui.groupSelect != nil && ui.groupSelect.Selected == "Ship class history"
}

func (ui *analyzerUI) groupedShipSeries(selected map[string]bool, averageWindow time.Duration) []characterSeries {
	return ui.view.historicalShipSeries(ui.analysisSnapshot(), ui.shipHistory, selected, ui.shipClassGrouping(), averageWindow, ui.graphMetadata())
}

func (ui *analyzerUI) characterRow(name, group, shipNames string) fyne.CanvasObject {
	check := widget.NewCheck(name, nil)
	check.SetChecked(ui.selected[name])
	check.OnChanged = func(checked bool) {
		ui.selected[name] = checked
		ui.syncCharacterFilter()
		ui.renderSelection()
		ui.rebuildCharacters()
	}
	if group == "" {
		group = name
	}
	swatch := newSeriesSwatch(group)
	label := widget.NewLabel(shipNames)
	label.TextStyle.Italic = true
	return container.NewBorder(nil, nil, swatch, nil, container.NewVBox(check, label))
}

func (ui *analyzerUI) groupedCharacterList(query string) []fyne.CanvasObject {
	var rows []fyne.CanvasObject
	var groups []characterSeries
	if ui.systemGrouping() {
		groups = ui.view.historicalAttributeSeries(ui.analysisSnapshot(), ui.shipHistory, ui.locationLookup, nil, filterSystem, ui.graphAverageWindow, ui.graphMetadata())
	} else if ui.damageAttributeGrouping() {
		groups = ui.view.damageAttributeSeries(ui.analysisSnapshot(), nil, ui.groupSelect.Selected, ui.graphAverageWindow)
	} else if ui.characterGroupGrouping() {
		series := ui.view.activeSeries()
		if ui.graphAverageWindow > 0 {
			series = ui.view.rollingAverage(ui.analysisSnapshot(), ui.graphAverageWindow)
		}
		groups = ui.groupCustomCharacterSeries(series, nil)
	} else {
		groups = ui.groupedShipSeries(nil, ui.graphAverageWindow)
	}
	for _, group := range groups {
		var names []string
		for _, name := range group.Members {
			if strings.Contains(strings.ToLower(name+" "+group.Name+" "+strings.Join(group.MemberShips[name], " ")), query) {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			continue
		}
		members := append([]string(nil), group.Members...)
		selectGroup := widget.NewButton("Select", func() {
			for _, name := range members {
				ui.selected[name] = true
			}
			ui.syncCharacterFilter()
			ui.rebuildCharacters()
			ui.renderSelection()
		})
		rows = append(rows, container.NewBorder(nil, nil, nil, selectGroup, widget.NewLabelWithStyle(fmt.Sprintf("%s (%d)", group.Name, len(group.Members)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})))
		for _, name := range names {
			rows = append(rows, ui.characterRow(name, group.Name, strings.Join(group.MemberShips[name], ", ")))
		}
	}
	return rows
}

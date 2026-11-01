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
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type esiAction struct {
	Kind, ClientID string
	CharacterID    int64
	CallbackPort   int
}

func (ui *analyzerUI) initESI() {
	ui.ships = map[string]shipRecord{}
	ui.esiActions = make(chan esiAction, 8)
	ui.esiLogins = make(chan esiLoginRequest, 1)
	ui.logLocationUpdates = make(chan logLocationHistory, 1)
	ui.clientID = widget.NewEntry()
	ui.clientID.SetPlaceHolder("Public EVE application client ID")
	ui.clientID.SetText(defaultESIClientID)
	ui.initESISettings()
	ui.esiStatus = widget.NewLabel("Set up EVE SSO to look up current ships.")
	ui.esiStatus.Wrapping = fyne.TextWrapWord
	ui.esiStatus.Truncation = fyne.TextTruncateEllipsis
	ui.esiConnections = container.NewVBox()
	ui.shipAuto = widget.NewCheck("Refresh every 30 seconds", nil)
	ui.shipAuto.SetChecked(true)
	ui.shipRefresh = widget.NewButton("Refresh ships & locations", func() { ui.requestESI(esiAction{Kind: "refresh"}) })
	ui.connect = widget.NewButton("Sign in with EVE Online", func() { ui.requestESIConfiguration("connect") })
	ui.cancelLogin = widget.NewButton("Cancel sign-in", func() {
		if ui.cancelESILogin != nil {
			ui.cancelESILogin()
		}
	})
	ui.cancelLogin.Hide()
	ui.groupSelect = widget.NewSelect(ui.characterGroupingOptions(), func(string) {
		ui.saveGraphFormatPreferences()
		ui.rebuildCharacters()
		ui.renderSelection()
	})
	ui.groupSelect.Selected = "Characters"
}

func (ui *analyzerUI) esiContent() fyne.CanvasObject {
	intro := widget.NewLabel("Connect characters through Settings → ESI configuration. Sign in with EVE Online for each character. Matching log characters are linked automatically by name.")
	intro.Wrapping = fyne.TextWrapWord
	controls := container.NewBorder(nil, nil, nil, container.NewHBox(widget.NewButton("Ship history…", ui.openShipHistory), widget.NewButton("Location history…", ui.openLocationHistory)), container.NewHBox(widget.NewButton("Configure ESI…", ui.openESISettings), ui.shipRefresh, ui.shipAuto))
	settings := container.NewVBox(intro, controls, ui.esiStatusRow(ui.esiStatus), widget.NewSeparator(), widget.NewLabel("CONNECTED CHARACTERS / CURRENT SHIPS & LOCATIONS"))
	return container.NewPadded(container.NewBorder(settings, nil, nil, nil, container.NewVScroll(ui.esiConnections)))
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
		return
	}
	service := newESIService(filepath.Join(directory, "Evela", "esi.dat"))
	ui.watchESIService(service, ui.openESIBrowser)
}

func (ui *analyzerUI) watchESIService(service *esiService, openBrowser func(string) error) {
	loadErr := service.load()
	fyne.DoAndWait(func() {
		if !ui.esiSettingsDirty {
			ui.settingESI = true
			ui.clientID.SetText(service.config.ClientID)
			ui.callbackPort.SetText(strconv.Itoa(service.config.CallbackPort))
			ui.settingESI = false
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
			ui.applyShipRecords(records)
			ui.setESIStatus(message)
		})
	}
	message := "Connect characters to record ship history for damage grouping."
	if loadErr != nil {
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
			ui.shipRefresh.Disable()
			ui.setESIStatus("Querying current ships and locations from ESI…")
		})
		if len(loginResults) > 0 {
			finish()
		}
		err := service.refreshShips(refreshCtx)
		interrupted := refreshCtx.Err() == context.Canceled
		finish()
		fyne.DoAndWait(func() { ui.cancelESIRefresh = nil; ui.shipRefresh.Enable() })
		if ctx.Err() != nil {
			return
		}
		if interrupted {
			return
		}
		message := "Ships and locations updated " + time.Now().Format("15:04:05") + " · " + strconv.Itoa(len(service.config.Accounts)) + " connected characters."
		if err != nil {
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
					publish(err.Error())
					continue
				}
				publish("ESI configuration saved. Register " + esiCallbackURL(service.config.CallbackPort) + " in your EVE application.")
			case "refresh":
				refresh()
			case "disconnect":
				account := service.config.Accounts[action.CharacterID]
				delete(service.config.Accounts, action.CharacterID)
				if err := service.save(); err != nil {
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
	var connections, tokens []fyne.CanvasObject
	for _, record := range records {
		ui.ships[characterKey(record.CharacterName)] = record
		name := widget.NewLabelWithStyle(record.CharacterName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		text := shipLabel(record.CharacterName, ui.ships)
		if record.ClassName != "" {
			text += " · " + record.ClassName
		}
		if record.ShipName != "" {
			text += " · " + record.ShipName
		}
		if !record.CheckedAt.IsZero() {
			text += " · checked " + record.CheckedAt.Local().Format("15:04:05")
		}
		if record.Error != "" {
			text += " · " + record.Error
		}
		if record.ClassError != "" {
			text += " · " + record.ClassError
		}
		locationText := locationLabel(record.Location)
		if location, known := locations.atName(record.CharacterName, time.Now()); location.Source == "Game log" {
			locationText = "Unknown system · conflicting game logs"
			if known {
				locationText = location.SystemName + " · game log"
			}
		}
		text += "\n" + locationText
		details := widget.NewLabel(text)
		details.Wrapping = fyne.TextWrapWord
		id := record.CharacterID
		remove := widget.NewButton("Disconnect", func() { ui.requestESI(esiAction{Kind: "disconnect", CharacterID: id}) })
		connections = append(connections, container.NewBorder(nil, nil, nil, remove, container.NewVBox(name, details)))
		tokenStatus := "Access token expires " + record.TokenExpiresAt.Local().Format("2006-01-02 15:04:05")
		if record.TokenExpiresAt.IsZero() {
			tokenStatus = "Access token expiry unavailable"
		}
		if record.CanRefresh {
			tokenStatus += " · refresh token saved"
		} else {
			tokenStatus += " · no refresh token"
		}
		tokenLabel := widget.NewLabel(tokenStatus)
		tokenLabel.Wrapping = fyne.TextWrapWord
		removeTokens := widget.NewButton("Disconnect", func() { ui.requestESI(esiAction{Kind: "disconnect", CharacterID: id}) })
		tokens = append(tokens, container.NewBorder(nil, nil, nil, removeTokens, container.NewVBox(widget.NewLabelWithStyle(record.CharacterName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), tokenLabel)))
	}
	if len(records) == 0 {
		connections = append(connections, widget.NewLabel("No connected characters yet."))
		tokens = append(tokens, widget.NewLabel("No saved character tokens yet."))
	}
	ui.esiConnections.Objects = connections
	ui.esiConnections.Refresh()
	ui.tokenConnections.Objects = tokens
	ui.tokenConnections.Refresh()
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
	swatch := canvas.NewRectangle(characterColor(group))
	swatch.SetMinSize(fyne.NewSize(6, 18))
	label := widget.NewLabel(shipNames)
	label.TextStyle.Italic = true
	return container.NewBorder(nil, nil, swatch, nil, container.NewVBox(check, label))
}

func (ui *analyzerUI) groupedCharacterList(query string) []fyne.CanvasObject {
	var rows []fyne.CanvasObject
	var groups []characterSeries
	if ui.systemGrouping() {
		groups = ui.view.historicalAttributeSeries(ui.analysisSnapshot(), ui.shipHistory, ui.locationLookup, nil, filterSystem, ui.graphAverageWindow, ui.graphMetadata())
	} else if ui.weaponGrouping() {
		groups = ui.view.weaponSeries(ui.analysisSnapshot(), nil, ui.groupSelect.Selected, ui.graphAverageWindow)
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

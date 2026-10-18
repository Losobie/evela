package main

import (
	"context"
	"fmt"
	"net/url"
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
	Kind, ClientID            string
	CharacterID               int64
	CallbackPort              int
	AccessToken, RefreshToken string
	AuthMode, ClientSecret    string
}

func (ui *analyzerUI) initESI() {
	ui.ships = map[string]shipRecord{}
	ui.esiActions = make(chan esiAction, 8)
	ui.clientID = widget.NewEntry()
	ui.clientID.SetPlaceHolder("Public EVE application client ID")
	ui.initESISettings()
	ui.esiStatus = widget.NewLabel("Set up EVE SSO to look up current ships.")
	ui.esiStatus.Wrapping = fyne.TextWrapWord
	ui.esiStatus.Truncation = fyne.TextTruncateEllipsis
	ui.esiConnections = container.NewVBox()
	ui.shipAuto = widget.NewCheck("Refresh ships every 30 seconds", nil)
	ui.shipAuto.SetChecked(true)
	ui.shipRefresh = widget.NewButton("Refresh ships", func() { ui.requestESI(esiAction{Kind: "refresh"}) })
	ui.connect = widget.NewButton("Sign in with EVE Online", func() { ui.requestESIConfiguration("connect") })
	ui.cancelLogin = widget.NewButton("Cancel sign-in", func() {
		if ui.cancelESILogin != nil {
			ui.cancelESILogin()
		}
	})
	ui.cancelLogin.Hide()
	ui.groupSelect = widget.NewSelect([]string{"Characters", "Ship history", "Ship class history"}, func(string) {
		ui.saveGraphFormatPreferences()
		ui.rebuildCharacters()
		ui.renderSelection()
	})
	ui.groupSelect.Selected = "Characters"
}

func (ui *analyzerUI) esiContent() fyne.CanvasObject {
	intro := widget.NewLabel("Connect characters through Settings → ESI configuration. Sign in with EVE Online or import existing tokens for your application. Matching log characters are linked automatically by name.")
	intro.Wrapping = fyne.TextWrapWord
	controls := container.NewBorder(nil, nil, nil, widget.NewButton("Ship history…", ui.openShipHistory), container.NewHBox(widget.NewButton("Configure ESI…", ui.openESISettings), ui.shipRefresh, ui.shipAuto))
	settings := container.NewVBox(intro, controls, ui.esiStatusRow(ui.esiStatus), widget.NewSeparator(), widget.NewLabel("CONNECTED CHARACTERS / CURRENT SHIPS"))
	note := widget.NewLabel("Graph and totals grouping uses ship history at each damage event's UTC time. Events outside observed periods appear under Unknown ship / Unknown ship class. A character can appear in several groups after changing ships. Ship classes come from ESI groups. View recorded periods in Ship history. Saved sign-ins and history are encrypted for your Windows user.")
	note.Wrapping = fyne.TextWrapWord
	return container.NewPadded(container.NewBorder(settings, note, nil, nil, container.NewVScroll(ui.esiConnections)))
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
	loadErr := service.load()
	fyne.DoAndWait(func() {
		if !ui.esiSettingsDirty {
			ui.settingESI = true
			ui.clientID.SetText(service.config.ClientID)
			ui.clientSecret.SetText(service.config.ClientSecret)
			mode := "Native / desktop (PKCE)"
			if service.config.AuthMode == esiAuthClientSecret {
				mode = "Client ID + secret"
			}
			ui.esiAuthMode.SetSelected(mode)
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
	publish := func(message string) {
		records := service.records()
		history := service.config.ShipHistory.clone()
		fyne.DoAndWait(func() {
			ui.shipHistory = history
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
		if len(service.config.Accounts) == 0 && len(service.config.ShipHistory) == 0 {
			publish("No characters connected. Sign in with EVE Online first.")
			return
		}
		fyne.Do(func() { ui.shipRefresh.Disable(); ui.setESIStatus("Querying current ships from ESI…") })
		refreshCtx, finish := context.WithTimeout(ctx, 25*time.Second)
		err := service.refreshShips(refreshCtx)
		finish()
		if ctx.Err() != nil {
			return
		}
		message := "Ships updated " + time.Now().Format("15:04:05") + " · " + strconv.Itoa(len(service.config.Accounts)) + " connected characters."
		if err != nil {
			message = err.Error()
		}
		publish(message)
		fyne.Do(func() { ui.shipRefresh.Enable() })
	}
	if len(service.config.Accounts) > 0 || len(service.config.ShipHistory) > 0 {
		refresh()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
			case "configure", "connect", "import":
				if err := service.configureApplication(action.ClientID, action.CallbackPort, action.AuthMode, action.ClientSecret); err != nil {
					publish(err.Error())
					continue
				}
				if action.Kind == "configure" {
					publish("ESI configuration saved. Register " + esiCallbackURL(service.config.CallbackPort) + " in your EVE application.")
					continue
				}
				if action.Kind == "import" {
					fyne.Do(func() { ui.setESIStatus("Validating and importing ESI tokens…") })
					importCtx, finish := context.WithTimeout(ctx, 30*time.Second)
					_, importErr := service.importTokens(importCtx, action.AccessToken, action.RefreshToken)
					finish()
					if ctx.Err() != nil {
						return
					}
					if importErr != nil {
						publish("Import ESI tokens: " + importErr.Error())
					} else {
						refresh()
					}
					continue
				}
				loginCtx, finish := context.WithTimeout(ctx, 5*time.Minute)
				fyne.DoAndWait(func() {
					ui.cancelESILogin = finish
					ui.connect.Disable()
					ui.cancelLogin.Show()
					ui.setESIStatus("Complete EVE sign-in in your browser. Select a character and grant current-ship access.")
				})
				_, loginErr := service.authorize(loginCtx, func(address string) error {
					parsed, err := url.Parse(address)
					if err != nil {
						return err
					}
					fyne.DoAndWait(func() { err = fyne.CurrentApp().OpenURL(parsed) })
					return err
				})
				finish()
				if ctx.Err() != nil {
					return
				}
				fyne.Do(func() { ui.cancelESILogin = nil; ui.connect.Enable(); ui.cancelLogin.Hide() })
				if loginErr != nil {
					publish("EVE sign-in: " + loginErr.Error())
				} else {
					refresh()
				}
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
	ui.rebuildCharacters()
	ui.renderSelection()
}

func (ui *analyzerUI) shipGrouping() bool {
	return ui.groupSelect != nil && (ui.groupSelect.Selected == "Ship history" || ui.shipClassGrouping())
}

func (ui *analyzerUI) shipClassGrouping() bool {
	return ui.groupSelect != nil && ui.groupSelect.Selected == "Ship class history"
}

func (ui *analyzerUI) groupedShipSeries(selected map[string]bool, average bool) []characterSeries {
	return ui.view.historicalShipSeries(ui.snapshot, ui.shipHistory, selected, ui.shipClassGrouping(), average)
}

func (ui *analyzerUI) characterRow(name, group, shipNames string) fyne.CanvasObject {
	check := widget.NewCheck(name, nil)
	check.SetChecked(ui.selected[name])
	check.OnChanged = func(checked bool) { ui.selected[name] = checked; ui.renderSelection() }
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
	for _, group := range ui.groupedShipSeries(nil, ui.graphAverage10s) {
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

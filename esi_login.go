package main

import (
	"context"
	"errors"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type esiLoginRequest struct {
	action esiAction
	ctx    context.Context
	cancel context.CancelFunc
}

type esiLoginResult struct {
	action  esiAction
	account *esiAccount
	taken   chan struct{}
}

func (ui *analyzerUI) initESILoginScopes() {
	ui.loginStatus = widget.NewLabel("")
	ui.loginStatus.Truncation = fyne.TextTruncateEllipsis
	for _, scope := range []string{shipScope, locationScope, structureScope} {
		check := widget.NewCheck(scope, nil)
		check.SetChecked(true)
		ui.loginScopeChecks = append(ui.loginScopeChecks, check)
	}
}

func (ui *analyzerUI) openESILogin() {
	if ui.esiLoginDialog != nil {
		ui.esiLoginDialog.Show()
		return
	}
	content := container.NewVBox(widget.NewLabel("Scopes:"))
	for _, check := range ui.loginScopeChecks {
		content.Add(check)
	}
	content.Add(ui.loginStatus)
	content.Add(container.NewHBox(ui.loginSubmit, ui.cancelLogin))
	ui.esiLoginDialog = dialog.NewCustom("Sign in with EVE Online", "Close", content, ui.window)
	ui.esiLoginDialog.SetOnClosed(func() {
		if ui.cancelESILogin != nil {
			ui.cancelESILogin()
		}
		ui.esiLoginDialog = nil
	})
	ui.esiLoginDialog.Resize(fyne.NewSize(580, 310))
	ui.esiLoginDialog.Show()
}

func (ui *analyzerUI) showESILoginStatus(message string) {
	ui.loginStatus.SetText(message)
}

func (ui *analyzerUI) setESILoginPending(pending bool) {
	for _, button := range []*widget.Button{ui.connect, ui.loginSubmit} {
		if pending {
			button.Disable()
		} else {
			button.Enable()
		}
	}
	for _, check := range ui.loginScopeChecks {
		if pending {
			check.Disable()
		} else {
			check.Enable()
		}
	}
	if pending {
		ui.cancelLogin.Show()
	} else {
		ui.cancelLogin.Hide()
	}
}

func (ui *analyzerUI) requestESILogin(action esiAction) {
	if ui.cancelESILogin != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	select {
	case ui.esiLogins <- esiLoginRequest{action: action, ctx: ctx, cancel: cancel}:
		ui.cancelESILogin = cancel
		ui.loginStatus.SetText("")
		ui.setESILoginPending(true)
		ui.setESIStatus("Opening EVE sign-in…")
	default:
		cancel()
		ui.setESIStatus("An EVE sign-in is already pending.")
	}
}

func (ui *analyzerUI) openESIBrowser(address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}
	fyne.DoAndWait(func() {
		err = fyne.CurrentApp().OpenURL(parsed)
		if err == nil {
			ui.setESIStatus("Complete EVE sign-in in your browser and grant the selected scopes.")
		}
	})
	return err
}

func (ui *analyzerUI) watchESILogins(ctx context.Context, source *esiService, results chan<- esiLoginResult, openBrowser func(string) error) {
	// Share only immutable endpoints and the concurrency-safe HTTP client.
	// Discovery caches, tokens and configuration belong to this worker alone.
	auth := newESIService("")
	auth.http, auth.baseURL, auth.discoveryURL = source.http, source.baseURL, source.discoveryURL
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-ui.esiLogins:
			stop := context.AfterFunc(ctx, request.cancel)
			auth.config.ClientID, auth.config.CallbackPort = request.action.ClientID, request.action.CallbackPort
			account, err := auth.authenticate(request.ctx, openBrowser, request.action.Scopes)
			request.cancel()
			stop()
			if ctx.Err() != nil {
				return
			}
			var result esiLoginResult
			if err == nil {
				result = esiLoginResult{action: request.action, account: account, taken: make(chan struct{})}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
			fyne.DoAndWait(func() {
				ui.cancelESILogin = nil
				ui.setESILoginPending(false)
				if err != nil {
					ui.setESIStatus("EVE sign-in: " + err.Error())
					reportRuntimeError("EVE sign-in", err)
					if !errors.Is(err, context.Canceled) {
						dialog.ShowError(err, ui.window)
					}
				} else {
					select {
					case <-result.taken:
						// Already adopted; leave its new refresh running.
					default:
						if ui.cancelESIRefresh != nil {
							ui.cancelESIRefresh()
						}
					}
				}
			})
		}
	}
}

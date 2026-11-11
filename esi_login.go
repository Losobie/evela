package main

import (
	"context"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
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

func (ui *analyzerUI) requestESILogin(action esiAction) {
	if ui.cancelESILogin != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	select {
	case ui.esiLogins <- esiLoginRequest{action: action, ctx: ctx, cancel: cancel}:
		ui.cancelESILogin = cancel
		ui.connect.Disable()
		ui.cancelLogin.Show()
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
			ui.setESIStatus("Complete EVE sign-in in your browser. Select a character and grant ship and location access.")
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
			account, err := auth.authenticate(request.ctx, openBrowser)
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
				ui.connect.Enable()
				ui.cancelLogin.Hide()
				if err != nil {
					ui.setESIStatus("EVE sign-in: " + err.Error())
					reportRuntimeError("EVE sign-in", err)
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

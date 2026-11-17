package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

func TestESISelectedScopesThroughSSO(t *testing.T) {
	for _, tc := range []struct {
		name               string
		requested, granted []string
		wantError          bool
	}{
		{"none", nil, []string{}, false},
		{"ship only", []string{shipScope}, []string{shipScope}, false},
		{"location only", []string{locationScope}, []string{locationScope}, false},
		{"structure only", []string{structureScope}, []string{structureScope}, false},
		{"all", []string{shipScope, locationScope, structureScope}, []string{shipScope, locationScope, structureScope}, false},
		{"missing selected grant", []string{locationScope}, []string{shipScope}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newESIFixture(t)
			f.token = signedTestToken(t, f.key, func(claims map[string]any) { claims["scp"] = tc.granted })
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			f.service.config.CallbackPort = listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			account, err := f.service.authenticate(ctx, func(address string) error {
				parsed, err := url.Parse(address)
				if err != nil {
					return err
				}
				query := parsed.Query()
				if query.Get("scope") != strings.Join(tc.requested, " ") {
					t.Error("authorization did not request exactly the selected scopes")
				}
				f.challenge = query.Get("code_challenge")
				response, err := http.Get(query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "code": {"test-code"}}.Encode())
				if err != nil {
					return err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("callback returned %d", response.StatusCode)
				}
				return nil
			}, tc.requested)
			if tc.wantError {
				if err == nil || account != nil || !strings.Contains(err.Error(), "did not grant requested scope") {
					t.Fatalf("missing selected permission accepted: %v", err)
				}
			} else if err != nil || strings.Join(account.Token.Scopes, " ") != strings.Join(tc.granted, " ") {
				t.Fatalf("selected scopes did not authenticate: %v", err)
			}
		})
	}
}

func TestESIScopeDialogSelectionAndCloseCancels(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Choose ESI scopes")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	ui.openESILogin()
	if len(ui.loginScopeChecks) != 3 || !ui.loginScopeChecks[0].Checked || !ui.loginScopeChecks[1].Checked || !ui.loginScopeChecks[2].Checked {
		t.Fatal("unexpected initial scope choices")
	}
	ui.loginScopeChecks[0].SetChecked(false)
	ui.loginScopeChecks[2].SetChecked(false)
	test.Tap(ui.loginSubmit)
	request := <-ui.esiLogins
	defer request.cancel()
	if len(request.action.Scopes) != 1 || request.action.Scopes[0] != locationScope {
		t.Fatalf("wrong scope choices: %v", request.action.Scopes)
	}
	ui.esiLoginDialog.Hide()
	if request.ctx.Err() != context.Canceled {
		t.Fatal("closing the dialog left authentication pending")
	}
}

func TestESINoScopesSurviveRestartWithoutPrivateQueries(t *testing.T) {
	f, account := locationFixture(t)
	account.Token.Scopes = []string{}
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if loaded.config.Accounts[123].Token.Scopes == nil {
		t.Fatal("empty grants became unknown legacy grants after restart")
	}
	f.service.config = loaded.config
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.shipRequests.Load() != 0 || f.locationRequests.Load() != 0 {
		t.Fatal("unchecked scopes still caused private ESI queries")
	}
	record := f.service.records()[0]
	if characterESIScopes(record.Scopes) != "None" || characterESIStatus(record) != "OK" {
		t.Fatalf("missing scopes should not be an error: %+v", record)
	}
}

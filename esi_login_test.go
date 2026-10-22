package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Fyne's test driver normally executes UI callbacks on the calling goroutine.
// Serialize them here to model its real event loop while testing two workers.
type esiLoginTestDriver struct {
	fyne.Driver
	mu sync.Mutex
}

func (driver *esiLoginTestDriver) DoFromGoroutine(callback func(), _ bool) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	callback()
}

type esiLoginTestApp struct {
	fyne.App
	driver fyne.Driver
}

func (app *esiLoginTestApp) Driver() fyne.Driver { return app.driver }

type esiLoginTestTransport struct {
	base    http.RoundTripper
	entered chan struct{}
}

func (transport esiLoginTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasSuffix(request.URL.Path, "/ship/") {
		transport.entered <- struct{}{}
		// A deliberately stalled ESI request is released only by cancellation.
		<-request.Context().Done()
		return nil, request.Context().Err()
	}
	return transport.base.RoundTrip(request)
}

func TestESILoginsOpenDuringBlockedRefreshAndPreserveAccounts(t *testing.T) {
	app := test.NewTempApp(t)
	fyne.SetCurrentApp(&esiLoginTestApp{App: app, driver: &esiLoginTestDriver{Driver: app.Driver()}})
	window := app.NewWindow("Independent sign-in")
	defer window.Close()
	ui := newAnalyzerUI(window)
	f := newESIFixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.service.config.CallbackPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	f.service.config.Accounts[456] = &esiAccount{
		Ship:  shipRecord{CharacterID: 456, CharacterName: "Existing Pilot", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()},
		Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{shipScope}},
	}
	f.service.rememberShip(f.service.config.Accounts[456].Ship, false)
	entered := make(chan struct{}, 8)
	f.service.http.Transport = esiLoginTestTransport{base: http.DefaultTransport, entered: entered}
	opened := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ui.watchESIService(f.service, func(address string) error { opened <- address; return nil })
	}()
	defer func() {
		close(ui.stop)
		select {
		case <-done:
			loaded := newESIService(f.service.file)
			if err := loaded.load(); err != nil {
				t.Error(err)
			} else if len(loaded.config.Accounts) != 2 || loaded.config.Accounts[123] == nil || loaded.config.Accounts[123].Token.RefreshToken != "rotated-refresh" || len(loaded.config.ShipHistory[456]) == 0 {
				t.Error("shutdown lost a saved sign-in, fresh tokens or existing ship history")
			}
		case <-time.After(5 * time.Second):
			t.Error("ESI workers did not stop after cancellation")
		}
	}()
	waitRefresh := func() {
		t.Helper()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("expected a blocked background refresh")
		}
	}
	waitRefresh()
	for attempt := 0; attempt < 2; attempt++ {
		// Wait for UI completion of the previous sign-in, as a user would.
		deadline := time.Now().Add(5 * time.Second)
		for {
			ready := false
			fyne.DoAndWait(func() { ready = !ui.connect.Disabled() })
			if ready {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("sign-in remained disabled during background refresh")
			}
			time.Sleep(time.Millisecond)
		}
		fyne.DoAndWait(func() { test.Tap(ui.connect) })
		var address string
		select {
		case address = <-opened:
		case <-time.After(3 * time.Second):
			t.Fatal("browser opening waited behind a stalled ESI refresh")
		}
		parsed, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		f.challenge = query.Get("code_challenge")
		response, err := http.Get(query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "code": {"test-code"}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal("sign-in callback failed")
		}
		waitRefresh()
		fyne.DoAndWait(func() {
			if len(ui.ships) != 2 || ui.ships[characterKey("Existing Pilot")].CharacterID != 456 || ui.ships[characterKey("Test Pilot")].CharacterID != 123 {
				t.Error("adopting a login lost a connected character")
			}
		})
	}
}

func TestESIIndependentLoginCancellationAndPortRelease(t *testing.T) {
	app := test.NewTempApp(t)
	fyne.SetCurrentApp(&esiLoginTestApp{App: app, driver: &esiLoginTestDriver{Driver: app.Driver()}})
	ui := newAnalyzerUI(app.NewWindow("Cancel sign-in"))
	defer ui.window.Close()
	f := newESIFixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan struct{}, 1)
	done := make(chan struct{})
	ui.clientID.SetText("test-client")
	ui.callbackPort.SetText(strconv.Itoa(port))
	ui.requestESIConfiguration("connect")
	go func() {
		defer close(done)
		ui.watchESILogins(ctx, f.service, make(chan esiLoginResult, 1), func(string) error { opened <- struct{}{}; return nil })
	}()
	select {
	case <-opened:
	case <-time.After(3 * time.Second):
		t.Fatal("sign-in did not open")
	}
	// Cancel while authorization is waiting for the browser callback.
	fyne.DoAndWait(func() { test.Tap(ui.cancelLogin) })
	deadline := time.Now().Add(3 * time.Second)
	for {
		ready := false
		fyne.DoAndWait(func() { ready = !ui.connect.Disabled() && !ui.cancelLogin.Visible() })
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled login did not restore the sign-in controls")
		}
		time.Sleep(time.Millisecond)
	}
	listener, err = net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("cancelled sign-in retained its callback port: %v", err)
	}
	listener.Close()
	// The next login can reuse that port; app shutdown cancels it as well.
	fyne.DoAndWait(func() { test.Tap(ui.connect) })
	select {
	case <-opened:
	case <-time.After(3 * time.Second):
		t.Fatal("could not sign in again after cancelling")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("sign-in ignored cancellation")
	}
	listener, err = net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("cancelled sign-in retained its callback port: %v", err)
	}
	listener.Close()
}

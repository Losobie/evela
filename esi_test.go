package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func signedTestToken(t *testing.T, key *rsa.PrivateKey, edit func(map[string]any)) string {
	t.Helper()
	claims := map[string]any{"sub": "CHARACTER:EVE:123", "name": "Test Pilot", "iss": "https://login.eveonline.com/", "aud": []string{"test-client", "EVE Online"}, "scp": []string{shipScope}, "exp": time.Now().Add(time.Hour).Unix()}
	if edit != nil {
		edit(claims)
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

type esiFixture struct {
	service                                   *esiService
	key                                       *rsa.PrivateKey
	ecKey                                     *ecdsa.PrivateKey
	token                                     string
	challenge                                 string
	shipRequests, typeRequests, tokenRequests atomic.Int32
	groupRequests, groupStatus                atomic.Int32
	shipStatus                                atomic.Int32
	onceUnauthorized                          atomic.Bool
	shipType                                  atomic.Int64
	shipItem                                  atomic.Int64
	typeStatus                                atomic.Int32
	server                                    *httptest.Server
}

func newESIFixture(t *testing.T) *esiFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &esiFixture{key: key}
	f.ecKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// EVE commonly emits a scalar scp when only one scope is requested.
	f.token = signedTestToken(t, key, func(claims map[string]any) { claims["scp"] = shipScope })
	f.shipType.Store(29990)
	f.shipItem.Store(1000000001)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/token" && strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Error("application credentials sent outside the token endpoint")
		}
		switch r.URL.Path {
		case "/metadata":
			json.NewEncoder(w).Encode(ssoMetadata{Authorization: f.server.URL + "/authorize", Token: f.server.URL + "/token", JWKS: f.server.URL + "/jwks"})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
				{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())},
				{"kty": "EC", "kid": "ec-key", "alg": "ES256", "use": "sig", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(f.ecKey.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(f.ecKey.Y.FillBytes(make([]byte, 32)))},
			}})
		case "/token":
			f.tokenRequests.Add(1)
			r.ParseForm()
			if r.Method != http.MethodPost {
				t.Error("incorrect token request method")
			}
			if f.service.config.AuthMode == esiAuthClientSecret {
				id, secret, ok := r.BasicAuth()
				if !ok || id != "test-client" || secret != "fake-client-secret" {
					t.Error("incorrect confidential client authentication")
				}
				if r.Form.Get("client_id") != "" || r.Form.Get("client_secret") != "" {
					t.Error("client credentials included in the token form")
				}
			} else if r.Form.Get("client_id") != "test-client" || r.Header.Get("Authorization") != "" {
				t.Error("incorrect native token request")
			}
			if r.Form.Get("grant_type") == "authorization_code" {
				hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if f.service.config.AuthMode == esiAuthPKCE && base64.RawURLEncoding.EncodeToString(hash[:]) != f.challenge {
					t.Error("PKCE verification failed")
				}
				if f.service.config.AuthMode == esiAuthClientSecret && r.Form.Get("code_verifier") != "" {
					t.Error("unexpected PKCE verifier for confidential flow")
				}
				if r.Form.Get("code") != "test-code" || r.Form.Get("redirect_uri") != esiCallbackURL(f.service.config.CallbackPort) {
					t.Error("code or redirect verification failed")
				}
			} else if r.Form.Get("refresh_token") != "original-refresh" {
				t.Error("wrong refresh token")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": f.token, "refresh_token": "rotated-refresh", "token_type": "Bearer", "expires_in": 3600})
		case "/latest/characters/123/ship/", "/latest/characters/456/ship/":
			f.shipRequests.Add(1)
			if r.Header.Get("Authorization") != "Bearer "+f.token {
				t.Error("missing ship access token")
			}
			if r.Header.Get("X-Compatibility-Date") == "" {
				t.Error("missing ESI compatibility date")
			}
			status := int(f.shipStatus.Load())
			if f.onceUnauthorized.Swap(false) {
				status = 401
			}
			if status != 0 {
				if status == 429 {
					w.Header().Set("Retry-After", "120")
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":"unavailable"}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ship_type_id": f.shipType.Load(), "ship_name": "My ship", "ship_item_id": f.shipItem.Load()})
		case "/latest/universe/types/29990/", "/latest/universe/types/29984/":
			f.typeRequests.Add(1)
			if status := f.typeStatus.Load(); status != 0 {
				w.WriteHeader(int(status))
				return
			}
			if r.Header.Get("Authorization") != "" {
				t.Error("private token sent to public type lookup")
			}
			name := "Loki"
			if strings.Contains(r.URL.Path, "29984") {
				name = "Tengu"
			}
			json.NewEncoder(w).Encode(map[string]any{"name": name, "group_id": 963})
		case "/latest/universe/groups/963/":
			f.groupRequests.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Error("private token sent to public class lookup")
			}
			if status := f.groupStatus.Load(); status != 0 {
				w.WriteHeader(int(status))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"name": "Strategic Cruiser"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.service = newESIService(filepath.Join(t.TempDir(), "esi.dat"))
	f.service.discoveryURL = f.server.URL + "/metadata"
	f.service.baseURL = f.server.URL + "/latest"
	f.service.config.ClientID = "test-client"
	return f
}

func TestESIClientSecretLoginRefreshAndStorage(t *testing.T) {
	f := newESIFixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if err := f.service.configureApplication("test-client", port, esiAuthClientSecret, ""); err == nil {
		t.Fatal("missing secret accepted")
	}
	if err := f.service.configureApplication("test-client", port, "unsupported", ""); err == nil {
		t.Fatal("unknown authentication mode accepted")
	}
	if err := f.service.configureApplication("test-client", port, esiAuthClientSecret, "fake-client-secret"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	account, err := f.service.authorize(ctx, func(address string) error {
		parsed, err := url.Parse(address)
		if err != nil {
			return err
		}
		query := parsed.Query()
		if query.Get("code_challenge") != "" || query.Get("code_challenge_method") != "" || strings.Contains(address, "fake-client-secret") {
			t.Error("confidential authorization URL contained unexpected credentials or PKCE parameters")
		}
		response, err := http.Get(query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "code": {"test-code"}}.Encode())
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("callback returned %d", response.StatusCode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	account.Token.ExpiresAt = time.Now().Add(-time.Second)
	account.Token.RefreshToken = "original-refresh"
	if err := f.service.refreshShips(ctx); err != nil {
		t.Fatal(err)
	}
	if account.Token.RefreshToken != "rotated-refresh" || account.Ship.TypeName != "Loki" || f.tokenRequests.Load() != 2 {
		t.Fatal("confidential flow failed login or token renewal")
	}
	if _, err := f.service.importTokens(ctx, "", "original-refresh"); err != nil {
		t.Fatal(err)
	}
	if f.tokenRequests.Load() != 3 {
		t.Fatal("refresh-token import did not use confidential authentication")
	}
	data, err := os.ReadFile(f.service.file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fake-client-secret") {
		t.Fatal("application secret stored in plaintext")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if loaded.config.AuthMode != esiAuthClientSecret || loaded.config.ClientSecret != "fake-client-secret" || loaded.config.CallbackPort != port {
		t.Fatal("application authentication settings did not round-trip")
	}
	if err := f.service.configureApplication("test-client", port, esiAuthPKCE, "unused-secret"); err != nil {
		t.Fatal(err)
	}
	if f.service.config.ClientSecret != "" {
		t.Fatal("PKCE configuration retained unused secret")
	}
}

func TestESILoginPKCEAndEncryptedStorage(t *testing.T) {
	f := newESIFixture(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.service.config.CallbackPort = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	account, err := f.service.authorize(ctx, func(address string) error {
		parsed, err := url.Parse(address)
		if err != nil {
			return err
		}
		query := parsed.Query()
		f.challenge = query.Get("code_challenge")
		if query.Get("scope") != shipScope || query.Get("code_challenge_method") != "S256" || query.Get("client_id") != "test-client" {
			t.Error("incorrect authorization parameters")
		}
		if query.Get("redirect_uri") != esiCallbackURL(f.service.config.CallbackPort) {
			t.Error("configured callback port was not used")
		}
		response, err := http.Get(query.Get("redirect_uri") + "?" + url.Values{"state": {query.Get("state")}, "code": {"test-code"}}.Encode())
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("callback returned %d", response.StatusCode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if account.Ship.CharacterID != 123 || account.Ship.CharacterName != "Test Pilot" {
		t.Fatal("wrong authenticated character")
	}
	data, err := os.ReadFile(f.service.file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rotated-refresh") || strings.Contains(string(data), f.token) {
		t.Fatal("credentials stored in plaintext")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if loaded.config.Accounts[123].Token.RefreshToken != "rotated-refresh" || loaded.config.ClientID != "test-client" || loaded.config.CallbackPort != f.service.config.CallbackPort {
		t.Fatal("saved account did not round-trip")
	}
}

func TestESICallbackConfiguration(t *testing.T) {
	service := newESIService(filepath.Join(t.TempDir(), "esi.dat"))
	if service.config.CallbackPort != 8080 || esiCallbackURL(service.config.CallbackPort) != "http://localhost:8080/callback" {
		t.Fatal("incorrect default localhost callback")
	}
	for _, value := range []string{"", "0", "-1", "65536", "8080/path", "example.com", "1.2"} {
		if _, err := parseCallbackPort(value); err == nil {
			t.Fatalf("accepted invalid port %q", value)
		}
	}
	for _, value := range []string{"1", "8080", "65535", " 9000 "} {
		if _, err := parseCallbackPort(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.configure("test-client", 9123); err != nil {
		t.Fatal(err)
	}
	loaded := newESIService(service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if loaded.config.CallbackPort != 9123 || loaded.config.ClientID != "test-client" {
		t.Fatal("configuration not persisted")
	}
	if err := service.configure("test-client", 0); err == nil || service.config.CallbackPort != 9123 {
		t.Fatal("invalid port changed configuration")
	}
	// Existing encrypted settings have no CallbackPort field. Zero migrates to 8080.
	service.config.CallbackPort = 0
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if loaded.config.CallbackPort != 8080 {
		t.Fatal("legacy configuration did not adopt default port")
	}
}

func TestESIImportsAndReplacesTokens(t *testing.T) {
	f := newESIFixture(t)
	account, err := f.service.importTokens(context.Background(), f.token, "")
	if err != nil {
		t.Fatal(err)
	}
	if account.Ship.CharacterID != 123 || account.Token.RefreshToken != "" {
		t.Fatal("access-only token import failed")
	}
	if err := f.service.configure("another-client", 8080); err == nil {
		t.Fatal("client ID changed while tokens remained connected")
	}
	if err := f.service.configure("test-client", 9000); err != nil {
		t.Fatal(err)
	}
	account, err = f.service.importTokens(context.Background(), "", "original-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if account.Token.RefreshToken != "rotated-refresh" || len(f.service.config.Accounts) != 1 {
		t.Fatal("refresh-only import did not replace the existing account")
	}
	account, err = f.service.importTokens(context.Background(), f.token, "original-refresh")
	if err != nil || account.Token.RefreshToken != "rotated-refresh" {
		t.Fatalf("token pair import failed: %v", err)
	}
	before := f.service.config.Accounts[123]
	if _, err := f.service.importTokens(context.Background(), "invalid-token", ""); err == nil {
		t.Fatal("invalid token imported")
	}
	if f.service.config.Accounts[123] != before {
		t.Fatal("failed token import replaced existing credentials")
	}
	if _, err := f.service.importTokens(context.Background(), "", ""); err == nil {
		t.Fatal("empty tokens imported")
	}
	f.token = signedTestToken(t, f.key, func(c map[string]any) { c["sub"] = "CHARACTER:EVE:999" })
	if _, err := f.service.importTokens(context.Background(), before.Token.AccessToken, "original-refresh"); err == nil {
		t.Fatal("mismatched token pair imported")
	}
	if len(f.service.config.Accounts) != 1 {
		t.Fatal("mismatched import created another account")
	}
}

func TestESIAccessOnlyTokenWorksUntilExpiry(t *testing.T) {
	f := newESIFixture(t)
	f.token = signedTestToken(t, f.key, func(c map[string]any) { c["exp"] = time.Now().Add(30 * time.Second).Unix() })
	account, err := f.service.importTokens(context.Background(), f.token, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.refreshShip(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if account.Ship.TypeName != "Loki" || f.tokenRequests.Load() != 0 {
		t.Fatal("valid access-only token tried to renew early")
	}
	account.Token.ExpiresAt = time.Now().Add(-time.Second)
	if err := f.service.refreshShip(context.Background(), account); err == nil {
		t.Fatal("expired access-only token stayed usable")
	}
}

func TestESICallbackRejectsInvalidStateAndReplay(t *testing.T) {
	result := make(chan loginResult, 1)
	handler := loginCallback("expected-state", result)
	for _, path := range []string{"/callback?code=x&state=bad", "/callback?state=expected-state"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 400 {
			t.Fatalf("invalid callback accepted: %s", path)
		}
	}
	if len(result) != 0 {
		t.Fatal("invalid callbacks consumed the login")
	}
	path := "/callback?code=valid&state=expected-state"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	if (<-result).code != "valid" {
		t.Fatal("valid authorization code lost")
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	if recorder.Code != 409 {
		t.Fatal("callback replay accepted")
	}
}

func TestESITokenValidation(t *testing.T) {
	f := newESIFixture(t)
	for name, edit := range map[string]func(map[string]any){
		"expired":  func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"issuer":   func(c map[string]any) { c["iss"] = "https://evil.example" },
		"audience": func(c map[string]any) { c["aud"] = []string{"other-client", "EVE Online"} },
		"scope":    func(c map[string]any) { c["scp"] = []string{"other-scope"} },
		"identity": func(c map[string]any) { c["sub"] = "CHARACTER:EVE:0" },
		"future":   func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() },
	} {
		t.Run(name, func(t *testing.T) {
			token := signedTestToken(t, f.key, edit)
			if _, err := f.service.validateToken(context.Background(), token); err == nil {
				t.Fatal("invalid claims accepted")
			}
		})
	}
	parts := strings.Split(f.token, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"CHARACTER:EVE:999"}`))
	if _, err := f.service.validateToken(context.Background(), strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := f.service.validateToken(context.Background(), f.token); err != nil {
		t.Fatal(err)
	}
}

func TestESISingleScopeStringToken(t *testing.T) {
	f := newESIFixture(t)
	token := signedTestToken(t, f.key, func(claims map[string]any) { claims["scp"] = shipScope })
	identity, err := f.service.validateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("valid signed single-scope token rejected: %v", err)
	}
	if identity.ID != 123 || identity.Name != "Test Pilot" {
		t.Fatal("wrong character from single-scope token")
	}
}

func TestESIAdditionalGrantedScopes(t *testing.T) {
	for _, mode := range []string{esiAuthPKCE, esiAuthClientSecret} {
		t.Run(mode, func(t *testing.T) {
			f := newESIFixture(t)
			f.service.config.AuthMode = mode
			if mode == esiAuthClientSecret {
				f.service.config.ClientSecret = "fake-client-secret"
			}
			for _, testCase := range []struct {
				name    string
				scopes  []string
				allowed bool
			}{
				{"exact application scopes", []string{"esi-location.read_location.v1", "esi-location.read_ship_type.v1", "esi-location.read_online.v1"}, true},
				{"required scope last", []string{"esi-location.read_online.v1", "esi-location.read_location.v1", shipScope}, true},
				{"minimum only", []string{shipScope}, true},
				{"additional unknown scope", []string{shipScope, "future.optional.scope.v99"}, true},
				{"duplicate scope", []string{shipScope, "esi-location.read_online.v1", shipScope}, true},
				{"optional scopes only", []string{"esi-location.read_location.v1", "esi-location.read_online.v1"}, false},
				{"similar name is insufficient", []string{"esi-location.read_location.v1", shipScope + ".extra", "esi-location.read_online.v1"}, false},
				{"no granted scopes", []string{}, false},
			} {
				t.Run(testCase.name, func(t *testing.T) {
					f.token = signedTestToken(t, f.key, func(claims map[string]any) { claims["scp"] = testCase.scopes })
					// Login, refresh and refresh-token import all use this exchange
					// path, including application authentication in both modes.
					token, identity, err := f.service.exchange(context.Background(), url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"original-refresh"}, "client_id": {"test-client"}})
					if testCase.allowed {
						if err != nil {
							t.Fatalf("additional scopes broke authentication: %v", err)
						}
						if identity.ID != 123 || token.AccessToken != f.token {
							t.Fatal("wrong character or token after exchange")
						}
					} else if err == nil || !strings.Contains(err.Error(), "missing required scope "+shipScope) {
						t.Fatalf("missing minimum scope should be rejected, got %v", err)
					}
				})
			}
		})
	}
}

func TestESIScopeFormatsAndValidationDiagnostics(t *testing.T) {
	f := newESIFixture(t)
	for _, testCase := range []struct {
		name      string
		edit      func(map[string]any)
		wantError string
	}{
		{"single scope string", func(c map[string]any) { c["scp"] = shipScope }, ""},
		{"one scope array", func(c map[string]any) { c["scp"] = []string{shipScope} }, ""},
		{"multiple scopes", func(c map[string]any) { c["scp"] = []string{"esi-location.read_location.v1", shipScope} }, ""},
		{"missing scope", func(c map[string]any) { delete(c, "scp") }, "missing required scope"},
		{"wrong scope", func(c map[string]any) { c["scp"] = shipScope + ".extra" }, "missing required scope"},
		{"numeric scope", func(c map[string]any) { c["scp"] = 123 }, "unsupported JWT claim format"},
		{"mixed scope array", func(c map[string]any) { c["scp"] = []any{shipScope, 123} }, "unsupported JWT claim format"},
		{"null scope array item", func(c map[string]any) { c["scp"] = []any{shipScope, nil} }, "unsupported JWT claim format"},
		{"wrong application", func(c map[string]any) { c["aud"] = []string{"other-application", "EVE Online"} }, "different application"},
		{"scalar audience", func(c map[string]any) { c["aud"] = "EVE Online" }, "different application"},
		{"missing EVE audience", func(c map[string]any) { c["aud"] = []string{"test-client"} }, "audience does not include EVE Online"},
		{"wrong issuer", func(c map[string]any) { c["iss"] = "https://evil.example" }, "issuer is not EVE SSO"},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, "token has expired"},
		{"missing expiry", func(c map[string]any) { delete(c, "exp") }, "missing or invalid token expiry"},
		{"not valid yet", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() }, "token is not valid yet"},
		{"wrong identity", func(c map[string]any) { c["sub"] = "CORPORATION:EVE:123" }, "invalid character identity"},
		{"wrong character ID", func(c map[string]any) { c["sub"] = "CHARACTER:EVE:0" }, "invalid character ID"},
		{"missing name", func(c map[string]any) { delete(c, "name") }, "missing character name"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			token := signedTestToken(t, f.key, func(c map[string]any) { c["private-marker"] = "DO_NOT_INCLUDE_CLAIMS_IN_ERRORS"; testCase.edit(c) })
			_, err := f.service.validateToken(context.Background(), token)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("wanted reason %q, got %v", testCase.wantError, err)
			}
			if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "DO_NOT_INCLUDE_CLAIMS_IN_ERRORS") {
				t.Fatal("validation error exposed token contents")
			}
		})
	}
	parts := strings.Split(f.token, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"scp":"esi-location.read_ship_type.v1"}`))
	if _, err := f.service.validateToken(context.Background(), strings.Join(parts, ".")); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("tampered token did not report signature failure: %v", err)
	}
}

func TestESIECDSASignatureValidation(t *testing.T) {
	f := newESIFixture(t)
	parts := strings.Split(f.token, ".")
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": "ec-key"})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + parts[1]
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, f.ecKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	signature := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	if _, err = f.service.validateToken(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	signature[0] ^= 1
	if _, err = f.service.validateToken(context.Background(), unsigned+"."+base64.RawURLEncoding.EncodeToString(signature)); err == nil {
		t.Fatal("tampered ECDSA signature accepted")
	}
}

func TestESIRefreshRotationShipChangesAndRateLimit(t *testing.T) {
	f := newESIFixture(t)
	account := &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{RefreshToken: "original-refresh"}}
	f.service.config.Accounts[123] = account
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if account.Token.RefreshToken != "rotated-refresh" || account.Ship.TypeName != "Loki" || !account.Ship.current() {
		t.Fatalf("refresh failed: %+v", account.Ship)
	}
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.typeRequests.Load() != 1 || f.tokenRequests.Load() != 1 {
		t.Fatal("ship type cache or token expiry cache missed")
	}
	f.shipType.Store(29984)
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if account.Ship.TypeName != "Tengu" {
		t.Fatal("ship change was not detected")
	}
	f.shipStatus.Store(429)
	if err := f.service.refreshShips(context.Background()); err == nil || account.Ship.current() {
		t.Fatal("failed lookup stayed current")
	}
	before := f.shipRequests.Load()
	if err := f.service.refreshShips(context.Background()); err == nil {
		t.Fatal("rate limit not reported")
	}
	if f.shipRequests.Load() != before || time.Until(f.service.backoffUntil) < time.Minute {
		t.Fatal("rate limit not respected")
	}
}

func TestESIRetriesUnauthorizedOnce(t *testing.T) {
	f := newESIFixture(t)
	f.onceUnauthorized.Store(true)
	account := &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, RefreshToken: "original-refresh", ExpiresAt: time.Now().Add(time.Hour)}}
	f.service.config.Accounts[123] = account
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.tokenRequests.Load() != 1 || f.shipRequests.Load() != 2 {
		t.Fatal("401 should trigger one token renewal and one retry")
	}
}

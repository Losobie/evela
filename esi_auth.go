package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type ssoMetadata struct {
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	JWKS          string `json:"jwks_uri"`
}

func randomURLValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (service *esiService) discover(ctx context.Context) (*ssoMetadata, error) {
	if service.metadata != nil && time.Since(service.metadataAt) < time.Hour {
		return service.metadata, nil
	}
	var metadata ssoMetadata
	if err := service.requestJSON(ctx, http.MethodGet, service.discoveryURL, "", nil, &metadata); err != nil {
		return nil, err
	}
	origin, err := url.Parse(service.discoveryURL)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range []string{metadata.Authorization, metadata.Token, metadata.JWKS} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host != origin.Host || parsed.Scheme != origin.Scheme || parsed.User != nil {
			return nil, fmt.Errorf("SSO discovery returned an unexpected endpoint")
		}
	}
	service.metadata, service.metadataAt = &metadata, time.Now()
	return &metadata, nil
}

type loginResult struct {
	code string
	err  error
}

func loginCallback(state string, result chan<- loginResult) http.Handler {
	var accepted atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		query := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Login state did not match. Return to Evela and try again.", http.StatusBadRequest)
			return
		}
		if query.Get("code") == "" && query.Get("error") == "" {
			http.Error(w, "Missing authorization code.", http.StatusBadRequest)
			return
		}
		if accepted.Swap(true) {
			http.Error(w, "This login has already been handled.", http.StatusConflict)
			return
		}
		answer := loginResult{code: query.Get("code")}
		if query.Get("error") != "" {
			answer.err = fmt.Errorf("EVE sign-in was cancelled or denied")
		}
		fmt.Fprintln(w, "You can close this tab and return to Evela.")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		result <- answer
	})
}

func (service *esiService) authorize(ctx context.Context, openBrowser func(string) error) (*esiAccount, error) {
	account, err := service.authenticate(ctx, openBrowser)
	if err != nil {
		return nil, err
	}
	return account, service.acceptLogin(service.config.ClientID, service.config.CallbackPort, account)
}

// Authentication has private configuration and never writes accounts or history.
// The ESI worker adopts the validated result and remains the only storage writer.
func (service *esiService) authenticate(ctx context.Context, openBrowser func(string) error) (*esiAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(service.config.ClientID) == "" {
		return nil, fmt.Errorf("enter your EVE application's public client ID first")
	}
	port, err := parseCallbackPort(strconv.Itoa(service.config.CallbackPort))
	if err != nil {
		return nil, err
	}
	callbackURL := esiCallbackURL(port)
	metadata, err := service.discover(ctx)
	if err != nil {
		return nil, err
	}
	verifier, err := randomURLValue()
	if err != nil {
		return nil, err
	}
	state, err := randomURLValue()
	if err != nil {
		return nil, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("start EVE login callback on localhost port %d: %w", port, err)
	}
	result := make(chan loginResult, 1)
	server := &http.Server{Handler: loginCallback(state, result), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	query := url.Values{"response_type": {"code"}, "client_id": {service.config.ClientID}, "redirect_uri": {callbackURL}, "scope": {shipScope + " " + locationScope}, "state": {state}}
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err = openBrowser(metadata.Authorization + "?" + query.Encode()); err != nil {
		return nil, fmt.Errorf("open EVE sign-in browser: %w", err)
	}
	var answer loginResult
	select {
	case answer = <-result:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if answer.err != nil {
		return nil, answer.err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {answer.code}, "client_id": {service.config.ClientID}, "redirect_uri": {callbackURL}}
	form.Set("code_verifier", verifier)
	token, identity, err := service.exchange(ctx, form)
	if err != nil {
		return nil, err
	}
	account := &esiAccount{Ship: shipRecord{CharacterID: identity.ID, CharacterName: identity.Name}, Token: token}
	return account, nil
}

func (service *esiService) acceptLogin(clientID string, port int, account *esiAccount) error {
	clientID = strings.TrimSpace(clientID)
	if err := service.validateConfiguration(clientID, port); err != nil {
		return err
	}
	service.config.ClientID, service.config.CallbackPort = clientID, port
	if previous := service.config.Accounts[account.Ship.CharacterID]; previous != nil {
		name := account.Ship.CharacterName
		account.Ship, account.Location = previous.Ship, previous.Location
		account.Ship.CharacterName = name
	}
	service.config.Accounts[account.Ship.CharacterID] = account
	if err := service.save(); err != nil {
		return fmt.Errorf("connected for this session, but could not save sign-in: %w", err)
	}
	return nil
}

type ssoIdentity struct {
	ID     int64
	Name   string
	Expiry time.Time
	Scopes []string
}

func (service *esiService) exchange(ctx context.Context, form url.Values) (esiToken, ssoIdentity, error) {
	metadata, err := service.discover(ctx)
	if err != nil {
		return esiToken{}, ssoIdentity{}, err
	}
	var response struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	if err = service.requestJSON(ctx, http.MethodPost, metadata.Token, "", form, &response); err != nil {
		return esiToken{}, ssoIdentity{}, fmt.Errorf("EVE token request failed; check the application authentication settings or reconnect the character: %w", err)
	}
	if !strings.EqualFold(response.TokenType, "Bearer") {
		return esiToken{}, ssoIdentity{}, fmt.Errorf("SSO returned an unsupported token type")
	}
	identity, err := service.validateToken(ctx, response.AccessToken)
	if err != nil {
		return esiToken{}, ssoIdentity{}, err
	}
	return esiToken{AccessToken: response.AccessToken, RefreshToken: response.RefreshToken, ExpiresAt: identity.Expiry, Scopes: identity.Scopes}, identity, nil
}

func (service *esiService) renew(ctx context.Context, account *esiAccount) error {
	if account.Token.RefreshToken == "" {
		return fmt.Errorf("sign in to this character again")
	}
	token, identity, err := service.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {account.Token.RefreshToken}, "client_id": {service.config.ClientID}})
	if err != nil {
		return err
	}
	if identity.ID != account.Ship.CharacterID {
		return fmt.Errorf("SSO returned a different character; reconnect this character")
	}
	if token.RefreshToken == "" {
		token.RefreshToken = account.Token.RefreshToken
	}
	account.Token = token
	account.Ship.CharacterName = identity.Name
	// Persist rotated refresh tokens immediately, before any subsequent request.
	if err = service.save(); err != nil {
		return fmt.Errorf("save renewed EVE sign-in: %w", err)
	}
	return nil
}

// EVE encodes a single scp value as a JSON string and multiple values as an
// array. JWT audiences also allow either shape. Normalize the representation
// without weakening the signature, audience or exact scope checks below.
type jwtStringValues []string

func (values *jwtStringValues) UnmarshalJSON(data []byte) error {
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	switch value := decoded.(type) {
	case nil:
		*values = nil
	case string:
		*values = []string{value}
	case []any:
		list := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return fmt.Errorf("expected a string or array of strings")
			}
			list = append(list, text)
		}
		*values = list
	default:
		return fmt.Errorf("expected a string or array of strings")
	}
	return nil
}

func (service *esiService) validateToken(ctx context.Context, token string) (ssoIdentity, error) {
	fail := func(reason string) (ssoIdentity, error) {
		// Reasons are fixed text: never include the token, claims or credentials.
		return ssoIdentity{}, fmt.Errorf("EVE access token validation failed: %s", reason)
	}
	if strings.TrimSpace(service.config.ClientID) == "" {
		return fail("no application Client ID is configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fail("invalid JWT format")
	}
	decode := base64.RawURLEncoding.DecodeString
	headerBytes, err := decode(parts[0])
	if err != nil {
		return fail("invalid JWT header encoding")
	}
	var header struct{ Alg, Kid string }
	if json.Unmarshal(headerBytes, &header) != nil || (header.Alg != "RS256" && header.Alg != "ES256") || header.Kid == "" {
		return fail("unsupported signing algorithm or missing signing key ID")
	}
	metadata, err := service.discover(ctx)
	if err != nil {
		return ssoIdentity{}, err
	}
	var keys struct {
		Keys []struct{ Kty, Kid, Alg, Use, N, E, Crv, X, Y string }
	}
	if err = service.requestJSON(ctx, http.MethodGet, metadata.JWKS, "", nil, &keys); err != nil {
		return ssoIdentity{}, fmt.Errorf("fetch EVE SSO signing keys: %w", err)
	}
	var key *rsa.PublicKey
	var ecKey *ecdsa.PublicKey
	for _, candidate := range keys.Keys {
		if candidate.Kid != header.Kid || (candidate.Alg != "" && candidate.Alg != header.Alg) || (candidate.Use != "" && candidate.Use != "sig") {
			continue
		}
		if header.Alg == "ES256" && candidate.Kty == "EC" && candidate.Crv == "P-256" {
			x, xErr := decode(candidate.X)
			y, yErr := decode(candidate.Y)
			if xErr != nil || yErr != nil || len(x) != 32 || len(y) != 32 {
				continue
			}
			ecKey = &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
			if !ecKey.Curve.IsOnCurve(ecKey.X, ecKey.Y) {
				ecKey = nil
			}
			break
		}
		if header.Alg != "RS256" || candidate.Kty != "RSA" {
			continue
		}
		n, nErr := decode(candidate.N)
		e, eErr := decode(candidate.E)
		if nErr != nil || eErr != nil || len(e) > 4 {
			continue
		}
		exponent := 0
		for _, value := range e {
			exponent = exponent*256 + int(value)
		}
		if exponent < 3 || exponent > 2147483647 {
			continue
		}
		key = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
		if key.N.BitLen() < 2048 {
			key = nil
		}
		break
	}
	if key == nil && ecKey == nil {
		return fail("no matching valid signing key in EVE SSO's key set")
	}
	signature, err := decode(parts[2])
	if err != nil {
		return fail("invalid JWT signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if key != nil {
		if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
			return fail("signature verification failed")
		}
	} else if len(signature) != 64 || !ecdsa.Verify(ecKey, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return fail("signature verification failed")
	}
	payload, err := decode(parts[1])
	if err != nil {
		return fail("invalid JWT payload encoding")
	}
	var claims struct {
		Sub, Name, Iss string
		Aud            jwtStringValues
		Scp            jwtStringValues
		Exp, Nbf       int64
	}
	if json.Unmarshal(payload, &claims) != nil {
		return fail("unsupported JWT claim format; audience and scopes must be strings or arrays of strings")
	}
	issuer := strings.TrimSuffix(strings.TrimPrefix(claims.Iss, "https://"), "/")
	if issuer != "login.eveonline.com" {
		return fail("token issuer is not EVE SSO")
	}
	if claims.Exp <= 0 {
		return fail("missing or invalid token expiry")
	}
	if claims.Exp <= time.Now().Unix() {
		return fail("token has expired; check your computer's clock and sign in again")
	}
	if claims.Nbf > time.Now().Unix()+30 {
		return fail("token is not valid yet; check your computer's clock")
	}
	has := func(values []string, want string) bool {
		for _, value := range values {
			if value == want {
				return true
			}
		}
		return false
	}
	if !has(claims.Aud, "EVE Online") {
		return fail("token audience does not include EVE Online")
	}
	if !has(claims.Aud, service.config.ClientID) {
		return fail("token was issued for a different application; check the configured Client ID")
	}
	// Required scopes are a subset of the granted scopes. Additional grants,
	// their ordering and duplicates must not affect authentication.
	if !has(claims.Scp, shipScope) {
		return fail("missing required scope " + shipScope + "; enable it in the EVE application and authorize the character again")
	}
	if !strings.HasPrefix(claims.Sub, "CHARACTER:EVE:") {
		return fail("invalid character identity")
	}
	if strings.TrimSpace(claims.Name) == "" {
		return fail("missing character name")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(claims.Sub, "CHARACTER:EVE:"), 10, 64)
	if err != nil || id <= 0 {
		return fail("invalid character ID")
	}
	return ssoIdentity{ID: id, Name: claims.Name, Expiry: time.Unix(claims.Exp, 0), Scopes: append([]string{}, claims.Scp...)}, nil
}

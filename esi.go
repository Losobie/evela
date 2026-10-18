package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const shipScope = "esi-location.read_ship_type.v1"
const defaultESICallbackPort = 8080
const esiAuthPKCE = "pkce"
const esiAuthClientSecret = "client_secret"

func validateESIAuth(mode, secret string) error {
	if mode != esiAuthPKCE && mode != esiAuthClientSecret {
		return fmt.Errorf("select a supported EVE authentication method")
	}
	if mode == esiAuthClientSecret && strings.TrimSpace(secret) == "" {
		return fmt.Errorf("enter the application's client secret for Client ID + secret authentication")
	}
	return nil
}

func esiCallbackURL(port int) string { return fmt.Sprintf("http://localhost:%d/callback", port) }

func parseCallbackPort(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("callback port must be a number from 1 to 65535")
	}
	return port, nil
}

type shipRecord struct {
	CharacterID        int64
	CharacterName      string
	ShipID             int64
	TypeID             int64
	TypeName, ShipName string
	GroupID            int64
	ClassName          string
	ClassError         string `json:"-"`
	CheckedAt          time.Time
	Error              string
	TokenExpiresAt     time.Time `json:"-"`
	CanRefresh         bool      `json:"-"`
}

func characterKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func (ship shipRecord) current() bool {
	return ship.TypeID > 0 && ship.TypeName != "" && ship.Error == "" && time.Since(ship.CheckedAt) < 90*time.Second
}

func shipLabel(name string, ships map[string]shipRecord) string {
	ship, ok := ships[characterKey(name)]
	if !ok {
		return "Not connected"
	}
	if ship.current() {
		return ship.TypeName
	}
	if ship.TypeName != "" {
		return ship.TypeName + " (stale)"
	}
	return "Ship unavailable"
}

type esiToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type esiAccount struct {
	Ship  shipRecord
	Token esiToken
}
type esiConfig struct {
	ClientID     string
	AuthMode     string
	ClientSecret string
	CallbackPort int
	Accounts     map[int64]*esiAccount
	ShipHistory  shipHistory `json:",omitempty"`
}

// The ESI worker owns this service. UI snapshots contain ship data only.
type esiService struct {
	config                esiConfig
	file                  string
	http                  *http.Client
	baseURL, discoveryURL string
	metadata              *ssoMetadata
	metadataAt            time.Time
	shipTypes             map[int64]shipTypeInfo
	groupNames            map[int64]string
	backoffUntil          time.Time
	now                   func() time.Time
}

func newESIService(file string) *esiService {
	return &esiService{file: file, config: esiConfig{AuthMode: esiAuthPKCE, CallbackPort: defaultESICallbackPort, Accounts: map[int64]*esiAccount{}}, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, baseURL: "https://esi.evetech.net/latest", discoveryURL: "https://login.eveonline.com/.well-known/oauth-authorization-server", shipTypes: map[int64]shipTypeInfo{}, groupNames: map[int64]string{}, now: time.Now}
}

type shipTypeInfo struct {
	Name    string `json:"name"`
	GroupID int64  `json:"group_id"`
}

func (service *esiService) load() error {
	data, err := os.ReadFile(service.file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err = unsealESI(data)
	if err != nil {
		return fmt.Errorf("unlock saved ESI connections: %w", err)
	}
	var config esiConfig
	if err = json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("read ESI settings: %w", err)
	}
	if config.CallbackPort == 0 {
		config.CallbackPort = defaultESICallbackPort
	}
	if config.AuthMode == "" {
		config.AuthMode = esiAuthPKCE
	}
	if err := validateESIAuth(config.AuthMode, config.ClientSecret); err != nil {
		return fmt.Errorf("read ESI settings: %w", err)
	}
	if _, err := parseCallbackPort(strconv.Itoa(config.CallbackPort)); err != nil {
		return fmt.Errorf("read ESI settings: %w", err)
	}
	service.config = config
	if service.config.Accounts == nil {
		service.config.Accounts = map[int64]*esiAccount{}
	}
	for id, account := range service.config.Accounts {
		if account == nil || account.Ship.CharacterID != id || id <= 0 {
			delete(service.config.Accounts, id)
			continue
		}
		// Older settings contain one last successful sighting. Preserve its actual
		// timestamp as a single observation, never as a claim about earlier logs.
		if len(service.config.ShipHistory[id]) == 0 {
			ship := account.Ship
			ship.Error = ""
			service.rememberShip(ship, false)
		}
		account.Ship.Error = "Waiting for ship refresh"
	}
	return nil
}

func (service *esiService) save() error {
	data, err := json.Marshal(service.config)
	if err != nil {
		return err
	}
	data, err = sealESI(data)
	if err != nil {
		return fmt.Errorf("encrypt ESI connections: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(service.file), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(service.file), ".esi-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), service.file)
}

func (service *esiService) records() []shipRecord {
	var records []shipRecord
	for _, account := range service.config.Accounts {
		record := account.Ship
		record.TokenExpiresAt, record.CanRefresh = account.Token.ExpiresAt, account.Token.RefreshToken != ""
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CharacterName < records[j].CharacterName })
	return records
}

func (service *esiService) configure(clientID string, port int) error {
	return service.configureApplication(clientID, port, service.config.AuthMode, service.config.ClientSecret)
}

func (service *esiService) configureApplication(clientID string, port int, mode, secret string) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("enter your EVE application's public Client ID")
	}
	if _, err := parseCallbackPort(strconv.Itoa(port)); err != nil {
		return err
	}
	secret = strings.TrimSpace(secret)
	if err := validateESIAuth(mode, secret); err != nil {
		return err
	}
	if mode == esiAuthPKCE {
		secret = ""
	}
	if len(service.config.Accounts) > 0 && service.config.ClientID != clientID {
		return fmt.Errorf("disconnect existing characters before changing the Client ID")
	}
	previous := service.config
	service.config.ClientID, service.config.CallbackPort = clientID, port
	service.config.AuthMode, service.config.ClientSecret = mode, secret
	if err := service.save(); err != nil {
		service.config = previous
		return fmt.Errorf("save ESI settings: %w", err)
	}
	return nil
}

func (service *esiService) importTokens(ctx context.Context, access, refresh string) (*esiAccount, error) {
	if service.config.ClientID == "" {
		return nil, fmt.Errorf("configure the EVE application Client ID first")
	}
	access, refresh = strings.TrimSpace(access), strings.TrimSpace(refresh)
	if access == "" && refresh == "" {
		return nil, fmt.Errorf("enter an access token or a refresh token")
	}
	var token esiToken
	var identity ssoIdentity
	var err error
	if access == "" {
		token, identity, err = service.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {service.config.ClientID}})
		if token.RefreshToken == "" {
			token.RefreshToken = refresh
		}
	} else {
		identity, err = service.validateToken(ctx, access)
		token = esiToken{AccessToken: access, RefreshToken: refresh, ExpiresAt: identity.Expiry}
		if err == nil && refresh != "" {
			var renewedIdentity ssoIdentity
			token, renewedIdentity, err = service.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {service.config.ClientID}})
			if err == nil && renewedIdentity.ID != identity.ID {
				return nil, fmt.Errorf("access and refresh tokens belong to different characters")
			}
			if token.RefreshToken == "" {
				token.RefreshToken = refresh
			}
		}
	}
	if err != nil {
		return nil, err
	}
	account := &esiAccount{Ship: shipRecord{CharacterID: identity.ID, CharacterName: identity.Name}, Token: token}
	previous := service.config.Accounts[identity.ID]
	service.config.Accounts[identity.ID] = account
	if err = service.save(); err != nil {
		if previous == nil {
			delete(service.config.Accounts, identity.ID)
		} else {
			service.config.Accounts[identity.ID] = previous
		}
		return nil, fmt.Errorf("save imported ESI tokens: %w", err)
	}
	return account, nil
}

type httpStatusError struct{ Status int }

func (err *httpStatusError) Error() string { return fmt.Sprintf("ESI returned HTTP %d", err.Status) }

func (service *esiService) requestJSON(ctx context.Context, method, endpoint, bearer string, form url.Values, result any) error {
	if strings.HasPrefix(endpoint, service.baseURL+"/") && time.Now().Before(service.backoffUntil) {
		return fmt.Errorf("ESI rate limit: retry after %s", service.backoffUntil.Format("15:04:05"))
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Evela/1.0 (EVE Online desktop damage analyzer)")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Compatibility-Date", "2026-10-16")
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	// Application credentials go only to the discovered SSO token endpoint.
	// The HTTP client rejects redirects, so a redirect cannot forward them.
	if form != nil && method == http.MethodPost && service.metadata != nil && endpoint == service.metadata.Token && service.config.AuthMode == esiAuthClientSecret {
		request.SetBasicAuth(service.config.ClientID, service.config.ClientSecret)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := service.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if strings.HasPrefix(endpoint, service.baseURL+"/") {
		if response.StatusCode == 420 || response.StatusCode == 429 || response.Header.Get("X-Esi-Error-Limit-Remain") == "0" {
			delay := 60 * time.Second
			if seconds, e := strconv.Atoi(response.Header.Get("Retry-After")); e == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			} else if at, e := http.ParseTime(response.Header.Get("Retry-After")); e == nil && at.After(time.Now()) {
				delay = time.Until(at)
			} else if seconds, e := strconv.Atoi(response.Header.Get("X-Esi-Error-Limit-Reset")); e == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
			service.backoffUntil = time.Now().Add(delay)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &httpStatusError{response.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(result)
}

func (service *esiService) refreshShips(ctx context.Context) error {
	var issues []string
	for _, account := range service.config.Accounts {
		if ctx.Err() != nil {
			issues = append(issues, ctx.Err().Error())
			break
		}
		err := service.refreshShip(ctx, account)
		if err != nil {
			account.Ship.Error = err.Error()
			issues = append(issues, account.Ship.CharacterName+": "+err.Error())
		} else {
			account.Ship.Error = ""
			if account.Ship.ClassError != "" {
				issues = append(issues, account.Ship.CharacterName+": "+account.Ship.ClassError)
			}
		}
	}
	if err := service.refreshShipHistoryMetadata(ctx); err != nil {
		issues = append(issues, err.Error())
	}
	// Save even after failures: a refresh token may have rotated successfully
	// before the subsequent ship request failed.
	if err := service.save(); err != nil {
		issues = append(issues, "Save ESI connections: "+err.Error())
	}
	if len(issues) > 0 {
		return fmt.Errorf("%d ESI issue(s). %s", len(issues), issues[0])
	}
	return nil
}

func (service *esiService) refreshShip(ctx context.Context, account *esiAccount) error {
	now := time.Now()
	if account.Token.ExpiresAt.Before(now.Add(time.Minute)) && (account.Token.RefreshToken != "" || !account.Token.ExpiresAt.After(now)) {
		if err := service.renew(ctx, account); err != nil {
			return err
		}
	}
	var response struct {
		TypeID int64  `json:"ship_type_id"`
		Name   string `json:"ship_name"`
		ShipID int64  `json:"ship_item_id"`
	}
	endpoint := fmt.Sprintf("%s/characters/%d/ship/", service.baseURL, account.Ship.CharacterID)
	err := service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &response)
	var status *httpStatusError
	if errors.As(err, &status) && status.Status == http.StatusUnauthorized {
		if err = service.renew(ctx, account); err == nil {
			err = service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &response)
		}
	}
	if err != nil {
		return err
	}
	if response.TypeID <= 0 {
		return fmt.Errorf("ESI did not return a ship type")
	}
	ship := shipRecord{CharacterID: account.Ship.CharacterID, CharacterName: account.Ship.CharacterName,
		ShipID: response.ShipID, TypeID: response.TypeID, ShipName: response.Name, CheckedAt: service.now().UTC()}
	continuous := account.Ship.Error == "" && !account.Ship.CheckedAt.IsZero()
	// The private endpoint already supplied a valid observation. Keep it even
	// if the public hull-name lookup fails; the type ID still supports correlation.
	defer func() { service.rememberShip(ship, continuous) }()
	hull, err := service.lookupShipType(ctx, response.TypeID)
	if err != nil {
		return err
	}
	ship.TypeName, ship.GroupID = hull.Name, hull.GroupID
	// Class lookup failures leave the confirmed hull usable and retry next poll.
	ship.ClassName, err = service.lookupShipClass(ctx, hull.GroupID)
	if err != nil {
		ship.ClassError = err.Error()
	}
	account.Ship = ship
	return nil
}

func (service *esiService) lookupShipType(ctx context.Context, typeID int64) (shipTypeInfo, error) {
	hull := service.shipTypes[typeID]
	if hull.Name == "" {
		if err := service.requestJSON(ctx, http.MethodGet, fmt.Sprintf("%s/universe/types/%d/?language=en", service.baseURL, typeID), "", nil, &hull); err != nil {
			return hull, err
		}
		if hull.Name == "" {
			return hull, fmt.Errorf("ESI did not return a ship type name")
		}
		service.shipTypes[typeID] = hull
	}
	return hull, nil
}

func (service *esiService) lookupShipClass(ctx context.Context, groupID int64) (string, error) {
	if name := service.groupNames[groupID]; name != "" {
		return name, nil
	}
	if groupID <= 0 {
		return "", fmt.Errorf("ESI did not return a ship class ID")
	}
	var group struct {
		Name string `json:"name"`
	}
	if err := service.requestJSON(ctx, http.MethodGet, fmt.Sprintf("%s/universe/groups/%d/?language=en", service.baseURL, groupID), "", nil, &group); err != nil {
		return "", fmt.Errorf("Look up ship class: %w", err)
	}
	if strings.TrimSpace(group.Name) == "" {
		return "", fmt.Errorf("ESI did not return a ship class name")
	}
	service.groupNames[groupID] = group.Name
	return group.Name, nil
}

// Older history did not save class metadata. Resolve it by hull ID without
// altering the observed periods or assigning a character's current hull to them.
func (service *esiService) refreshShipHistoryMetadata(ctx context.Context) error {
	type metadata struct {
		hull  shipTypeInfo
		class string
	}
	resolved := map[int64]metadata{}
	attempted := map[int64]bool{}
	for _, account := range service.config.Accounts {
		if account.Ship.ClassError != "" {
			attempted[account.Ship.TypeID] = true
		}
	}
	var firstError error
	for _, periods := range service.config.ShipHistory {
		for index := range periods {
			period := &periods[index]
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if period.TypeID <= 0 || (period.TypeName != "" && period.ClassName != "") {
				continue
			}
			info, ok := resolved[period.TypeID]
			if !ok && !attempted[period.TypeID] {
				attempted[period.TypeID] = true
				hull, err := service.lookupShipType(ctx, period.TypeID)
				if err == nil {
					info.hull = hull
					info.class, err = service.lookupShipClass(ctx, hull.GroupID)
					resolved[period.TypeID] = info
				}
				if err != nil && firstError == nil {
					firstError = fmt.Errorf("Ship history metadata: %w", err)
				}
			}
			if info.hull.Name != "" {
				period.TypeName, period.GroupID = info.hull.Name, info.hull.GroupID
			}
			if info.class != "" {
				period.ClassName = info.class
			}
		}
	}
	return firstError
}

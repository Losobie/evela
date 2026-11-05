# ESI setup and connection help

[User guide](../README.md) · [Data and history](DATA.md) · [Developer guide](DEVELOPMENT.md)

ESI is optional. Evela can analyze local logs without a connection; sign in to record your characters' ships and locations for historical filtering and grouping.

## Connect characters

1. Open **Settings → ESI configuration**, or **Configure ESI…** in the **ESI & ships** tab.
2. The **Application** tab is prefilled with Evela's public Client ID. Keep it unless you want to use your own application.
3. Open **Character tokens**, choose **Sign in with EVE Online**, and authorize a character in the browser. Repeat for each character. Log characters match by name, ignoring capitalization.
4. In **ESI & ships**, enable **Refresh every 30 seconds**, or choose **Refresh ships & locations** manually.

Only one browser sign-in runs at a time. **Cancel sign-in** releases the local callback listener so you can retry. Background ship/location refreshes can continue during sign-in.

Use **Ship history…** and **Location history…** to inspect recorded observations. Ship type/class and system filters use history at the event timestamp, not the character's present ship or location. Evela cannot recover ship observations from before tracking began. See [history timing and attribution](DATA.md#ship-and-location-history) for coverage rules.

## Use your own EVE application

Sign-in uses a public native/desktop application with OAuth PKCE. No client secret is needed or stored.

1. Open [EVE developer applications](https://developers.eveonline.com/applications), also linked from the Application tab.
2. Configure a native/desktop application with both `esi-location.read_ship_type.v1` and `esi-location.read_location.v1` scopes.
3. Register the exact callback URL displayed by Evela. The default is `http://localhost:8080/callback`.
4. Enter the application's public **Client ID**, set the matching **Callback port**, and choose **Save application configuration**.
5. Sign in from **Character tokens**.

The default public Client ID is `b680ade5175c4385a1e800b4bedb70bb`. The callback hostname stays `localhost`; the port can be 1–65535. Use **Copy callback URL** to avoid a mismatch. Changing the port preserves saved connections but also requires updating the callback registered with CCP. Changing Client ID requires disconnecting existing tokens first.

Older ship-only sign-ins still work for ships. Sign in again to grant location access. Legacy client-secret settings are ignored and omitted when settings are next saved; old applications may need conversion to PKCE and a new sign-in.

## Connection behavior and troubleshooting

- **Browser sign-in cannot complete:** check that the callback exactly matches the registered URL and the local port is available. Cancel the current attempt before retrying.
- **Ships appear but locations do not:** confirm both scopes are enabled and sign in again if the token predates location access.
- **History shows unknown values:** observations cover recorded periods, with a short last-observed allowance. Old logs do not acquire earlier ship history just because a character connects now.
- **ESI is unavailable or rate-limited:** log analysis continues. Check the ESI status/details and retry later; recorded history is retained.
- **A hull, class or system name is missing:** confirmed IDs remain recorded, and refresh retries public name lookups, including for disconnected characters.

Access tokens renew automatically, including rotating refresh tokens. An HTTP 401 triggers one renewal/retry; ESI rate limits pause further requests. Ship and location queries succeed independently, so failure of one does not discard the other. Public type, group and system names are cached for the session.

## Stored data and access

On Windows, settings, tokens and ship/location history are stored atomically in `%APPDATA%\Evela\esi.dat`, encrypted with Windows DPAPI for the current user. Token values are not displayed in the UI. Persistent ESI connections currently require Windows.

**Disconnect** removes the local saved sign-in while retaining history. To revoke server-side authorization too, revoke the application's grant in EVE account management. There is no automatic history expiry.

The callback listener binds only to IPv4 loopback (`127.0.0.1`) on the configured port and advertises `http://localhost:<port>/callback`. Sign-in validates OAuth state and PKCE. Access tokens are checked against EVE's published RSA/ECDSA keys, issuer, audience, expiry and required scope. Queries send access tokens and character IDs to CCP; Evela does not upload combat logs.

References: [EVE SSO documentation](https://developers.eveonline.com/docs/services/sso/) and [ESI overview](https://developers.eveonline.com/docs/services/esi/overview/).

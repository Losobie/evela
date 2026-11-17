# ESI setup and connection help

[User guide](../README.md) · [Data and history](DATA.md) · [Developer guide](DEVELOPMENT.md)

ESI is optional. Evela can analyze local logs without a connection; sign in to record your characters' ships and locations for historical filtering and grouping.

## Connect characters

1. Open **ESI & ships**. **Configure ESI** opens the application form directly, with default Client ID and callback values as grey placeholders. Leave the fields blank to use those defaults, or type your own values.
2. Choose **Sign in with EVE Online** to open the scope-selection dialog.
3. Select ship access, location access and/or structure-name access, then choose **Add character**. All three scopes are selected initially and can be unchecked, including signing in without data scopes. Only the checked scopes are requested, and the EVE application registration must allow them.
4. Authorize a character in the browser. The dialog stays open and displays **Added character:** followed by the character's name after the connection is accepted. Use **Add character** again for another character, or close the dialog when finished. Log characters match by name, ignoring capitalization. Enable **Refresh every 30 seconds** on the right to poll the granted data access automatically. A successful sign-in also refreshes once immediately.

Only one browser sign-in runs at a time. **Cancel sign-in** releases the local callback listener so you can retry. Background ship/location refreshes can continue during sign-in.

The table's **Scopes** column, immediately before **ESI Status**, lists granted access as **Ship**, **Location**, and **Structures**, or **All** when all three are granted. **None** means no supported data scopes are granted; **Unknown** means legacy permissions have not yet been identified. Missing scopes alone leave **ESI Status** as **OK**; actual authentication, network and lookup failures remain visible.

Use **Ship & Location History** to inspect recorded observations. Ship type/class and system filters use history at the event timestamp, not the character's present ship or location. Evela cannot recover ship observations from before tracking began. See [history timing and attribution](DATA.md#ship-and-location-history) for coverage rules.

## Use your own EVE application

Sign-in uses a public native/desktop application with OAuth PKCE. No client secret is needed or stored.

1. Open [EVE developer applications](https://developers.eveonline.com/applications), also linked from the Application tab.
2. Configure a native/desktop application with the scopes you intend to select: `esi-location.read_ship_type.v1` for ship tracking, `esi-location.read_location.v1` for location tracking, and `esi-universe.read_structures.v1` for player-owned structure names.
3. Register the exact callback URL displayed by Evela. The default is `http://localhost:8080/callback`.
4. Enter the application's public **Client ID** and the full **Callback** URL, then choose **Save application configuration**. Clearing either field restores its default.
5. Return to **ESI & ships**, choose **Sign in with EVE Online**, select the matching scopes and sign in.

The default public Client ID is `b680ade5175c4385a1e800b4bedb70bb`. The callback uses `http://localhost:<port>/callback`; the port can be 1–65535. The default is `http://localhost:8080/callback`. Changing the callback port preserves saved connections but also requires updating the callback registered with CCP. Changing Client ID requires disconnecting existing tokens first. Saved custom values appear as editable text; saved defaults remain placeholders.

Older ship-only sign-ins still work for ships. Sign in again to grant location access. Legacy client-secret settings are ignored and omitted when settings are next saved; old applications may need conversion to PKCE and a new sign-in.

The ESI table shows ship types and resolves NPC station names publicly. Player-owned structure names require `esi-universe.read_structures.v1` and a character with docking access. Enable that scope in the application's EVE registration, then select it in the sign-in dialog. Current docking information also requires location access. Unchecked ship/location scopes disable their private ESI queries while preserving observations and history. An unresolved system, station or structure displays its numeric ID; individual lookup and query errors appear in the **ESI Status** column. Successful station names are cached for the session; structure names are cached separately for each character for five minutes, then checked again. Copied cells use the same values as the table.

## Connection behavior and troubleshooting

- **Browser sign-in cannot complete:** check that the callback exactly matches the registered URL and the local port is available. Cancel the current attempt before retrying.
- **Sign-in reports `invalid_scope`:** one of the checked scopes may not be enabled for that Client ID in the EVE application registration. Cancel the attempt, uncheck that scope, or enable it in the registration before retrying.
- **Ships appear but locations do not:** confirm both scopes are enabled and sign in again if the token predates location access.
- **History shows unknown values:** observations cover recorded periods, with a short last-observed allowance. Old logs do not acquire earlier ship history just because a character connects now.
- **ESI is unavailable or rate-limited:** log analysis continues. Check the ESI status/details and retry later; recorded history is retained.
- **A hull, class or system name is missing:** confirmed IDs remain recorded, and refresh retries public name lookups, including for disconnected characters.

Access tokens renew automatically, including rotating refresh tokens. An HTTP 401 triggers one renewal/retry; ESI rate limits pause further requests. Ship and location queries succeed independently, so failure of one does not discard the other. Public type, group and system names are cached for the session.

## Stored data and access

On Windows, settings, tokens and ship/location history are stored atomically in `%APPDATA%\Evela\esi.dat`, encrypted with Windows DPAPI for the current user. Token values are not displayed in the UI. Persistent ESI connections currently require Windows.

Right-click a character cell and choose **Disconnect** to remove its local saved sign-in while retaining history. To revoke server-side authorization too, revoke the application's grant in EVE account management. There is no automatic history expiry.

The callback listener binds only to IPv4 loopback (`127.0.0.1`) on the configured port and advertises `http://localhost:<port>/callback`. Sign-in validates OAuth state and PKCE. Access tokens are checked against EVE's published RSA/ECDSA keys, issuer, audience, expiry and character identity. Sign-in also verifies that the selected scopes were granted. Queries send access tokens and character IDs to CCP; Evela does not upload combat logs.

References: [EVE SSO documentation](https://developers.eveonline.com/docs/services/sso/) and [ESI overview](https://developers.eveonline.com/docs/services/esi/overview/).

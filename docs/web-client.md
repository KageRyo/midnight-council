# Web Client

The web client is a responsive single-page interface served by the same Go process as the WebSocket API. It is intentionally framework-free and embedded in the executable, so local development and deployment do not require Node.js or a separate static-file server.

## Source Layout

```text
internal/webui/
├── handler.go             embedded asset HTTP handler
├── handler_test.go        routing and response-header tests
└── static/
    ├── index.html         semantic application structure
    ├── app.css            desktop and mobile presentation
    └── app.js             WebSocket client and state rendering
```

`cmd/server/main.go` mounts the handler at `/`. More-specific `/healthz` and `/ws/rooms/` routes continue to take precedence.

## Joining a Room

1. Enter a room ID and display name, then choose whether to join as a spectator.
2. The client creates a random player ID unless that room already has a saved browser session.
3. It opens `/ws/rooms/{room_id}` with `player_id`, `name`, optional `spectator=true`, and, when available, `reconnect_token` query parameters.
4. The game view appears only after the first valid state envelope arrives.

The browser UI limits room IDs to 2–48 ASCII letters, digits, hyphens, or underscores. Display names are limited to 32 JavaScript characters; the server still performs its own required-field validation.

Invitation links use `?room={room_id}`. They include only the public room ID and never contain a player ID or reconnect token.

## Rendering Rules

The client treats each server state envelope as authoritative and re-renders these areas:

- connection, room, phase, round, and server-synchronized countdown status;
- public player seats plus connected, disconnected, and AFK state;
- room access state, player capacity, and the spectator list;
- the current preset, timing, minimum players, role pool, and death-reveal rule;
- the current player's private role and life state;
- detective investigation results;
- controls allowed by the current phase and `private.available`;
- shooter availability;
- final winner and public role reveal;
- the capped public game event log.
- owner-only room administration controls.

It does not calculate kills, vote results, role assignments, or winners locally. Client-side disabled buttons are a usability feature only; the room state validates every submitted event again.

## Action Mapping

| Phase | Browser actions |
| --- | --- |
| `WAITING` | Non-owner player ready toggle; owner start and room administration; player and spectator chat |
| `NIGHT` | Role-specific target action or pass; missing actions pass at timeout; living-player chat; spectators observe |
| `DAY_DISCUSSION` | Owner may start voting early; deadline starts it automatically; shooter may fire; living-player chat; spectators observe |
| `DAY_VOTING` | Living players vote or abstain; missing votes abstain at timeout; shooter may fire; living-player chat; spectators observe |
| `FINISHED` | Final result and roles; owner may administer or return to waiting; player and spectator chat |

Night actors can change their submitted target until the last required action causes resolution. Voters can similarly change a non-empty vote until the last living player submits. The current private projection cannot distinguish a submitted abstention from no vote, so the UI continues to describe abstention as available until resolution.

## Room Administration and Rematches

The owner panel reflects authoritative room state. It can lock or unlock new joins, set the player cap from 2 through 20 while waiting or after a game, transfer ownership to another connected player, and remove players or spectators while waiting or after a game. These controls disappear immediately after ownership transfers.

During an active or finished game, the owner can return the room to `WAITING`. The client asks for confirmation because the server clears the current result and game log. Connected identities remain in the room, player readiness resets, and the same group can prepare for another match without changing invitation links.

Spectators have a separate identity card and participant list. They never see role actions, readiness, or voting controls. Spectator chat is available while waiting and after settlement, but disabled during active play to prevent live information from being relayed into the game.

## Game Settings

Every participant sees the current game-setting summary. The owner additionally receives an editor while the room is waiting or finished. The editor can apply the server-provided `STANDARD`, `QUICK`, `BEGINNER`, `ADVANCED`, or `MINIMAL` preset, or switch to `CUSTOM` and choose:

- night, discussion, and voting durations from one second through one hour;
- minimum players, bounded by the room player cap;
- immediate role reveal when a player is eliminated;
- one required killer and optional detective, doctor, escort, and shooter slots.

The escort is a night role. Its target's submitted role action is ignored for that night. The client prevents self-targeting and presents the server-defined resolution order as block, protect, kill, then investigate; the room state validates the same rules authoritatively.

The browser performs convenience checks, but the server remains authoritative for duration parsing, capacity compatibility, role counts, and start eligibility. Applying a setting resets all non-owner readiness. The start button and waiting hint use `game_settings.minimum_players`, not a hard-coded count.

## Server-Synchronized Countdown

Timed snapshots contain `phase_started_at`, `phase_deadline`, and `server_time`. On receipt, the browser calculates the offset between server time and the local clock, then renders the remaining time against the absolute deadline. The display updates four times per second and switches to an urgent treatment for the last ten seconds.

At `00:00`, the browser stops its local display and waits for the next authoritative state broadcast. It does not submit timeout events, fill missing actions, calculate a result, or change phases locally. Reconnect snapshots carry the same absolute deadline, so a returning player immediately sees the current remaining time. The timer stops displaying synchronized time if the WebSocket disconnects.

## Browser Storage and Reconnect

The client stores two versioned local-storage entries:

- the most recently used display name;
- a map of room IDs to participant ID, display name, spectator flag, reconnect token, next client sequence, and up to 50 unacknowledged event payloads.

On refresh, the room query parameter, saved name, and participant type repopulate the join form. The participant explicitly selects **Enter Council** again, after which the saved token reclaims the existing identity. After a live socket loss, the current page reconnects automatically with exponential delay capped at ten seconds and no attempt limit. Controls stay disabled while offline. Once open, pending events are replayed in ascending sequence order; `applied`, `duplicate`, and `rejected` acknowledgements all remove the corresponding pending payload. Missing or rejected reconnect credentials are cleared so the next attempt can create a new identity where the room state permits it. A participant removed by the owner also loses the saved room session.

Rooms opened in multiple tabs of the same browser profile share local storage and therefore represent the same saved seat. The newest valid socket takes exclusive ownership of that seat; the previous tab is told it was replaced, disables its controls, and does not start a reconnect contest. Use separate profiles, browsers, or private contexts to test multiple players on one machine.

Pointer, keyboard, or touch input resets a two-minute activity timer. Timer expiry or hiding the document sends `presence` with `afk: true`; renewed activity or visibility sends `afk: false`. This signal and transport `connected` state are rendered separately for both players and spectators.

Reconnect tokens are bearer credentials. Do not log them, include them in invitation URLs, expose them in screenshots, or move them into public state. Local storage is acceptable for this prototype; an authenticated production client should revisit token storage, rotation, revocation, and WebSocket origin policy together.

## Chat Behavior

Chat messages are broadcast live and the browser keeps only the latest 150 messages received by that page. The room state does not retain chat history, so a refreshed or reconnected client starts with an empty chat panel. Public game events are separate and remain in the room snapshot up to the server's 100-entry cap.

During an active game, dead players and spectators cannot chat. The client disables the field, and the server independently rejects attempts.

The server also limits chat and game events with separate per-connection token buckets. When either limit is exceeded, the rejected event never reaches the room actor and the browser presents the returned error as a localized toast. The browser does not predict local token availability; the server remains authoritative, and a normally paced retry succeeds after tokens refill.

After rate limiting, the server's chat policy may allow, reject, or replace a message. Rejected messages produce an error toast only for the sender. Replaced messages arrive through the normal chat envelope, so every client—including the sender—renders only the server-approved text. Policy availability failures are also shown as localized errors.

## Accessibility and Responsive Layout

The HTML uses landmarks, headings, form labels, live status regions, and keyboard focus styles. All dynamic player content is inserted as text rather than HTML. The layout collapses from three columns to two and then one column at smaller widths; controls remain native buttons, inputs, and selects.

Reduced-motion preferences disable nonessential transition duration. The interface does not rely on color alone for phase, life, connection, or result information.

## Local Verification

Start the application and open the root page:

```bash
make run
```

```text
http://localhost:8080
```

For a manual multiplayer check:

1. Open the same room in two separate browser profiles.
2. Ready the non-owner and start from the owner.
3. Confirm that each browser sees only its own role.
4. Submit or pass all required night actions.
5. Exercise discussion, voting, and any available shooter action.
6. Interrupt one live connection and confirm automatic reconnect restores its seat and pending actions are not applied twice.
7. Join a spectator during the active game and confirm it receives no role or action controls and cannot chat.
8. Return to waiting and confirm roles, readiness, result, and game log reset while connected identities remain.
9. Exercise lock, player cap, kick, and ownership transfer controls.
10. Apply a preset, then a custom rule set; confirm readiness resets, minimum-player gating changes, and the next night uses the selected duration.
11. Enable death-role reveal and confirm an eliminated role becomes public while living roles remain hidden.
12. Run with short phase durations and confirm missing actions, discussion, and votes advance automatically.
13. Open the same saved seat in a second tab and confirm the first tab is terminally replaced rather than reconnecting.
14. Hide a tab or wait two minutes and confirm AFK appears separately from disconnected state.

The Go suite tests embedded assets and the complete WebSocket game flow:

```bash
make test
```

When Node.js is available, JavaScript syntax can also be checked without installing dependencies:

```bash
node --check internal/webui/static/app.js
```

## Current Limitations

- no retained chat history;
- no audible cues or notifications;
- no built-in word list, external moderation provider, reporting, mute, or ban workflow;
- no frontend unit-test framework or committed browser-test suite;
- browser-generated player IDs are identities only within an in-memory room, not accounts.

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

1. Enter a room ID and display name.
2. The client creates a random player ID unless that room already has a saved browser session.
3. It opens `/ws/rooms/{room_id}` with `player_id`, `name`, and, when available, `reconnect_token` query parameters.
4. The game view appears only after the first valid state envelope arrives.

The browser UI limits room IDs to 2–48 ASCII letters, digits, hyphens, or underscores. Display names are limited to 32 JavaScript characters; the server still performs its own required-field validation.

Invitation links use `?room={room_id}`. They include only the public room ID and never contain a player ID or reconnect token.

## Rendering Rules

The client treats each server state envelope as authoritative and re-renders these areas:

- connection, room, phase, round, and server-synchronized countdown status;
- public player seats and connection state;
- the current player's private role and life state;
- detective investigation results;
- controls allowed by the current phase and `private.available`;
- shooter availability;
- final winner and public role reveal;
- the capped public game event log.

It does not calculate kills, vote results, role assignments, or winners locally. Client-side disabled buttons are a usability feature only; the room state validates every submitted event again.

## Action Mapping

| Phase | Browser actions |
| --- | --- |
| `WAITING` | Non-owner ready toggle; owner start when eligible; chat |
| `NIGHT` | Role-specific target action or pass; missing actions pass at timeout; chat for living players |
| `DAY_DISCUSSION` | Owner may start voting early; deadline starts it automatically; shooter may fire; chat |
| `DAY_VOTING` | Living players vote or abstain; missing votes abstain at timeout; shooter may fire; chat |
| `FINISHED` | Final result and roles; chat |

Night actors can change their submitted target until the last required action causes resolution. Voters can similarly change a non-empty vote until the last living player submits. The current private projection cannot distinguish a submitted abstention from no vote, so the UI continues to describe abstention as available until resolution.

## Server-Synchronized Countdown

Timed snapshots contain `phase_started_at`, `phase_deadline`, and `server_time`. On receipt, the browser calculates the offset between server time and the local clock, then renders the remaining time against the absolute deadline. The display updates four times per second and switches to an urgent treatment for the last ten seconds.

At `00:00`, the browser stops its local display and waits for the next authoritative state broadcast. It does not submit timeout events, fill missing actions, calculate a result, or change phases locally. Reconnect snapshots carry the same absolute deadline, so a returning player immediately sees the current remaining time. The timer stops displaying synchronized time if the WebSocket disconnects.

## Browser Storage and Reconnect

The client stores two versioned local-storage entries:

- the most recently used display name;
- a map of room IDs to player ID, display name, and reconnect token.

On refresh, the room query parameter and saved name repopulate the join form. The player explicitly selects **Enter Council** again, after which the saved token reclaims the existing seat. Missing or rejected reconnect credentials are cleared so the next attempt can create a new player ID where the room state permits it.

Rooms opened in multiple tabs of the same browser profile share local storage and therefore represent the same saved seat. Use separate profiles, browsers, or private contexts to test multiple players on one machine.

Reconnect tokens are bearer credentials. Do not log them, include them in invitation URLs, expose them in screenshots, or move them into public state. Local storage is acceptable for this prototype; an authenticated production client should revisit token storage, rotation, revocation, and WebSocket origin policy together.

## Chat Behavior

Chat messages are broadcast live and the browser keeps only the latest 150 messages received by that page. The room state does not retain chat history, so a refreshed or reconnected client starts with an empty chat panel. Public game events are separate and remain in the room snapshot up to the server's 100-entry cap.

During an active game, dead players cannot chat. The client disables the field, and the server independently rejects attempts.

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
6. Refresh one browser and confirm the reconnect prompt restores its seat.
7. Run with short phase durations and confirm missing actions, discussion, and votes advance automatically.

The Go suite tests embedded assets and the complete WebSocket game flow:

```bash
make test
```

When Node.js is available, JavaScript syntax can also be checked without installing dependencies:

```bash
node --check internal/webui/static/app.js
```

## Current Limitations

- no automatic reconnect loop after a live connection drops;
- no retained chat history;
- no audible cues or notifications;
- no room restart after `FINISHED`;
- no built-in word list, external moderation provider, reporting, mute, or ban workflow;
- no frontend unit-test framework or committed browser-test suite;
- browser-generated player IDs are identities only within an in-memory room, not accounts.

# Architecture

Midnight Council is a single-process Go application with an embedded browser client. The server owns every game decision; browser clients only submit intent and render the public and private projections returned by the server.

## Components

```text
Browser client
  ├─ GET /                         embedded HTML, CSS, and JavaScript
  └─ WebSocket /ws/rooms/{id}
             │
             ▼
internal/ws.Handler               connection lifecycle and transport
             │
             ▼
internal/protocol                 strict client-event decoding
             │
             ▼
internal/ws connection limiter   chat/game token buckets
             │
             ▼
internal/moderation.ChatPolicy   allow/reject/replace chat
             │
             ▼
internal/room.Hub                 room lookup and idle cleanup
             │
             ▼
internal/room.Actor               serialized commands and phase timers
             │
             ▼
internal/room.State               game rules and state projections
```

### Server entrypoint

`cmd/server/main.go` builds a single `http.ServeMux` with three routes:

| Route | Purpose |
| --- | --- |
| `GET /` | Embedded browser game client |
| `GET /healthz` | Process health check |
| `/ws/rooms/{room_id}` | WebSocket room connection |

The process handles `SIGINT` and `SIGTERM` and gives active HTTP requests up to ten seconds to shut down.

### Browser client

`internal/webui` embeds its `static` directory into the Go binary. Its handler only accepts `GET` and `HEAD`, serves no runtime-generated HTML, and adds a restrictive Content Security Policy plus framing, referrer, and content-type headers.

The client has no build step or third-party runtime dependency. It renders server state using DOM APIs and `textContent`; player names and chat messages are never inserted as HTML.

### WebSocket transport

`internal/ws` upgrades the request, dispatches the initial join, subscribes the connection to its room actor, and then runs one reader and one writer path per connection.

- Reads are limited to 1 KiB.
- Client events must be text JSON messages.
- The server expects a pong within 60 seconds and sends a ping every 45 seconds.
- Writes have a 10-second deadline.
- A transport loss marks the active participant disconnected without deleting its seat; an explicit normal close with reason `player left` performs the permanent waiting-room leave path.
- Each connection has independent chat and game-event token buckets.
- The `spectator=true` query parameter creates or reclaims a spectator identity instead of a player seat.

Only text events that pass protocol decoding and shape validation reach the connection limiter. Chat consumes the chat bucket; every other accepted client event consumes the game bucket. An event with no token is answered with an error envelope and is never dispatched to the room actor. Invalid JSON and schema errors also never reach the actor.

An initial join rejection writes an error envelope and then closes the WebSocket with policy-violation code `1008`; short public errors are repeated as the close reason so browsers can still explain the rejection if they observe the close before rendering the envelope. A valid reconnect for a seat that already has a subscriber atomically replaces that subscription, sends `connection replaced by a newer session` to the old socket, and closes it with the same code. Actor dispatch from the replaced subscription is rejected, so a stale reader cannot mutate room state during shutdown.

Buckets start at burst capacity. Chat defaults to one token per second with a burst of five; game events default to five tokens per second with a burst of ten. `WS_CHAT_EVENTS_PER_SECOND`, `WS_CHAT_BURST`, `WS_GAME_EVENTS_PER_SECOND`, and `WS_GAME_BURST` override these positive values at startup. Limits are process-local and connection-local, so a new WebSocket receives fresh buckets; cross-instance and account-wide controls remain future infrastructure concerns.

After rate limiting, chat events pass through `moderation.ChatPolicy`. A policy receives room and player metadata plus the validated message, then returns allow, reject, or replace. Replacement text is trimmed and revalidated against the server's non-empty and 500-byte rules before dispatch. Rejections produce an error only for the sender. Policy errors, unsupported decisions, and invalid replacements fail closed; the transport logs the failure without logging message content and returns a generic availability error.

`moderation.AllowAllChat` is the default, so the hook does not alter existing gameplay until a deployment supplies another policy with `ws.WithChatPolicy`. The policy boundary is intentionally separate from room state: rejected text never enters the actor, while accepted or replaced text still passes through normal room authorization such as the dead-player chat rule.

The WebSocket upgrader currently accepts every HTTP origin. This is suitable for the prototype and local client, but an explicit deployment-origin allowlist is required before exposing authenticated sessions publicly.

### Protocol validation

`internal/protocol` decodes exactly one JSON object, rejects unknown fields, validates the event type, then enforces the fields accepted by that specific event. Only validated events are converted to `room.Event` values.

The mirrored machine-readable client schema is `docs/websocket-client-event.schema.json`. Transport details and server envelopes are documented in `docs/websocket-protocol.md`.

### Connection reliability

The official client assigns each event a monotonically increasing JavaScript-safe `sequence`. The room stores only the latest successfully applied sequence on the private player or spectator record. A sequence at or below that value returns a `duplicate` acknowledgement and does not run the state transition again. Newer successful events return `applied`; validated events rejected by rate limiting, moderation, or room authorization return `rejected` alongside the normal error envelope. Unsequenced clients remain supported but do not receive delivery acknowledgements.

The browser persists its next sequence and up to 50 unacknowledged payloads with the room reconnect token. A live connection loss disables controls, retries forever with exponential delay capped at ten seconds, and replays pending payloads in ascending order after the socket opens. An acknowledgement removes the payload regardless of status. A terminal kick, invalid identity, or newer connection replacement blocks that retry loop.

Presence is an ordinary validated `presence` event. The browser reports AFK after two minutes without pointer, keyboard, or touch activity, and immediately when the document becomes hidden; activity or visibility clears it. Public player and spectator projections include both `connected` and `afk`, while transport disconnect always clears AFK.

### Hub and room actors

`room.Hub` owns the map of room IDs to actors. The first connection creates a room implicitly. Each actor has one inbox and applies commands sequentially, so state transitions within a room do not need shared-state locks.

The actor broadcasts successful state changes to all subscribers. Each participant ID can own only one subscription. Each subscriber has a 16-message buffer; a subscriber that cannot keep up is removed, marked disconnected, and announced to the remaining room rather than blocking the actor. State envelopes are personalized immediately before delivery.

Kicking a participant is an actor-level terminal operation: the room discards pending events for that identity, guarantees one private removal error, removes and closes every matching subscription, then broadcasts the new public state to the remaining participants.

The actor owns one phase timer in addition to its idle timer. `State` supplies an absolute deadline, while the actor only schedules its wake-up and sends the internal timeout event back through the same serialized state-machine path. A phase change stops and resets the old timer before the actor waits again. Closing the actor stops both timers.

When a room has no subscribers, its idle timer starts. The default is 30 minutes and can be overridden with `ROOM_IDLE_TIMEOUT`. Internal phase transitions do not extend an already-idle room's lifetime. On idle expiry, the actor closes and the hub removes it. Because rooms are in memory, expiry or a process restart discards the room permanently.

## State Model

The room state machine has six phases:

```text
WAITING
   │ owner starts after all non-owners are ready
   ▼
NIGHT
   │ all actions or night deadline
   ├──────────────────────────────► FINISHED when a win condition is met
   ▼
DAY_DISCUSSION
   │ owner or deadline
   ▼
DAY_VOTING
   │ all votes or deadline
   ├── no execution ──────────────► next NIGHT
   ▼ execution
LAST_WORDS
   │ last-words deadline
   ├──────────────────────────────► next NIGHT
   └──────────────────────────────► FINISHED when a win condition is met

FINISHED ── owner return-to-waiting ──► WAITING
```

`State.Apply` is the only game-state mutation entrypoint. It validates phase, ownership, life state, role, targets, and early timeout attempts before applying an event. Night and voting phases resolve when every required living player submits or when their deadlines expire. Discussion moves to voting when its deadline expires. A successful vote execution enters `LAST_WORDS`, publishes the executed player ID, and delays win evaluation until that deadline; a tied or empty vote skips the phase.

At timeout, missing night actions become passes and missing votes become abstentions. A last-words timeout clears the speaker and evaluates the win condition before starting another night. Timeout transitions append a public `phase_timed_out` log entry before normal resolution logs. Waiting and finished phases have no deadline.

Default durations are 90 seconds for night, five minutes for discussion, 60 seconds for voting, and 30 seconds for last words. `NIGHT_DURATION`, `DAY_DISCUSSION_DURATION`, `DAY_VOTING_DURATION`, and `LAST_WORDS_DURATION` override the process values used by each new room's `STANDARD` preset; non-positive startup values are rejected.

Roles are shuffled using `crypto/rand`. Reconnect tokens and subscription IDs also use cryptographically secure randomness.

Role capabilities live in `internal/room/rules.go` as a rule-set registry. Each role definition owns its faction, optional night action, action priority, self-target policy, and optional day ability. State validation, private action availability, deck construction, investigation alignment, ordered night resolution, and win counts all read this registry instead of maintaining separate role switches. `Hub` accepts an alternate validated rule set for future modes. The default order is escort block, doctor protect, killer attack, then detective investigate; equal-priority actions use player ID only as a deterministic tie-breaker.

## Game Settings

Every room owns a public `game_settings` value and a server-defined `game_presets` catalog. The built-in presets are `STANDARD`, `QUICK`, `BEGINNER`, `ADVANCED`, and `MINIMAL`; arbitrary client-supplied preset definitions are not trusted. Owners may apply a preset or submit a complete custom configuration only in `WAITING` or `FINISHED`.

Settings contain the four timed phase durations, minimum player count, whether eliminated roles become public immediately, and role counts. Custom durations must be valid Go duration strings between one second and one hour. Minimum players must be from 2 through 20 and cannot exceed the room's player cap; conversely, the cap cannot be lowered below the configured minimum.

The current rules engine requires exactly one killer and permits zero or one detective, doctor, escort, and shooter. This constraint prevents the configuration UI from exposing role combinations whose team knowledge or night resolution is not implemented yet. At start, enabled special roles are added in killer, detective, doctor, escort, shooter order while reserving at least one seat for a villager; every remaining seat becomes a villager. The server validates the full configuration again before accepting it.

Changing settings clears every non-owner readiness flag so players must acknowledge the new rules before start. The selected settings and preset survive return-to-waiting resets and rematches. When `reveal_roles_on_death` is enabled, only eliminated players' roles enter the public projection during an active game; living roles remain private and all roles are still revealed at settlement.

## Room Lifecycle and Participants

The state distinguishes seated players from spectators. Players count toward the room's player cap and can receive roles; spectators have their own public list and private reconnect token but never receive a role or game action. A new player may join only in `WAITING`, while a new spectator may join during any phase. A locked room rejects both kinds of new participant but still permits a valid reconnect.

Rooms default to 12 player seats and owners may choose a limit from 2 through 20 while waiting or after a game. Spectators do not consume player capacity. Owners may lock or unlock the room in any phase, explicitly transfer ownership to a connected seated player, and kick a player or spectator while waiting or after a game. When the owner disconnects in any phase, ownership moves to another connected seated player when one exists.

The owner may send `return_to_waiting` from an active or finished phase. This clears the round, deadlines, roles, readiness, actions, votes, investigations, result, and public game log. Connected players and spectators keep their identities and reconnect tokens; disconnected participants are removed. Room-level options such as lock state and player cap remain unchanged for the rematch.

## Public and Private Projections

Every state broadcast consists of:

- `state`: safe public room information for every subscriber;
- `private`: information generated only for the subscribing player.

Before the game ends, public player views omit roles. A player's private view may include their role, reconnect token, currently available events, vote state, shooter availability, and detective investigations. A spectator private view contains only its identity, reconnect token, and spectator marker. At `FINISHED`, roles are copied into the public player list.

Public snapshots include player and spectator lists, owner ID, lock state, player cap, current settings, the authoritative preset catalog, `phase_started_at`, an optional `phase_deadline`, optional `last_words_player_id`, and `server_time`. The browser uses `server_time` only to compensate for client clock skew when rendering the deadline. It never advances a phase locally.

This projection boundary is a core invariant: hidden state must remain in `internal/room` and must never be derived or trusted from the client.

## Reconnect Lifecycle

Joining a new player or spectator identity creates a 32-byte reconnect token. A network disconnect preserves the identity in every phase and sets `connected` false, including in `WAITING`, so the token can reclaim it. An explicit leave removes the identity while waiting; during an active or finished game it remains disconnected so the authoritative match record is not altered. Disconnected waiting-room participants may be removed by the owner, and a return-to-waiting reset removes disconnected identities.

Reclaiming an existing participant ID requires the exact token and the same player-versus-spectator type. The browser stores the generated ID, type, and token per room and resubmits them when the participant chooses to reconnect. The token is present only in `PrivatePlayerView` and in the WebSocket connection query; it is never broadcast publicly.

## Persistence and Scaling Boundaries

The current design deliberately has no external state:

- accounts and rooms are not persisted;
- chat messages are live-only and are not retained in snapshots;
- game logs are public, capped at 100 entries, and live only as long as the room actor;
- a room exists on exactly one server process;
- there is no cross-instance routing or Pub/Sub.

PostgreSQL, Redis, and multi-instance routing belong behind these boundaries rather than inside the WebSocket transport or browser client.

## Test Boundaries

- `internal/protocol`: malformed JSON and event-shape validation;
- `internal/room`: state-machine rules, setting and role-pool validation, room lifecycle, spectator isolation, last-words authorization, timeout semantics, actor timer cancellation, private projections, sequence deduplication, single-subscription enforcement, reconnect, and idle cleanup;
- `internal/moderation`: default allow policy and chat policy contract;
- `internal/ws`: real WebSocket multiplayer flow, room administration, spectator connections, terminal replacement, network-versus-explicit disconnect, acknowledgements, rate-limit ordering, moderation decisions, last-words chat enforcement, automatic phase broadcasts, and transport errors;
- `internal/webui`: embedded asset routing, reconnect and presence source checks, deadline-consumption checks, content types, method handling, and security headers.

Run the full suite with `make test`.

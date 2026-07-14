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
internal/room.Hub                 room lookup and idle cleanup
             │
             ▼
internal/room.Actor               serialized room command processing
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
- A disconnect dispatches a room leave event with a two-second timeout.

The WebSocket upgrader currently accepts every HTTP origin. This is suitable for the prototype and local client, but an explicit deployment-origin allowlist is required before exposing authenticated sessions publicly.

### Protocol validation

`internal/protocol` decodes exactly one JSON object, rejects unknown fields, validates the event type, then enforces the fields accepted by that specific event. Only validated events are converted to `room.Event` values.

The mirrored machine-readable client schema is `docs/websocket-client-event.schema.json`. Transport details and server envelopes are documented in `docs/websocket-protocol.md`.

### Hub and room actors

`room.Hub` owns the map of room IDs to actors. The first connection creates a room implicitly. Each actor has one inbox and applies commands sequentially, so state transitions within a room do not need shared-state locks.

The actor broadcasts successful state changes to all subscribers. Each subscriber has a 16-message buffer; a subscriber that cannot keep up is removed rather than blocking the room. State envelopes are personalized immediately before delivery.

When a room has no subscribers, its idle timer starts. The default is 30 minutes and can be overridden with `ROOM_IDLE_TIMEOUT`. On expiry, the actor closes and the hub removes it. Because rooms are in memory, expiry or a process restart discards the room permanently.

## State Model

The room state machine has five phases:

```text
WAITING
   │ owner starts after all non-owners are ready
   ▼
NIGHT ────────────────┐
   │ all required     │
   │ actions submitted│
   ▼                  │
DAY_DISCUSSION        │
   │ owner starts vote│
   ▼                  │
DAY_VOTING ───────────┘
   │ all living players vote
   │
   └──────────────► FINISHED when a win condition is met
```

`State.Apply` is the only game-state mutation entrypoint. It validates phase, ownership, life state, role, and targets before applying an event. Night and voting phases resolve automatically when every required living player has submitted an action.

Roles are shuffled using `crypto/rand`. Reconnect tokens and subscription IDs also use cryptographically secure randomness.

## Public and Private Projections

Every state broadcast consists of:

- `state`: safe public room information for every subscriber;
- `private`: information generated only for the subscribing player.

Before the game ends, public player views omit roles. A player's private view may include their role, reconnect token, currently available events, vote state, shooter availability, and detective investigations. At `FINISHED`, roles are copied into the public player list.

This projection boundary is a core invariant: hidden state must remain in `internal/room` and must never be derived or trusted from the client.

## Reconnect Lifecycle

Joining a new seat creates a 32-byte reconnect token. During `WAITING`, leaving removes the seat. After the game begins, leaving only marks the seat disconnected so its role and game state remain intact.

Reclaiming an existing player ID requires the exact token. The browser stores the generated player ID and token per room and resubmits them when the player chooses to reconnect. The token is present only in `PrivatePlayerView` and in the WebSocket connection query; it is never broadcast publicly.

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
- `internal/room`: state-machine rules, private projections, reconnect, and idle cleanup;
- `internal/ws`: real WebSocket multiplayer flow and transport errors;
- `internal/webui`: embedded asset routing, content types, method handling, and security headers.

Run the full suite with `make test`.

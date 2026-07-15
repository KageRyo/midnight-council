# WebSocket Protocol

The WebSocket protocol carries validated client intent into a server-authoritative room and returns public state plus a private projection for the connected player.

## Connection

```text
ws://localhost:8080/ws/rooms/{room_id}?player_id={player_id}&name={display_name}
```

To reclaim an existing seat:

```text
ws://localhost:8080/ws/rooms/{room_id}?player_id={player_id}&name={display_name}&reconnect_token={token}
```

`room_id`, `player_id`, and `name` must be non-empty. A new player can join only while the room is in `WAITING`. An existing player ID always requires its reconnect token, including when the original connection is still active.

The prototype sends the reconnect token as a query parameter. Deployments must avoid logging raw query strings and should use TLS so WebSocket connections use `wss://`.

## Client Events

Every client event is one text frame containing exactly one JSON object. Unknown JSON fields, multiple JSON values, unknown event types, and fields not accepted by that event are rejected.

| Event | Required fields | Optional fields | Meaning |
| --- | --- | --- | --- |
| `ready` | `ready` boolean | — | Set waiting-room readiness |
| `start_game` | — | — | Owner starts the game |
| `chat` | `message` string | — | Send public chat, max 500 bytes after trimming |
| `night_action` | `target_id` string | — | Submit the player's role-specific action |
| `night_pass` | — | — | Skip a role's night action |
| `start_vote` | — | — | Owner moves discussion to voting |
| `vote` | — | `target_id` string | Vote for a living target; omit or use empty value to abstain |
| `shoot` | `target_id` string | — | Shooter's one-use daytime action |

Examples and the current event schema are in `README.md` and `docs/websocket-client-event.schema.json`.

## Server Envelopes

The server sends one of three envelope types.

### State

```json
{
  "type": "state",
  "state": {
    "room_id": "demo",
    "owner_id": "p1",
    "phase": "NIGHT",
    "phase_started_at": "2026-07-14T09:00:00Z",
    "phase_deadline": "2026-07-14T09:01:30Z",
    "round": 1,
    "players": [],
    "log": [],
    "updated_at": "2026-07-14T09:00:00Z",
    "server_time": "2026-07-14T09:00:10Z"
  },
  "private": {
    "player_id": "p1",
    "reconnect_token": "private-token",
    "role": "DETECTIVE",
    "alive": true,
    "action_required": true,
    "available": ["night_action", "night_pass"],
    "can_vote": false,
    "can_shoot": false,
    "investigations": []
  }
}
```

The `state` object is identical for all subscribers and is safe to broadcast. Roles are omitted from public player views until `FINISHED`. The `private` object is generated for the receiving player and must never be shown to another player.

`available` describes events the UI may offer, but it is not authorization: the server validates the current state again when an event arrives.

`phase_started_at` identifies the current phase transition. `phase_deadline` is present for `NIGHT`, `DAY_DISCUSSION`, and `DAY_VOTING`, and omitted for `WAITING` and `FINISHED`. `server_time` is generated with each snapshot so clients can display the absolute deadline without trusting their local clock. `updated_at` remains the time of the most recent room event and can therefore be older than `server_time` on a new subscription.

### Chat

```json
{
  "type": "chat",
  "chat": {
    "room_id": "demo",
    "player_id": "p1",
    "name": "Player 1",
    "message": "hello",
    "sent_at": "2026-07-14T09:00:00Z"
  }
}
```

Chat envelopes are not retained in room snapshots.

### Error

```json
{
  "type": "error",
  "error": "action is not allowed in the current phase"
}
```

Malformed client events and invalid room actions produce an error envelope for that connection. A failed initial join is followed by connection closure; errors from later game events do not normally close the socket.

### Per-Connection Rate Limits

After JSON and schema validation, each connection applies two independent token buckets before dispatching an event to the room actor:

- `chat` consumes the chat bucket, which defaults to 1 event per second with a burst of 5;
- every other client event consumes the game bucket, which defaults to 5 events per second with a burst of 10.

An exhausted bucket rejects the event only for its sending connection. The event does not mutate room state or produce a chat broadcast. Other connections retain their own capacity, and the chat and game buckets on one connection do not consume each other. Tokens refill continuously, so the same socket can send again later. Reconnecting creates new connection-local buckets.

The initial join performed by the connection handshake is not a client event and does not consume either bucket. A schema-valid client event consumes capacity before room-state authorization, even when the room later rejects it for reasons such as phase or ownership.

The error strings are `chat event rate limit exceeded; retry later` and `game event rate limit exceeded; retry later`. Servers can override the sustained rates and burst capacities with `WS_CHAT_EVENTS_PER_SECOND`, `WS_CHAT_BURST`, `WS_GAME_EVENTS_PER_SECOND`, and `WS_GAME_BURST`; all four values must be positive, and rates must be finite numbers.

## Phase/Event Matrix

| Event | `WAITING` | `NIGHT` | `DAY_DISCUSSION` | `DAY_VOTING` | `FINISHED` |
| --- | ---: | ---: | ---: | ---: | ---: |
| `ready` | yes | no | no | no | no |
| `start_game` | owner | no | no | no | no |
| `chat` | yes | living | living | living | yes |
| `night_action` / `night_pass` | no | eligible living role | no | no | no |
| `start_vote` | no | no | owner | no | no |
| `vote` | no | no | no | living | no |
| `shoot` | no | no | living shooter | living shooter | no |

The state layer remains authoritative for every condition shown in this table.

## Deadline Semantics

Phase expiry is an internal room event and is not an accepted client event. The JSON client-event schema intentionally does not contain `phase_timeout`.

- At the night deadline, every eligible living player without an action receives a pass before normal night resolution.
- At the discussion deadline, the room enters voting as if voting had started without a player initiator.
- At the voting deadline, every living player without a vote receives an abstention before normal vote resolution.
- A manual transition or early completion replaces the old absolute deadline; the actor cancels its previous timer.
- A phase timeout received before its current deadline is rejected by the state layer.

## Public Log Types

State snapshots may contain up to 100 chronological public log entries:

- `game_started`
- `night_started`
- `day_started`
- `night_eliminated`
- `night_no_elimination`
- `voting_started`
- `player_executed`
- `vote_no_execution`
- `shooter_fired`
- `phase_timed_out`
- `game_finished`

Entries contain their type, round, timestamp, and only the relevant phase, player, target, winner, or reason fields. `phase_timed_out` includes the expired `phase`.

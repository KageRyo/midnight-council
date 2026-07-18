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

To join or reclaim a spectator identity, add:

```text
spectator=true
```

`room_id`, `player_id`, and `name` must be non-empty. A new player can join only while the room is in `WAITING`; a new spectator can join in any phase. A locked room rejects either kind of new participant. Reconnects are still allowed through a lock and always require the exact reconnect token plus the same player-versus-spectator type. When that identity already has an active connection, the newer valid connection replaces it; the old socket receives `connection replaced by a newer session` and policy close code `1008`.

The prototype sends the reconnect token as a query parameter. Deployments must avoid logging raw query strings and should use TLS so WebSocket connections use `wss://`.

## Client Events

Every client event is one text frame containing exactly one JSON object. Unknown JSON fields, multiple JSON values, unknown event types, and fields not accepted by that event are rejected. Every event may include an optional positive `sequence` no greater than JavaScript's safe integer maximum (`9007199254740991`). The official browser always includes it.

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
| `transfer_owner` | `target_id` string | — | Owner transfers ownership to a connected seated player |
| `kick_participant` | `target_id` string | — | Owner removes a player or spectator while waiting or after a game |
| `set_room_locked` | `locked` boolean | — | Owner allows or blocks new participants |
| `set_player_limit` | `max_players` integer | — | Owner sets the player cap from 2 through 20 while waiting or after a game |
| `set_game_preset` | `preset` string | — | Owner applies `STANDARD`, `QUICK`, `BEGINNER`, `ADVANCED`, or `MINIMAL` |
| `set_game_settings` | all setting fields | — | Owner replaces the complete custom game configuration |
| `presence` | `afk` boolean | — | Publish the participant's current AFK signal |
| `return_to_waiting` | — | — | Owner resets an active or finished room for a rematch |

Examples and the current event schema are in `README.md` and `docs/websocket-client-event.schema.json`.

## Server Envelopes

The server sends one of four envelope types.

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
    "spectators": [],
    "locked": false,
    "max_players": 12,
    "game_settings": {
      "preset": "STANDARD",
      "night_duration": "1m30s",
      "day_discussion_duration": "5m0s",
      "day_voting_duration": "1m0s",
      "last_words_duration": "30s",
      "minimum_players": 2,
      "reveal_roles_on_death": false,
      "roles": {
        "killers": 1,
        "detectives": 1,
        "doctors": 1,
        "escorts": 0,
        "shooters": 1
      }
    },
    "game_presets": [],
    "log": [],
    "updated_at": "2026-07-14T09:00:00Z",
    "server_time": "2026-07-14T09:00:10Z"
  },
  "private": {
    "player_id": "p1",
    "reconnect_token": "private-token",
    "spectator": false,
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

Player and spectator entries expose `connected` and `afk`. A disconnect clears AFK. Spectators appear only in `state.spectators`, do not consume `max_players`, and receive a private view with `spectator: true`, their reconnect token, and no role or game actions.

`available` describes events the UI may offer, but it is not authorization: the server validates the current state again when an event arrives.

`phase_started_at` identifies the current phase transition. `phase_deadline` is present for `NIGHT`, `DAY_DISCUSSION`, `DAY_VOTING`, and `LAST_WORDS`, and omitted for `WAITING` and `FINISHED`. During `LAST_WORDS`, `last_words_player_id` identifies the executed player who alone may send chat; the field is omitted in every other phase. `server_time` is generated with each snapshot so clients can display the absolute deadline without trusting their local clock. `updated_at` remains the time of the most recent room event and can therefore be older than `server_time` on a new subscription.

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

Chat is available to players and spectators while waiting or finished, and to living players during normal active phases. During `LAST_WORDS`, the executed player identified by `last_words_player_id` is the only permitted speaker; living players, other eliminated players, and spectators can only receive the broadcast.

### Acknowledgement

Sequenced events receive a connection-private acknowledgement:

```json
{
  "type": "ack",
  "ack": {
    "sequence": 42,
    "status": "applied"
  }
}
```

`status` is `applied` after a successful first application, `duplicate` when that participant has already applied the same or a newer sequence, or `rejected` when a validated event fails rate limiting, moderation, or room authorization. A rejected event also receives the normal error envelope. Acknowledgement and broadcast state ordering is not significant; clients correlate by sequence and treat the latest state as authoritative.

### Error

```json
{
  "type": "error",
  "error": "action is not allowed in the current phase"
}
```

Malformed client events and invalid room actions produce an error envelope for that connection. A failed initial join is followed by WebSocket close code `1008`; short public errors are also used as the close reason. Being kicked sends `removed from room by the owner` and closes the socket. A newer valid connection for the same seat similarly sends `connection replaced by a newer session` and closes the older socket. Errors from other later game events do not normally close the socket.

### Reliable replay and disconnect semantics

The server stores the greatest successfully applied client sequence per player or spectator identity. A repeated sequence is acknowledged as `duplicate` without repeating chat, readiness, presence, or game actions. Sequence values are a deduplication cursor, not authentication; the reconnect token still authorizes the seat.

WebSocket transport loss removes the active subscription and publishes the participant with `connected: false`, but preserves its reconnect token and room seat even in `WAITING`. A normal close with code `1000` and reason `player left` is explicit: it removes a waiting-room identity, while active-game identities remain disconnected so match state cannot be erased. The official browser automatically retries transport loss with its stored token and replays unacknowledged events in sequence order.

### Per-Connection Rate Limits

After JSON and schema validation, each connection applies two independent token buckets before dispatching an event to the room actor:

- `chat` consumes the chat bucket, which defaults to 1 event per second with a burst of 5;
- every other client event consumes the game bucket, which defaults to 5 events per second with a burst of 10.

An exhausted bucket rejects the event only for its sending connection. The event does not mutate room state or produce a chat broadcast. Other connections retain their own capacity, and the chat and game buckets on one connection do not consume each other. Tokens refill continuously, so the same socket can send again later. Reconnecting creates new connection-local buckets.

The initial join performed by the connection handshake is not a client event and does not consume either bucket. A schema-valid client event consumes capacity before room-state authorization, even when the room later rejects it for reasons such as phase or ownership.

The error strings are `chat event rate limit exceeded; retry later` and `game event rate limit exceeded; retry later`. Servers can override the sustained rates and burst capacities with `WS_CHAT_EVENTS_PER_SECOND`, `WS_CHAT_BURST`, `WS_GAME_EVENTS_PER_SECOND`, and `WS_GAME_BURST`; all four values must be positive, and rates must be finite numbers.

### Chat Moderation

A chat event that passes its connection rate limit is reviewed before room dispatch. The configured server policy can:

- allow the validated original message;
- reject it, optionally returning a public reason to the sender;
- replace it with server-provided text.

Replacement text is trimmed and must remain non-empty and at most 500 bytes. A rejected message is not broadcast and never reaches the room actor. Policy errors, unknown decisions, and invalid replacements fail closed with `chat moderation unavailable; retry later`; the original message is never used as a fallback. A rejection without a custom reason returns `chat message rejected by moderation`.

The default policy allows every message unchanged. Moderation does not replace normal room authorization: an allowed message can still be rejected by room state, for example when a dead player attempts to chat during an active game.

## Game Settings

The server publishes the current `game_settings` plus the definitions of its available `game_presets`. Presets are authoritative server data; clients apply one by name:

```json
{"type":"set_game_preset","preset":"QUICK"}
```

A custom replacement must provide every field:

```json
{
  "type": "set_game_settings",
  "night_duration": "45s",
  "day_discussion_duration": "2m",
  "day_voting_duration": "45s",
  "last_words_duration": "20s",
  "minimum_players": 4,
  "reveal_roles_on_death": true,
  "killers": 1,
  "detectives": 1,
  "doctors": 1,
  "escorts": 0,
  "shooters": 0
}
```

All four durations use Go duration syntax and must be between one second and one hour. Minimum players must be from 2 through 20 and no greater than `max_players`. The current engine requires exactly one killer; detective, doctor, escort, and shooter counts must each be zero or one. Enabled roles are used in that order while reserving one villager seat, then all remaining players become villagers.

`ESCORT` submits the same `night_action` event as other night roles but cannot target itself. Night resolution follows the role registry's priority: escort block, doctor protection, killer attack, then detective investigation. A blocked player's action is ignored for that night. `ADVANCED` enables one escort and requires six players so every current special role can enter the deck while retaining a villager.

Both setting events are owner-only and accepted only in `WAITING` or `FINISHED`. A successful update resets non-owner readiness and changes the deadlines and role deck used by the next game. Settings persist across `return_to_waiting`. With `reveal_roles_on_death`, dead roles are public during active phases while living roles remain private.

## Room Lifecycle

- The first seated player becomes owner. Ownership can be transferred explicitly to another connected player.
- If an owner disconnects in any phase, another connected seated player becomes owner when available.
- Rooms begin unlocked with a 12-player cap. Owners can choose a cap from 2 through 20, but never below current player occupancy.
- Locking blocks new players and spectators without invalidating existing reconnect tokens.
- Spectators may join any phase when unlocked. They cannot ready, vote, receive a role, perform actions, or chat during active play.
- Kicking is restricted to `WAITING` and `FINISHED`; the owner cannot kick their own seat.
- Network-disconnected identities remain visible and reconnectable; public AFK is independent of transport connection state.
- A vote execution enters timed `LAST_WORDS`; win evaluation waits until its deadline, while a tied or empty vote skips directly to the next night.
- `return_to_waiting` is owner-only and valid from active or finished phases. It removes disconnected identities and clears all game-only state while preserving connected participants, reconnect tokens, room lock, and player cap.

## Phase/Event Matrix

| Event | `WAITING` | `NIGHT` | `DAY_DISCUSSION` | `DAY_VOTING` | `LAST_WORDS` | `FINISHED` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `ready` | yes | no | no | no | no | no |
| `start_game` | owner | no | no | no | no | no |
| `chat` | all participants | living players | living players | living players | executed speaker | all participants |
| `night_action` / `night_pass` | no | eligible living role | no | no | no | no |
| `start_vote` | no | no | owner | no | no | no |
| `vote` | no | no | no | living | no | no |
| `shoot` | no | no | living shooter | living shooter | no | no |
| `transfer_owner` | owner | owner | owner | owner | owner | owner |
| `kick_participant` | owner | no | no | no | no | owner |
| `set_room_locked` | owner | owner | owner | owner | owner | owner |
| `set_player_limit` | owner | no | no | no | no | owner |
| `set_game_preset` | owner | no | no | no | no | owner |
| `set_game_settings` | owner | no | no | no | no | owner |
| `presence` | participant | participant | participant | participant | participant | participant |
| `return_to_waiting` | no | owner | owner | owner | owner | owner |

The state layer remains authoritative for every condition shown in this table.

## Deadline Semantics

Phase expiry is an internal room event and is not an accepted client event. The JSON client-event schema intentionally does not contain `phase_timeout`.

- At the night deadline, every eligible living player without an action receives a pass before normal night resolution.
- At the discussion deadline, the room enters voting as if voting had started without a player initiator.
- At the voting deadline, every living player without a vote receives an abstention before normal vote resolution.
- At the last-words deadline, the speaker is cleared and the server evaluates the delayed win condition before either finishing or starting the next night.
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
- `last_words_started`
- `vote_no_execution`
- `shooter_fired`
- `phase_timed_out`
- `game_finished`

Entries contain their type, round, timestamp, and only the relevant phase, player, target, winner, or reason fields. `phase_timed_out` includes the expired `phase`.

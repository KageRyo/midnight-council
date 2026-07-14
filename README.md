# midnight-council

Midnight Council is a browser-based, real-time social deduction game prototype inspired by classic night-and-day party games.

The Go server and its embedded web client already support a complete playable loop: nickname-based join, room readiness, owner start, random hidden-role assignment, real-time chat, night actions, day discussion, voting, execution, win detection, settlement, and reconnect-token based disconnect/reconnect.

## Current Scope

- Go HTTP/WebSocket game server
- Embedded responsive web client with no frontend build step
- Room actor per room
- Server-authoritative game state
- Public room snapshots plus per-player private state
- Five prototype roles:
  - `VILLAGER`
  - `KILLER`
  - `DETECTIVE`
  - `DOCTOR`
  - `SHOOTER`
- Day/night state machine
- Night actions and pass events
- Day voting and execution
- Shooter one-shot day action
- Win detection and game result reveal
- Reconnect token for existing player seats
- Room idle timeout and hub cleanup
- Public event log for game flow
- Strict WebSocket client event schema validation
- Unit-tested room state rules
- WebSocket integration tests for a full multiplayer game flow
- Browser session recovery through local reconnect-token storage

Not included yet: persistent accounts, JWT auth, PostgreSQL, Redis, server-authoritative phase timers, moderation, rate limiting, or horizontal scaling.

See [`docs/architecture.md`](docs/architecture.md), [`docs/web-client.md`](docs/web-client.md), [`docs/websocket-protocol.md`](docs/websocket-protocol.md), and [`docs/roadmap.md`](docs/roadmap.md) for design details and upcoming work.

## Local Toolchain

This repo uses a project-local conda environment for Go:

```bash
conda activate /mnt/8tb_hdd/ryo/midnight-council/.conda-go
```

Run tests:

```bash
make test
```

Run the server:

```bash
make run
```

The server listens on `:8080` by default. Override with `ADDR`:

```bash
ADDR=:8081 make run
```

Rooms with no active WebSocket subscribers are removed after `30m` by default. Override with `ROOM_IDLE_TIMEOUT`:

```bash
ROOM_IDLE_TIMEOUT=5m make run
```

Open the game client after starting the server:

```text
http://localhost:8080
```

Use separate browser profiles or private windows when testing multiple players on one computer. Tabs in the same browser profile intentionally share the saved seat for a room.

The explicit Go command used by `make test` is:

```bash
CGO_ENABLED=0 GOCACHE=/tmp/go-build GOPATH=/tmp/go ./.conda-go/bin/go test ./...
```

## Browser Client

The root route serves a responsive Traditional Chinese game interface embedded in the Go binary. There is no Node.js install or asset build required to run it.

The client supports:

- room creation and invitation links
- player readiness and owner controls
- private role and investigation displays
- role-aware night actions and passing
- public discussion, voting, abstaining, and shooter actions
- public game log and final role reveal
- reconnect-token recovery after refresh
- desktop and mobile layouts

The browser stores a generated `player_id`, display name, and reconnect token per room in local storage. The reconnect token is never displayed or included in public room state. Chat is live-only and is not replayed after a refresh; authoritative game events remain available in the room log.

See [`docs/web-client.md`](docs/web-client.md) for client behavior, storage, and current limitations.

## WebSocket API

Rooms are created implicitly when the first player connects to a room URL.

Client event format is enforced by Go structs plus `go-playground/validator`.
The mirrored JSON Schema lives at:

```text
docs/websocket-client-event.schema.json
```

Connect:

```text
ws://localhost:8080/ws/rooms/{room_id}?player_id={player_id}&name={display_name}
```

Reconnect to an existing player seat:

```text
ws://localhost:8080/ws/rooms/{room_id}?player_id={player_id}&name={display_name}&reconnect_token={private_token}
```

The server broadcasts envelopes:

```json
{
  "type": "state",
  "state": {
    "room_id": "demo",
    "phase": "NIGHT",
    "round": 1,
    "players": []
  },
  "private": {
    "player_id": "p1",
    "reconnect_token": "private-token",
    "role": "DETECTIVE",
    "alive": true,
    "action_required": true,
    "available": ["night_action", "night_pass"]
  }
}
```

`state` is safe to broadcast to everyone. `private` is generated per subscriber and contains only that player's own hidden information, including the reconnect token needed to reclaim the same seat.

### Client Events

Ready:

```json
{"type":"ready","ready":true}
```

Chat:

```json
{"type":"chat","message":"hello"}
```

Owner starts the game:

```json
{"type":"start_game"}
```

Night action:

```json
{"type":"night_action","target_id":"p2"}
```

Night pass:

```json
{"type":"night_pass"}
```

Owner starts voting after day discussion:

```json
{"type":"start_vote"}
```

Vote or abstain:

```json
{"type":"vote","target_id":"p2"}
```

```json
{"type":"vote"}
```

Shooter day action:

```json
{"type":"shoot","target_id":"p2"}
```

## Game Flow

1. Players connect to the same room URL.
2. Non-owner players send `ready`.
3. The owner sends `start_game`.
4. The server shuffles roles and enters `NIGHT`.
5. Alive `KILLER`, `DETECTIVE`, and `DOCTOR` players submit `night_action` or `night_pass`.
6. The server resolves protection, kills, and detective results.
7. If no side has won, the room enters `DAY_DISCUSSION`.
8. The owner sends `start_vote`.
9. Alive players vote. When all living players have voted, the server resolves execution.
10. The server either finishes the game or starts the next night.

Win rules:

- Villagers win when all killers are dead.
- Killers win when living killers are greater than or equal to living non-killers.

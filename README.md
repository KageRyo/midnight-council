# midnight-council

A real-time social deduction game server prototype.

## Current Scope

- Go WebSocket game server
- Room actor per room
- Join, leave, ready, start game, and chat events
- Public room state broadcasts
- Unit-tested room state rules

The first playable slice intentionally does not include roles, night actions, voting, auth, or persistence yet. Those belong in the next increments once the room event core is stable.

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

The explicit Go command used by `make test` is:

```bash
CGO_ENABLED=0 GOCACHE=/tmp/go-build GOPATH=/tmp/go ./.conda-go/bin/go test ./...
```

## Git Workflow

- Use Conventional Commits for commit messages.
- Split unrelated work into separate commits.
- Run `make test` before every push.
- Do not push while unit tests are failing.

## WebSocket API

Connect:

```text
ws://localhost:8080/ws/rooms/{room_id}?player_id={player_id}&name={display_name}
```

Client event examples:

```json
{"type":"ready","ready":true}
```

```json
{"type":"chat","message":"hello"}
```

```json
{"type":"start_game"}
```

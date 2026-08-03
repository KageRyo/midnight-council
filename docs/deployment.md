# Deployment and Operations

## Run locally or in a container

The quickest start requires only Docker Compose:

```bash
docker compose up --build
```

Open `http://localhost:8080`. Copy [`.env.example`](../.env.example) to `.env` only when you need to change a default.

For native development, install Go `1.26.4` or newer and run:

```bash
make run
```

## Public deployment baseline

Put the service behind an HTTPS reverse proxy and terminate TLS there. The browser client will select `wss:` automatically when it is loaded over HTTPS.

Set `ALLOWED_ORIGINS` to the exact comma-separated web origins that may open a WebSocket:

```text
ALLOWED_ORIGINS=https://game.example.com,https://staging.game.example.com
```

When `ALLOWED_ORIGINS` is empty, the server permits only same-host browser origins. Requests with no `Origin` header are permitted for non-browser tooling. Wildcards are intentionally unsupported.

Keep the reverse proxy's request/body limits, connection limits, and IP-based rate limits enabled. The application uses the direct peer address for its per-IP controls and deliberately does not trust `X-Forwarded-For`; configure the proxy so the application is reachable only through it.

## Resource controls

| Variable | Default | Purpose |
| --- | ---: | --- |
| `MAX_ROOMS` | `1000` | Maximum in-memory room actors in one process |
| `MAX_SPECTATORS_PER_ROOM` | `24` | Maximum spectator identities in one room |
| `WS_CONNECTIONS_PER_SECOND` | `2` | Per-IP sustained WebSocket admission rate |
| `WS_CONNECTION_BURST` | `10` | Per-IP immediate connection burst |
| `WS_MAX_CONNECTIONS_PER_IP` | `20` | Simultaneous WebSockets accepted from one direct IP |
| `WS_ROOM_CREATIONS_PER_SECOND` | `0.2` | Per-IP sustained new-room rate |
| `WS_ROOM_CREATION_BURST` | `5` | Per-IP immediate new-room burst |

These limits protect one process. Distributed deployment and account-scoped protection remain future work.

## Load-test harness

`cmd/loadtest` is a Go WebSocket client that creates rooms, maintains real connections, and optionally sends one presence and chat event per connection. It reports connection success, failures, and p50/p95 dial latency.

For a local 100-room × 10-player run, start the server with matching test limits in one terminal:

```bash
MAX_ROOMS=120 \
WS_MAX_CONNECTIONS_PER_IP=1200 \
WS_CONNECTIONS_PER_SECOND=1000 WS_CONNECTION_BURST=1200 \
WS_ROOM_CREATIONS_PER_SECOND=100 WS_ROOM_CREATION_BURST=120 \
make run
```

Then run the client in another terminal:

```bash
go run ./cmd/loadtest -rooms 100 -players 10 -workers 100 -hold 30s
```

Record the machine, server settings, p95 connection latency, CPU, memory, and successful reconnect rate with each benchmark. Do not treat a local run as a production capacity guarantee.

The first reproducible 100 × 10 local baseline is recorded in [`benchmarks.md`](benchmarks.md).

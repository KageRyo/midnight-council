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

The server uses `ADDR` when it is set. Otherwise it listens on `:${PORT}` when `PORT` is present, falling back to `:8080`. Leave `ADDR` unset on a PaaS so the platform-provided `PORT` is used; `PORT` must be a number from `1` through `65535`.

Set `ALLOWED_ORIGINS` to the exact comma-separated web origins that may open a WebSocket:

```text
ALLOWED_ORIGINS=https://game.example.com,https://staging.game.example.com
```

When `ALLOWED_ORIGINS` is empty, the server permits only same-host browser origins. Requests with no `Origin` header are permitted for non-browser tooling. Wildcards are intentionally unsupported.

### Client IPs behind a reverse proxy

The connection and room-creation limits use the direct network peer by default. To recover the real client address behind a trusted proxy, set `TRUSTED_PROXIES` to a comma-separated list of the proxy's exact IP addresses or CIDR networks:

```text
TRUSTED_PROXIES=10.0.0.0/8,2001:db8:1234::/48
```

The server reads `X-Forwarded-For` (or `Forwarded`) only when the direct peer matches one of those entries. It walks the proxy chain from right to left and uses the first address that is not in a trusted network. A request from an untrusted peer cannot change its admission identity by sending a forwarded-address header. Configure the edge proxy to replace or sanitize these headers and restrict direct access to the application whenever possible.

Do not set `TRUSTED_PROXIES` to `0.0.0.0/0` or `::/0` simply to make a PaaS deployment work. Use the platform's documented ingress CIDRs. If the platform does not publish stable proxy ranges, leave this setting empty and enforce client-IP limits at the platform edge instead; otherwise one shared proxy address may be used for the application-level limits.

Keep the reverse proxy's request/body limits, WebSocket connection limits, and IP-based rate limits enabled. A single process owns all rooms in memory, so run one application instance until shared room state is added.

## Container PaaS deployment

This runbook uses [Render's Docker web service](https://render.com/docs/docker) as a concrete example. It also applies to another container PaaS that builds the root `Dockerfile`, injects `PORT`, supports WebSockets, and provides an HTTPS public URL.

### Create the service

1. Create a **Web Service** from the GitHub repository. Select **Docker** as the runtime and use the root `Dockerfile`.
2. Leave the build command and Docker start command empty. The image already contains the build stage and an `ENTRYPOINT`.
3. Configure the HTTP health check path as `/healthz`.
4. Start with one instance. Rooms are process-local and are lost when the process restarts; more than one instance cannot share a room.
5. Add the environment variables below in the platform dashboard:

   ```text
   # Injected by the platform; do not set ADDR here.
   # PORT=<platform-provided value>
   ALLOWED_ORIGINS=https://<your-service-host>
   TRUSTED_PROXIES=<documented-platform-ingress-CIDRs>
   ```

   Replace `<your-service-host>` with the generated service hostname or custom domain, including the `https://` scheme and no trailing slash. Set `TRUSTED_PROXIES` only to the platform's documented ingress addresses; keep it empty if no stable list is available.

### Verify the deployment

After the first deploy, verify the health endpoint:

```bash
curl -fsS https://<your-service-host>/healthz
```

The response should be `ok`. Open the same HTTPS URL in a browser, create a room, and confirm that the browser connects over `wss:`. For a custom domain, update `ALLOWED_ORIGINS` to the custom origin before sharing the link.

Container PaaS proxies may close WebSockets during deploys, restarts, maintenance, or scale changes. The client retries and reconnects, but an application restart still discards every in-memory room. Use the platform's edge rate limiting and monitoring in addition to the server's process-local admission limits. See the platform's [health-check](https://render.com/docs/health-checks) and [WebSocket](https://render.com/docs/websocket) guidance for the provider-specific settings.

## Resource controls

| Variable | Default | Purpose |
| --- | ---: | --- |
| `ADDR` | unset | Explicit listen address; overrides `PORT` when set |
| `PORT` | `8080` | PaaS-provided listen port when `ADDR` is unset |
| `TRUSTED_PROXIES` | empty | IP/CIDR list allowed to provide forwarded client addresses |
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

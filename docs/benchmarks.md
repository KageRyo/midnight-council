# Benchmark Baseline

This is a reproducible local smoke benchmark, not a production capacity claim. Repeat it after material transport, actor, or deployment changes and include system-level CPU and memory collection before making capacity decisions.

## 2026-08-03: 100 rooms × 10 players

| Metric | Result |
| --- | ---: |
| Successful WebSocket connections | 1,000 / 1,000 |
| Failed connections | 0 |
| Connection p50 | 5.62 ms |
| Connection p95 | 111.72 ms |
| Concurrent rooms | 100 |
| Clients per room | 10 |
| Connection workers | 100 |
| Steady hold | 5 s |

The harness sent one `presence` event and one `chat` event from each connected client. The reported latency measures the WebSocket dial/upgrade path, not end-to-end game-event latency.

### Environment

- Linux `x86_64`, Intel Xeon W-2255 (20 logical CPUs), 125 GiB memory (87 GiB available at collection).
- Go `1.26.4`.
- Local server on `127.0.0.1:18081`.
- Server settings: `MAX_ROOMS=120`, `WS_MAX_CONNECTIONS_PER_IP=1200`, `WS_CONNECTIONS_PER_SECOND=1000`, `WS_CONNECTION_BURST=1200`, `WS_ROOM_CREATIONS_PER_SECOND=100`, `WS_ROOM_CREATION_BURST=120`.
- Command: `go run ./cmd/loadtest -target ws://127.0.0.1:18081 -rooms 100 -players 10 -workers 100 -hold 5s`.

The raised admission limits are deliberately test-specific. The container defaults remain defensive for public deployment and should be paired with reverse-proxy limits.

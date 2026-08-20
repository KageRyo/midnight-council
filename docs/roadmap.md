# Roadmap

Midnight Council is focused on turning its playable full-stack prototype into a paced, abuse-resistant game that can be deployed for real groups.

## Completed

- Room creation by first WebSocket connection
- Nickname-based player join
- Player readiness
- Owner-only game start
- Random role assignment
- Public/private state projection
- Reconnect token for existing player seats
- Real-time chat
- Night actions for killer, detective, and doctor
- Day discussion and owner-started voting
- Vote execution
- Shooter one-shot day action
- Win detection and settlement
- Strict client event schema validation
- Room idle timeout and hub cleanup
- Unit tests and WebSocket integration tests
- Embedded responsive browser client
- Browser reconnect-token recovery
- Desktop and mobile browser-flow verification
- Server-authoritative phase deadlines and automatic progression
- Server-synchronized browser countdown
- Per-connection rate limiting for chat and game events
- Pluggable chat moderation policy with a default allow implementation
- Same-room rematch and return-to-waiting reset
- Owner transfer, automatic owner succession, and participant kick
- Room lock and configurable player cap
- Spectators with private reconnect credentials
- Room-owned phase durations, minimum players, death-role reveal, and role pool
- Server-authoritative standard, quick, beginner, advanced, and minimal presets
- Server validation for duration, capacity, role legality, and starting balance
- Automatic browser reconnect with capped exponential backoff
- One active WebSocket per player or spectator identity
- Sequenced client events, acknowledgements, pending replay, and server deduplication
- Public disconnected and AFK participant signals
- Registry-backed role capabilities and ordered night-action resolution
- Escort role with action blocking and an advanced preset
- Server-authoritative last-words phase with speaker-only timed chat
- Portable Dockerfile, Docker Compose configuration, and environment example
- GitHub Actions checks for formatting, vet, tests, race detection, JavaScript syntax, and browser E2E
- Same-host WebSocket origin policy with an exact deployment allowlist
- Server-side room ID and participant identity limits
- Per-IP connection admission, per-IP room-creation limits, global room cap, and spectator cap
- Playwright four-browser multiplayer, reconnect, settlement, and rematch flow
- Configurable Go WebSocket load-test harness
- Trusted reverse-proxy client IP detection with explicit proxy networks
- PaaS-provided `PORT` listen address support
- Public container PaaS deployment runbook
- Experimental project status, contributor guidance, security policy, and community health files
- Dependency integrity, `govulncheck`, and container vulnerability checks in CI
- Apache-2.0 source license, attribution notice, trademark policy, and release naming convention

## Next Tasks

1. Finalize the publication scope and repository visibility before making the repository public.
2. Run the load harness in a controlled environment and publish reproducible latency, memory, CPU, and reconnect measurements.
3. Add a registry-backed alternate win mode or another role with a distinct faction interaction.
4. Add deployment monitoring, structured request metrics, and alerting.
5. Add JWT authentication for persistent accounts.
6. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
7. Add report, mute, and ban workflows backed by moderation policy.
8. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The playable prototype now has a reproducible local container path, CI evidence, browser-flow coverage, bounded WebSocket admission, trusted proxy handling, a public container PaaS runbook, an Apache-2.0 source license, and initial open-source readiness checks. The immediate next step is to finalize the publication boundary and gather controlled single-instance deployment evidence before considering a public service or multi-process scaling.

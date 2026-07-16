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

## Next Tasks

1. Add configurable game settings with server-validated role balance and presets.
2. Improve connection reliability with automatic reconnect, one active connection per seat, event sequencing, deduplication, and AFK signals.
3. Refactor roles into an extensible rule system and add more roles, ordered night actions, last words, or alternate modes.
4. Add Docker Compose for local deployment.
5. Add GitHub Actions for `make test`.
6. Add a load test script for WebSocket rooms.
7. Add JWT authentication for persistent accounts.
8. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
9. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority is configurable room-owned game settings. The full room lifecycle now supports repeated play and administration; the next step is to make phase timing, role composition, identity reveal, minimum players, and validated rule presets part of each room rather than process-wide or hard-coded behavior.

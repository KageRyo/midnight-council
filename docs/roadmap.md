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
- Server-authoritative standard, quick, beginner, and minimal presets
- Server validation for duration, capacity, role legality, and starting balance
- Automatic browser reconnect with capped exponential backoff
- One active WebSocket per player or spectator identity
- Sequenced client events, acknowledgements, pending replay, and server deduplication
- Public disconnected and AFK participant signals

## Next Tasks

1. Refactor roles into an extensible rule system and add more roles, ordered night actions, last words, or alternate modes.
2. Add Docker Compose for local deployment.
3. Add GitHub Actions for `make test`.
4. Add a load test script for WebSocket rooms.
5. Add JWT authentication for persistent accounts.
6. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
7. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority is the extensible game-rule layer. Connection loss is now recoverable and sequenced; the next requirement is to separate role definitions, night-action ordering, resolution, and win rules so new roles and modes do not keep expanding one monolithic state switch.

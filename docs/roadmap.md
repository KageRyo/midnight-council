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

## Next Tasks

1. Add a last-words phase and continue expanding roles or alternate modes on the rule registry.
2. Add Docker Compose for local deployment.
3. Add GitHub Actions for `make test`.
4. Add a load test script for WebSocket rooms.
5. Add JWT authentication for persistent accounts.
6. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
7. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The role registry now centralizes faction, night ability, action priority, target policy, day ability, deck construction, investigation alignment, and win counting. The current priority is a server-authoritative last-words phase after execution, followed by additional registry-backed roles or alternate win modes.

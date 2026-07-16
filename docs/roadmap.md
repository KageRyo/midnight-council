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

## Next Tasks

1. Complete the room lifecycle: rematch, return to waiting, owner transfer, kick, lock, player cap, and spectators.
2. Add configurable game settings with server-validated role balance and presets.
3. Improve connection reliability with automatic reconnect, one active connection per seat, event sequencing, deduplication, and AFK signals.
4. Refactor roles into an extensible rule system and add more roles, ordered night actions, last words, or alternate modes.
5. Add Docker Compose for local deployment.
6. Add GitHub Actions for `make test`.
7. Add a load test script for WebSocket rooms.
8. Add JWT authentication for persistent accounts.
9. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
10. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority is the full room lifecycle. Rate limiting and the moderation hook now protect the chat path; rematch and room administration are the next requirements for repeated play without creating a new room after every game.

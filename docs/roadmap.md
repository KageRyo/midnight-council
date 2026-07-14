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

## Next Tasks

1. Add server-authoritative phase deadlines and automatic progression.
2. Add per-connection rate limiting for chat and game events.
3. Add a chat moderation hook with a default allow implementation.
4. Add Docker Compose for local deployment.
5. Add GitHub Actions for `make test`.
6. Add a load test script for WebSocket rooms.
7. Add JWT authentication for persistent accounts.
8. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
9. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority is server-authoritative timing. The browser now exposes the complete game loop, and phase deadlines are the next requirement for consistent pacing without relying on every actor to respond immediately or on the owner to advance discussion manually.

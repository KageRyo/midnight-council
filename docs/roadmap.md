# Roadmap

Midnight Council is currently focused on a clean, playable real-time backend prototype before adding persistence, scaling, or frontend work.

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

## Next Tasks

1. Add per-connection rate limiting for chat and game events.
2. Add a chat moderation hook with a default allow implementation.
3. Add Docker Compose for local deployment.
4. Add GitHub Actions for `make test`.
5. Expand architecture and security docs.
6. Add a load test script for WebSocket rooms.
7. Add JWT authentication for persistent accounts.
8. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
9. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority is production-readiness around server lifecycle and abuse resistance. Room idle timeout was implemented first because empty room actors would otherwise stay alive for the lifetime of the process.

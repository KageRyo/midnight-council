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

## Next Tasks

1. Add a chat moderation hook with a default allow implementation.
2. Add Docker Compose for local deployment.
3. Add GitHub Actions for `make test`.
4. Add a load test script for WebSocket rooms.
5. Add JWT authentication for persistent accounts.
6. Add PostgreSQL persistence for accounts, game summaries, and audit logs.
7. Add Redis Pub/Sub support for future multi-instance room routing.

## Current Priority

The current priority remains abuse resistance. Per-connection rate limiting now protects room actors from event floods; a chat moderation hook is next so deployments have a clear policy boundary before broadcasting player text.

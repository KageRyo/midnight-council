package ws

import (
	"fmt"
	"math"
	"time"

	"midnight-council/internal/room"
)

const (
	DefaultChatEventsPerSecond float64 = 1
	DefaultChatBurst           int     = 5
	DefaultGameEventsPerSecond float64 = 5
	DefaultGameBurst           int     = 10

	chatRateLimitError = "chat event rate limit exceeded; retry later"
	gameRateLimitError = "game event rate limit exceeded; retry later"
)

type RateLimit struct {
	EventsPerSecond float64
	Burst           int
}

type EventRateLimits struct {
	Chat RateLimit
	Game RateLimit
}

func DefaultEventRateLimits() EventRateLimits {
	return EventRateLimits{
		Chat: RateLimit{
			EventsPerSecond: DefaultChatEventsPerSecond,
			Burst:           DefaultChatBurst,
		},
		Game: RateLimit{
			EventsPerSecond: DefaultGameEventsPerSecond,
			Burst:           DefaultGameBurst,
		},
	}
}

func (l EventRateLimits) Validate() error {
	if err := l.Chat.validate("chat"); err != nil {
		return err
	}
	return l.Game.validate("game")
}

func (l RateLimit) validate(category string) error {
	if l.EventsPerSecond <= 0 || math.IsNaN(l.EventsPerSecond) || math.IsInf(l.EventsPerSecond, 0) {
		return fmt.Errorf("%s events per second must be finite and positive", category)
	}
	if l.Burst <= 0 {
		return fmt.Errorf("%s burst must be positive", category)
	}
	return nil
}

type tokenBucket struct {
	eventsPerSecond float64
	capacity        float64
	tokens          float64
	lastRefill      time.Time
}

func newTokenBucket(limit RateLimit, now time.Time) tokenBucket {
	capacity := float64(limit.Burst)
	return tokenBucket{
		eventsPerSecond: limit.EventsPerSecond,
		capacity:        capacity,
		tokens:          capacity,
		lastRefill:      now,
	}
}

func (b *tokenBucket) allow(now time.Time) bool {
	if now.Before(b.lastRefill) {
		now = b.lastRefill
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(b.capacity, b.tokens+elapsed*b.eventsPerSecond)
	b.lastRefill = now
	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}

type connectionRateLimiter struct {
	chat tokenBucket
	game tokenBucket
}

func newConnectionRateLimiter(limits EventRateLimits, now time.Time) *connectionRateLimiter {
	return &connectionRateLimiter{
		chat: newTokenBucket(limits.Chat, now),
		game: newTokenBucket(limits.Game, now),
	}
}

func (l *connectionRateLimiter) allow(eventType room.EventType, now time.Time) bool {
	if eventType == room.EventChat {
		return l.chat.allow(now)
	}
	return l.game.allow(now)
}

func rateLimitError(eventType room.EventType) string {
	if eventType == room.EventChat {
		return chatRateLimitError
	}
	return gameRateLimitError
}

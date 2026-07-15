package ws

import (
	"math"
	"strings"
	"testing"
	"time"

	"midnight-council/internal/room"
)

func TestEventRateLimitsValidate(t *testing.T) {
	valid := DefaultEventRateLimits()
	tests := []struct {
		name      string
		limits    EventRateLimits
		wantError string
	}{
		{name: "valid", limits: valid},
		{
			name: "chat rate must be positive",
			limits: EventRateLimits{
				Chat: RateLimit{Burst: 1},
				Game: valid.Game,
			},
			wantError: "chat events per second",
		},
		{
			name: "chat rate must be finite",
			limits: EventRateLimits{
				Chat: RateLimit{EventsPerSecond: math.Inf(1), Burst: 1},
				Game: valid.Game,
			},
			wantError: "chat events per second",
		},
		{
			name: "chat rate must not be NaN",
			limits: EventRateLimits{
				Chat: RateLimit{EventsPerSecond: math.NaN(), Burst: 1},
				Game: valid.Game,
			},
			wantError: "chat events per second",
		},
		{
			name: "chat burst must be positive",
			limits: EventRateLimits{
				Chat: RateLimit{EventsPerSecond: 1},
				Game: valid.Game,
			},
			wantError: "chat burst",
		},
		{
			name: "game rate must be positive",
			limits: EventRateLimits{
				Chat: valid.Chat,
				Game: RateLimit{Burst: 1},
			},
			wantError: "game events per second",
		},
		{
			name: "game burst must be positive",
			limits: EventRateLimits{
				Chat: valid.Chat,
				Game: RateLimit{EventsPerSecond: 1},
			},
			wantError: "game burst",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.limits.Validate()
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("err = %v, want error containing %q", err, test.wantError)
			}
		})
	}
}

func TestTokenBucketAllowsBurstRefillsAndCapsCapacity(t *testing.T) {
	startedAt := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	bucket := newTokenBucket(RateLimit{EventsPerSecond: 2, Burst: 2}, startedAt)

	if !bucket.allow(startedAt) || !bucket.allow(startedAt) {
		t.Fatal("initial burst should be allowed")
	}
	if bucket.allow(startedAt) {
		t.Fatal("event beyond initial burst should be rejected")
	}
	if bucket.allow(startedAt.Add(499 * time.Millisecond)) {
		t.Fatal("bucket should not allow a partial token")
	}
	if !bucket.allow(startedAt.Add(501 * time.Millisecond)) {
		t.Fatal("bucket should refill one token after slightly more than 500ms at 2 events/s")
	}

	refilledAt := startedAt.Add(10 * time.Second)
	if !bucket.allow(refilledAt) || !bucket.allow(refilledAt) {
		t.Fatal("bucket should refill to burst capacity")
	}
	if bucket.allow(refilledAt) {
		t.Fatal("refill should not exceed burst capacity")
	}
}

func TestConnectionRateLimiterKeepsChatAndGameBucketsIndependent(t *testing.T) {
	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	limits := EventRateLimits{
		Chat: RateLimit{EventsPerSecond: 1, Burst: 1},
		Game: RateLimit{EventsPerSecond: 1, Burst: 1},
	}
	limiter := newConnectionRateLimiter(limits, now)

	if !limiter.allow(room.EventChat, now) {
		t.Fatal("first chat event should be allowed")
	}
	if limiter.allow(room.EventChat, now) {
		t.Fatal("second chat event should be limited")
	}
	if !limiter.allow(room.EventReady, now) {
		t.Fatal("chat exhaustion should not consume the game bucket")
	}
	if limiter.allow(room.EventStartGame, now) {
		t.Fatal("all non-chat events should share the game bucket")
	}
}

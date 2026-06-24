package room

import (
	"context"
	"testing"
	"time"
)

func TestHubRemovesIdleRoomAfterTimeout(t *testing.T) {
	hub := NewHub(WithRoomIdleTimeout(20 * time.Millisecond))
	actor := hub.GetOrCreate("room-1")

	events, unsubscribe, err := actor.Subscribe(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	<-events

	unsubscribe()
	eventually(t, 250*time.Millisecond, func() bool {
		return hub.RoomCount() == 0
	})

	recreated := hub.GetOrCreate("room-1")
	if recreated == actor {
		t.Fatal("expected idle room to be recreated with a new actor")
	}
}

func TestHubKeepsRoomWithActiveSubscriber(t *testing.T) {
	hub := NewHub(WithRoomIdleTimeout(20 * time.Millisecond))
	actor := hub.GetOrCreate("room-1")

	events, unsubscribe, err := actor.Subscribe(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()
	<-events

	time.Sleep(60 * time.Millisecond)
	if got := hub.RoomCount(); got != 1 {
		t.Fatalf("room count = %d, want 1", got)
	}
}

func TestHubReplacesClosedActor(t *testing.T) {
	hub := NewHub(WithRoomIdleTimeout(0))
	actor := hub.GetOrCreate("room-1")
	actor.close()

	recreated := hub.GetOrCreate("room-1")
	if recreated == actor {
		t.Fatal("expected closed actor to be replaced")
	}
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatal("condition was not met before timeout")
}

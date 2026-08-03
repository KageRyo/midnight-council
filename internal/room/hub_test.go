package room

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHubRemovesIdleRoomAfterTimeout(t *testing.T) {
	hub := NewHub(WithRoomIdleTimeout(20 * time.Millisecond))
	actor, err := hub.GetOrCreate("room-1")
	if err != nil {
		t.Fatalf("get or create room: %v", err)
	}

	events, unsubscribe, err := actor.Subscribe(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	<-events

	unsubscribe()
	eventually(t, 250*time.Millisecond, func() bool {
		return hub.RoomCount() == 0
	})

	recreated, err := hub.GetOrCreate("room-1")
	if err != nil {
		t.Fatalf("recreate room: %v", err)
	}
	if recreated == actor {
		t.Fatal("expected idle room to be recreated with a new actor")
	}
}

func TestHubKeepsRoomWithActiveSubscriber(t *testing.T) {
	hub := NewHub(WithRoomIdleTimeout(20 * time.Millisecond))
	actor, err := hub.GetOrCreate("room-1")
	if err != nil {
		t.Fatalf("get or create room: %v", err)
	}

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
	actor, err := hub.GetOrCreate("room-1")
	if err != nil {
		t.Fatalf("get or create room: %v", err)
	}
	actor.close()

	recreated, err := hub.GetOrCreate("room-1")
	if err != nil {
		t.Fatalf("recreate room: %v", err)
	}
	if recreated == actor {
		t.Fatal("expected closed actor to be replaced")
	}
}

func TestHubRejectsNewRoomWhenAtCapacity(t *testing.T) {
	hub := NewHub(WithMaxRooms(1))
	if _, err := hub.GetOrCreate("first"); err != nil {
		t.Fatalf("create first room: %v", err)
	}
	if _, err := hub.GetOrCreate("second"); !errors.Is(err, ErrRoomLimitReached) {
		t.Fatalf("second room error = %v, want %v", err, ErrRoomLimitReached)
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

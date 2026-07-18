package room

import (
	"context"
	"testing"
	"time"
)

func TestActorAutomaticallyAdvancesExpiredPhases(t *testing.T) {
	durations := PhaseDurations{
		Night:         40 * time.Millisecond,
		DayDiscussion: 40 * time.Millisecond,
		DayVoting:     40 * time.Millisecond,
		LastWords:     40 * time.Millisecond,
	}
	actor := newActor("timed-room", 15*time.Millisecond, durations, nil)
	ctx := context.Background()

	joinReadyPlayers(t, ctx, actor, []string{"owner", "guest-1", "guest-2"})
	events, unsubscribe, err := actor.Subscribe(ctx, "owner")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if _, err := actor.Dispatch(ctx, Event{Type: EventStartGame, PlayerID: "owner"}); err != nil {
		t.Fatalf("start game: %v", err)
	}

	night := readActorStateUntil(t, events, func(state *Snapshot) bool {
		return state.Phase == PhaseNight && state.Round == 1
	})
	if night.PhaseDeadline == nil {
		t.Fatal("night snapshot has no deadline")
	}

	readActorStateUntil(t, events, func(state *Snapshot) bool {
		return state.Phase == PhaseDayDiscussion
	})
	readActorStateUntil(t, events, func(state *Snapshot) bool {
		return state.Phase == PhaseDayVoting
	})
	readActorStateUntil(t, events, func(state *Snapshot) bool {
		return state.Phase == PhaseNight && state.Round == 2
	})

	unsubscribe()
	eventually(t, 500*time.Millisecond, actor.Closed)
	time.Sleep(60 * time.Millisecond)
	if !actor.Closed() {
		t.Fatal("actor reopened after idle close")
	}
}

func TestActorCancelsPreviousPhaseTimerAfterManualTransition(t *testing.T) {
	durations := PhaseDurations{
		Night:         20 * time.Millisecond,
		DayDiscussion: 80 * time.Millisecond,
		DayVoting:     500 * time.Millisecond,
		LastWords:     500 * time.Millisecond,
	}
	actor := newActor("timer-reset-room", 15*time.Millisecond, durations, nil)
	ctx := context.Background()
	joinReadyPlayers(t, ctx, actor, []string{"owner", "guest-1", "guest-2"})
	events, unsubscribe, err := actor.Subscribe(ctx, "owner")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if _, err := actor.Dispatch(ctx, Event{Type: EventStartGame, PlayerID: "owner"}); err != nil {
		t.Fatalf("start game: %v", err)
	}
	readActorStateUntil(t, events, func(state *Snapshot) bool {
		return state.Phase == PhaseDayDiscussion
	})

	if _, err := actor.Dispatch(ctx, Event{Type: EventStartVote, PlayerID: "owner"}); err != nil {
		t.Fatalf("start vote: %v", err)
	}
	time.Sleep(140 * time.Millisecond)

	probe, stopProbe, err := actor.Subscribe(ctx, "owner")
	if err != nil {
		t.Fatalf("probe subscription: %v", err)
	}
	snapshot := <-probe
	stopProbe()
	if snapshot.State.Phase != PhaseDayVoting {
		t.Fatalf("phase = %s, want %s; previous discussion timer was not canceled", snapshot.State.Phase, PhaseDayVoting)
	}

	unsubscribe()
	eventually(t, 500*time.Millisecond, actor.Closed)
}

func TestActorDisconnectsKickedParticipantSubscription(t *testing.T) {
	actor := newActor("kick-room", 50*time.Millisecond, DefaultPhaseDurations(), nil)
	ctx := context.Background()
	for _, playerID := range []string{"owner", "guest"} {
		if _, err := actor.Dispatch(ctx, Event{Type: EventJoin, PlayerID: playerID, PlayerName: playerID}); err != nil {
			t.Fatalf("join %s: %v", playerID, err)
		}
	}

	guestEvents, unsubscribeGuest, err := actor.Subscribe(ctx, "guest")
	if err != nil {
		t.Fatalf("subscribe guest: %v", err)
	}
	defer unsubscribeGuest()
	<-guestEvents

	for i := 0; i < 15; i++ {
		if _, err := actor.Dispatch(ctx, Event{
			Type:     EventSetRoomLocked,
			PlayerID: "owner",
			Locked:   i%2 == 0,
		}); err != nil {
			t.Fatalf("fill guest event buffer: %v", err)
		}
	}

	if _, err := actor.Dispatch(ctx, Event{
		Type:     EventKickParticipant,
		PlayerID: "owner",
		TargetID: "guest",
	}); err != nil {
		t.Fatalf("kick guest: %v", err)
	}

	envelope, ok := <-guestEvents
	if !ok {
		t.Fatal("kicked participant channel closed before terminal error")
	}
	if envelope.Type != "error" || envelope.Error != ErrKicked.Error() {
		t.Fatalf("kicked participant envelope = %#v", envelope)
	}
	if _, ok := <-guestEvents; ok {
		t.Fatal("kicked participant subscription remained open")
	}

	eventually(t, 500*time.Millisecond, actor.Closed)
}

func joinReadyPlayers(t *testing.T, ctx context.Context, actor *Actor, playerIDs []string) {
	t.Helper()
	for _, playerID := range playerIDs {
		if _, err := actor.Dispatch(ctx, Event{Type: EventJoin, PlayerID: playerID, PlayerName: playerID}); err != nil {
			t.Fatalf("join %s: %v", playerID, err)
		}
		if playerID != "owner" {
			if _, err := actor.Dispatch(ctx, Event{Type: EventReady, PlayerID: playerID, Ready: true}); err != nil {
				t.Fatalf("ready %s: %v", playerID, err)
			}
		}
	}
}

func readActorStateUntil(t *testing.T, events <-chan Envelope, accept func(*Snapshot) bool) *Snapshot {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope, ok := <-events:
			if !ok {
				t.Fatal("subscription closed before expected state")
			}
			if envelope.Type == "state" && envelope.State != nil && accept(envelope.State) {
				return envelope.State
			}
		case <-timer.C:
			t.Fatal("timed out waiting for actor state")
		}
	}
}

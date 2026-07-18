package room

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNetworkDisconnectPreservesWaitingSeatForReconnect(t *testing.T) {
	state := NewState("disconnect-room")
	joined, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "owner",
		PlayerName: "Owner",
	})
	if err != nil {
		t.Fatalf("join owner: %v", err)
	}
	token := state.PrivateView("owner").ReconnectToken
	if token == "" || joined.State == nil {
		t.Fatal("join did not create a reconnectable seat")
	}

	disconnected, err := state.Apply(Event{Type: EventDisconnect, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("disconnect owner: %v", err)
	}
	if len(disconnected.State.Players) != 1 || disconnected.State.Players[0].Connected {
		t.Fatalf("disconnected players = %#v, want retained offline seat", disconnected.State.Players)
	}

	reconnected, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "owner",
		PlayerName:     "Owner Again",
		ReconnectToken: token,
	})
	if err != nil {
		t.Fatalf("reconnect owner: %v", err)
	}
	if len(reconnected.State.Players) != 1 || !reconnected.State.Players[0].Connected {
		t.Fatalf("reconnected players = %#v", reconnected.State.Players)
	}

	left, err := state.Apply(Event{Type: EventLeave, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("explicit leave: %v", err)
	}
	if len(left.State.Players) != 0 {
		t.Fatalf("explicit waiting-room leave retained players: %#v", left.State.Players)
	}
}

func TestClientSequenceDeduplicatesAppliedEventsAndPresenceIsPublic(t *testing.T) {
	state := readyRoom(t)

	if _, err := state.Apply(Event{
		Type:           EventReady,
		PlayerID:       "guest",
		Ready:          true,
		ClientSequence: 1,
	}); err != nil {
		t.Fatalf("apply sequenced ready: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:           EventReady,
		PlayerID:       "guest",
		Ready:          false,
		ClientSequence: 1,
	}); !errors.Is(err, ErrDuplicateClientEvent) {
		t.Fatalf("duplicate err = %v, want %v", err, ErrDuplicateClientEvent)
	}
	if !state.players["guest"].Ready {
		t.Fatal("duplicate event changed readiness")
	}

	envelope, err := state.Apply(Event{
		Type:           EventPresence,
		PlayerID:       "guest",
		AFK:            true,
		ClientSequence: 2,
	})
	if err != nil {
		t.Fatalf("set AFK presence: %v", err)
	}
	if !playerView(t, envelope.State, "guest").AFK {
		t.Fatal("AFK presence was not published")
	}
}

func TestActorReplacesPriorConnectionAndRejectsStaleDispatch(t *testing.T) {
	actor := newActor("replacement-room", 50*time.Millisecond, DefaultPhaseDurations(), nil)
	ctx := context.Background()
	if _, err := actor.Dispatch(ctx, Event{
		Type:       EventJoin,
		PlayerID:   "owner",
		PlayerName: "Owner",
	}); err != nil {
		t.Fatalf("join owner: %v", err)
	}

	oldEvents, oldID, oldDisconnect, err := actor.SubscribeConnection(ctx, "owner")
	if err != nil {
		t.Fatalf("subscribe old connection: %v", err)
	}
	initial := <-oldEvents
	private, ok := initial.Private.(*PrivatePlayerView)
	if !ok || private.ReconnectToken == "" {
		t.Fatalf("old private view = %#v", initial.Private)
	}

	if _, err := actor.Dispatch(ctx, Event{
		Type:           EventJoin,
		PlayerID:       "owner",
		PlayerName:     "Owner New",
		ReconnectToken: private.ReconnectToken,
	}); err != nil {
		t.Fatalf("join replacement: %v", err)
	}
	newEvents, newID, newDisconnect, err := actor.SubscribeConnection(ctx, "owner")
	if err != nil {
		t.Fatalf("subscribe replacement: %v", err)
	}

	terminal, ok := <-oldEvents
	if !ok || terminal.Type != "error" || terminal.Error != ErrConnectionReplaced.Error() {
		t.Fatalf("old terminal envelope = %#v, open=%t", terminal, ok)
	}
	if _, ok := <-oldEvents; ok {
		t.Fatal("old connection remained subscribed")
	}
	if _, err := actor.DispatchFrom(ctx, oldID, Event{
		Type:     EventSetRoomLocked,
		PlayerID: "owner",
		Locked:   true,
	}); !errors.Is(err, ErrConnectionReplaced) {
		t.Fatalf("stale dispatch err = %v, want %v", err, ErrConnectionReplaced)
	}
	if _, err := actor.DispatchFrom(ctx, newID, Event{
		Type:     EventSetRoomLocked,
		PlayerID: "owner",
		Locked:   true,
	}); err != nil {
		t.Fatalf("replacement dispatch: %v", err)
	}

	oldDisconnect(false)
	state := readActorStateUntil(t, newEvents, func(state *Snapshot) bool {
		return state.Locked && len(state.Players) == 1
	})
	if !state.Players[0].Connected {
		t.Fatal("stale disconnect marked replacement offline")
	}

	newDisconnect(true)
	eventually(t, 500*time.Millisecond, actor.Closed)
}

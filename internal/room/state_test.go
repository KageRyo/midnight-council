package room

import (
	"errors"
	"strings"
	"testing"
)

func TestStateJoinReadyStart(t *testing.T) {
	state := NewState("room-1")

	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"}); err != nil {
		t.Fatalf("join owner: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"}); err != nil {
		t.Fatalf("join guest: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventReady, PlayerID: "guest", Ready: true}); err != nil {
		t.Fatalf("ready guest: %v", err)
	}
	envelope, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("start game: %v", err)
	}

	if envelope.State.Phase != PhaseRoleAssignment {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseRoleAssignment)
	}
}

func TestStateRejectsStartFromNonOwner(t *testing.T) {
	state := readyRoom(t)

	_, err := state.Apply(Event{Type: EventStartGame, PlayerID: "guest"})
	if !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("err = %v, want %v", err, ErrOwnerOnly)
	}
}

func TestStateRejectsStartUntilPlayersReady(t *testing.T) {
	state := NewState("room-1")
	_, _ = state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"})
	_, _ = state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"})

	_, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"})
	if !errors.Is(err, ErrPlayersNotReady) {
		t.Fatalf("err = %v, want %v", err, ErrPlayersNotReady)
	}
}

func TestStateTrimsChat(t *testing.T) {
	state := readyRoom(t)

	envelope, err := state.Apply(Event{
		Type:     EventChat,
		PlayerID: "guest",
		Message:  "  hello table  ",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if envelope.Chat.Message != "hello table" {
		t.Fatalf("message = %q", envelope.Chat.Message)
	}
}

func TestStateRejectsLongChat(t *testing.T) {
	state := readyRoom(t)

	_, err := state.Apply(Event{
		Type:     EventChat,
		PlayerID: "guest",
		Message:  strings.Repeat("x", MaxChatBytes+1),
	})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("err = %v, want %v", err, ErrMessageTooLong)
	}
}

func readyRoom(t *testing.T) *State {
	t.Helper()

	state := NewState("room-1")
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"}); err != nil {
		t.Fatalf("join owner: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"}); err != nil {
		t.Fatalf("join guest: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventReady, PlayerID: "guest", Ready: true}); err != nil {
		t.Fatalf("ready guest: %v", err)
	}
	return state
}

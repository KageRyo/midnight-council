package room

import (
	"errors"
	"testing"
)

func TestRoomLockBlocksNewSeatsButAllowsReconnect(t *testing.T) {
	state := NewState("locked-room")
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"}); err != nil {
		t.Fatalf("join owner: %v", err)
	}
	token := state.players["owner"].ReconnectToken

	envelope, err := state.Apply(Event{Type: EventSetRoomLocked, PlayerID: "owner", Locked: true})
	if err != nil {
		t.Fatalf("lock room: %v", err)
	}
	if !envelope.State.Locked {
		t.Fatal("snapshot did not publish locked room")
	}

	for _, event := range []Event{
		{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"},
		{Type: EventJoin, PlayerID: "spectator", PlayerName: "Spectator", Spectator: true},
	} {
		if _, err := state.Apply(event); !errors.Is(err, ErrRoomLocked) {
			t.Fatalf("join err = %v, want %v", err, ErrRoomLocked)
		}
	}

	if _, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "owner",
		PlayerName:     "Owner Again",
		ReconnectToken: token,
	}); err != nil {
		t.Fatalf("reconnect owner to locked room: %v", err)
	}
}

func TestSpectatorReconnectRequiresMatchingTokenAndType(t *testing.T) {
	state := NewState("spectator-reconnect")
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"}); err != nil {
		t.Fatalf("join owner: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	}); err != nil {
		t.Fatalf("join spectator: %v", err)
	}
	token := state.spectators["spectator"].ReconnectToken

	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	}); !errors.Is(err, ErrReconnectTokenRequired) {
		t.Fatalf("missing token err = %v, want %v", err, ErrReconnectTokenRequired)
	}
	if _, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "spectator",
		PlayerName:     "Spectator",
		ReconnectToken: token,
	}); !errors.Is(err, ErrParticipantTypeMismatch) {
		t.Fatalf("participant type err = %v, want %v", err, ErrParticipantTypeMismatch)
	}
	if _, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "spectator",
		PlayerName:     "Spectator",
		ReconnectToken: "wrong",
		Spectator:      true,
	}); !errors.Is(err, ErrInvalidReconnectToken) {
		t.Fatalf("invalid token err = %v, want %v", err, ErrInvalidReconnectToken)
	}
	if _, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "spectator",
		PlayerName:     "Spectator Again",
		ReconnectToken: token,
		Spectator:      true,
	}); err != nil {
		t.Fatalf("reconnect spectator: %v", err)
	}

	private := state.PrivateView("spectator")
	if private == nil || !private.Spectator || private.ReconnectToken != token {
		t.Fatalf("spectator private reconnect view = %#v", private)
	}
}

func TestPlayerLimitAppliesOnlyToSeatedPlayers(t *testing.T) {
	state := NewState("limited-room")
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"}); err != nil {
		t.Fatalf("join owner: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: 2}); err != nil {
		t.Fatalf("set player limit: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"}); err != nil {
		t.Fatalf("join guest: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "extra", PlayerName: "Extra"}); !errors.Is(err, ErrRoomFull) {
		t.Fatalf("extra player err = %v, want %v", err, ErrRoomFull)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "spectator", PlayerName: "Spectator", Spectator: true}); err != nil {
		t.Fatalf("spectator should not consume player limit: %v", err)
	}

	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: 1}); !errors.Is(err, ErrPlayerLimitOutOfRange) {
		t.Fatalf("small limit err = %v, want %v", err, ErrPlayerLimitOutOfRange)
	}
	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: MaxPlayersAllowed + 1}); !errors.Is(err, ErrPlayerLimitOutOfRange) {
		t.Fatalf("large limit err = %v, want %v", err, ErrPlayerLimitOutOfRange)
	}

	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: 3}); err != nil {
		t.Fatalf("raise player limit: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "third", PlayerName: "Third"}); err != nil {
		t.Fatalf("join third: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: 2}); !errors.Is(err, ErrPlayerLimitBelowCurrent) {
		t.Fatalf("below occupancy err = %v, want %v", err, ErrPlayerLimitBelowCurrent)
	}
}

func TestSpectatorCanObserveActiveGameButCannotActOrChat(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2"})
	if _, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"}); err != nil {
		t.Fatalf("start game: %v", err)
	}

	envelope, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	})
	if err != nil {
		t.Fatalf("join spectator during game: %v", err)
	}
	if len(envelope.State.Players) != 3 || len(envelope.State.Spectators) != 1 {
		t.Fatalf("players/spectators = %d/%d, want 3/1", len(envelope.State.Players), len(envelope.State.Spectators))
	}
	private := state.PrivateView("spectator")
	if private == nil || !private.Spectator || private.ReconnectToken == "" || len(private.Available) != 0 {
		t.Fatalf("spectator private view = %#v", private)
	}

	if _, err := state.Apply(Event{Type: EventChat, PlayerID: "spectator", Message: "spoiler"}); !errors.Is(err, ErrSpectatorCannotChat) {
		t.Fatalf("active spectator chat err = %v, want %v", err, ErrSpectatorCannotChat)
	}
	if _, err := state.Apply(Event{Type: EventReady, PlayerID: "spectator", Ready: true}); !errors.Is(err, ErrRoomNotWaiting) {
		t.Fatalf("spectator ready err = %v, want %v", err, ErrRoomNotWaiting)
	}

	if _, err := state.Apply(Event{Type: EventReturnToWaiting, PlayerID: "owner"}); err != nil {
		t.Fatalf("return to waiting: %v", err)
	}
	chat, err := state.Apply(Event{Type: EventChat, PlayerID: "spectator", Message: "hello"})
	if err != nil {
		t.Fatalf("waiting spectator chat: %v", err)
	}
	if chat.Chat == nil || chat.Chat.Name != "Spectator" {
		t.Fatalf("spectator chat = %#v", chat.Chat)
	}
}

func TestOwnerCanTransferAndDisconnectTransfersAutomatically(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2"})

	if _, err := state.Apply(Event{Type: EventTransferOwner, PlayerID: "owner", TargetID: "guest-1"}); err != nil {
		t.Fatalf("transfer owner: %v", err)
	}
	if state.ownerID != "guest-1" {
		t.Fatalf("owner = %s, want guest-1", state.ownerID)
	}
	if _, err := state.Apply(Event{Type: EventTransferOwner, PlayerID: "owner", TargetID: "guest-2"}); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("old owner transfer err = %v, want %v", err, ErrOwnerOnly)
	}
	if _, err := state.Apply(Event{Type: EventReady, PlayerID: "owner", Ready: true}); err != nil {
		t.Fatalf("ready old owner: %v", err)
	}

	if _, err := state.Apply(Event{Type: EventStartGame, PlayerID: "guest-1"}); err != nil {
		t.Fatalf("start game: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventLeave, PlayerID: "guest-1"}); err != nil {
		t.Fatalf("owner leave: %v", err)
	}
	if state.ownerID != "guest-2" {
		t.Fatalf("automatic owner = %s, want guest-2", state.ownerID)
	}
}

func TestOwnerCanKickPlayersAndSpectatorsOnlyOutsideActiveGame(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2"})
	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	}); err != nil {
		t.Fatalf("join spectator: %v", err)
	}

	if _, err := state.Apply(Event{Type: EventKickParticipant, PlayerID: "guest-1", TargetID: "guest-2"}); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("non-owner kick err = %v, want %v", err, ErrOwnerOnly)
	}
	if _, err := state.Apply(Event{Type: EventKickParticipant, PlayerID: "owner", TargetID: "owner"}); !errors.Is(err, ErrCannotKickOwner) {
		t.Fatalf("kick owner err = %v, want %v", err, ErrCannotKickOwner)
	}
	if _, err := state.Apply(Event{Type: EventKickParticipant, PlayerID: "owner", TargetID: "guest-2"}); err != nil {
		t.Fatalf("kick player: %v", err)
	}
	if _, ok := state.players["guest-2"]; ok {
		t.Fatal("kicked player remains in room")
	}
	if _, err := state.Apply(Event{Type: EventKickParticipant, PlayerID: "owner", TargetID: "spectator"}); err != nil {
		t.Fatalf("kick spectator: %v", err)
	}
	if _, ok := state.spectators["spectator"]; ok {
		t.Fatal("kicked spectator remains in room")
	}

	if _, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"}); err != nil {
		t.Fatalf("start game: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventKickParticipant, PlayerID: "owner", TargetID: "guest-1"}); !errors.Is(err, ErrKickNotAllowed) {
		t.Fatalf("active kick err = %v, want %v", err, ErrKickNotAllowed)
	}
}

func TestReturnToWaitingResetsGameAndStartsRematch(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2"})
	if _, err := state.Apply(Event{Type: EventSetPlayerLimit, PlayerID: "owner", MaxPlayers: 4}); err != nil {
		t.Fatalf("set player limit: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventSetRoomLocked, PlayerID: "owner", Locked: true}); err != nil {
		t.Fatalf("lock room: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"}); err != nil {
		t.Fatalf("start game: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	}); !errors.Is(err, ErrRoomLocked) {
		t.Fatalf("locked spectator join err = %v, want %v", err, ErrRoomLocked)
	}
	if _, err := state.Apply(Event{Type: EventSetRoomLocked, PlayerID: "owner", Locked: false}); err != nil {
		t.Fatalf("unlock room: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator",
		PlayerName: "Spectator",
		Spectator:  true,
	}); err != nil {
		t.Fatalf("join spectator: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:       EventJoin,
		PlayerID:   "spectator-away",
		PlayerName: "Spectator Away",
		Spectator:  true,
	}); err != nil {
		t.Fatalf("join second spectator: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventLeave, PlayerID: "spectator-away"}); err != nil {
		t.Fatalf("disconnect spectator: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventLeave, PlayerID: "guest-2"}); err != nil {
		t.Fatalf("disconnect guest: %v", err)
	}

	reset, err := state.Apply(Event{Type: EventReturnToWaiting, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("return to waiting: %v", err)
	}
	if reset.State.Phase != PhaseWaiting || reset.State.Round != 0 || reset.State.PhaseDeadline != nil {
		t.Fatalf("reset phase/round/deadline = %s/%d/%v", reset.State.Phase, reset.State.Round, reset.State.PhaseDeadline)
	}
	if reset.State.Result != nil || len(reset.State.Log) != 0 {
		t.Fatalf("reset result/log = %#v/%#v", reset.State.Result, reset.State.Log)
	}
	if reset.State.Locked || reset.State.MaxPlayers != 4 {
		t.Fatalf("room options after reset locked/max = %t/%d, want false/4", reset.State.Locked, reset.State.MaxPlayers)
	}
	if _, ok := state.players["guest-2"]; ok {
		t.Fatal("disconnected player survived room reset")
	}
	for _, player := range state.players {
		if player.Ready || player.Role != "" || !player.Alive || player.ShooterUsed {
			t.Fatalf("player was not reset: %#v", player)
		}
	}
	if _, ok := state.spectators["spectator"]; !ok {
		t.Fatal("connected spectator was removed during reset")
	}
	if _, ok := state.spectators["spectator-away"]; ok {
		t.Fatal("disconnected spectator survived room reset")
	}

	if _, err := state.Apply(Event{Type: EventReady, PlayerID: "guest-1", Ready: true}); err != nil {
		t.Fatalf("ready for rematch: %v", err)
	}
	rematch, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("start rematch: %v", err)
	}
	if rematch.State.Phase != PhaseNight || rematch.State.Round != 1 {
		t.Fatalf("rematch phase/round = %s/%d, want NIGHT/1", rematch.State.Phase, rematch.State.Round)
	}
}

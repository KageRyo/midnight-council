package room

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeParticipantIdentity(t *testing.T) {
	tests := []struct {
		name       string
		playerID   string
		playerName string
		wantID     string
		wantName   string
		wantError  error
	}{
		{name: "trims valid values", playerID: " player-1 ", playerName: " 午夜議會 ", wantID: "player-1", wantName: "午夜議會"},
		{name: "rejects an invalid identifier", playerID: "player/id", playerName: "Player", wantError: ErrInvalidPlayerID},
		{name: "rejects an overlong identifier", playerID: strings.Repeat("a", MaxPlayerIDBytes+1), playerName: "Player", wantError: ErrInvalidPlayerID},
		{name: "rejects an overlong rune name", playerID: "player", playerName: strings.Repeat("議", MaxPlayerNameRunes+1), wantError: ErrPlayerNameTooLong},
		{name: "rejects an overlong byte name", playerID: "player", playerName: strings.Repeat("界", MaxPlayerNameBytes/len("界")+1), wantError: ErrPlayerNameTooLong},
		{name: "rejects controls", playerID: "player", playerName: "Player\nTwo", wantError: ErrInvalidPlayerName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			playerID, playerName, err := NormalizeParticipantIdentity(test.playerID, test.playerName)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("error = %v, want %v", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize identity: %v", err)
			}
			if playerID != test.wantID || playerName != test.wantName {
				t.Fatalf("identity = %q/%q, want %q/%q", playerID, playerName, test.wantID, test.wantName)
			}
		})
	}
}

func TestStateLimitsSpectators(t *testing.T) {
	state := newStateWithLimits("room", DefaultPhaseDurations(), DefaultRuleSet(), 1)
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "first", PlayerName: "First", Spectator: true}); err != nil {
		t.Fatalf("join first spectator: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventJoin, PlayerID: "second", PlayerName: "Second", Spectator: true}); !errors.Is(err, ErrSpectatorLimitReached) {
		t.Fatalf("second spectator error = %v, want %v", err, ErrSpectatorLimitReached)
	}
	if got := state.Snapshot().MaxSpectators; got != 1 {
		t.Fatalf("max spectators = %d, want 1", got)
	}
}

package protocol

import (
	"strings"
	"testing"

	"midnight-council/internal/room"
)

func TestDecodeClientEventRejectsUnknownFields(t *testing.T) {
	_, err := DecodeClientEvent(strings.NewReader(`{"type":"chat","message":"hi","extra":true}`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDecodeClientEventRequiresReadyValue(t *testing.T) {
	_, err := DecodeClientEvent(strings.NewReader(`{"type":"ready"}`))
	if err == nil {
		t.Fatal("expected missing ready error")
	}
}

func TestDecodeClientEventRequiresTargetForNightAction(t *testing.T) {
	_, err := DecodeClientEvent(strings.NewReader(`{"type":"night_action"}`))
	if err == nil {
		t.Fatal("expected missing target_id error")
	}
}

func TestDecodeClientEventAllowsVoteAbstain(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{"type":"vote"}`))
	if err != nil {
		t.Fatalf("decode vote abstain: %v", err)
	}

	roomEvent := event.RoomEvent("p1", "Player")
	if roomEvent.Type != room.EventVote {
		t.Fatalf("type = %s, want %s", roomEvent.Type, room.EventVote)
	}
	if roomEvent.TargetID != "" {
		t.Fatalf("target id = %q, want empty", roomEvent.TargetID)
	}
}

func TestDecodeClientEventMapsChatToRoomEvent(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{"type":"chat","message":" hello "}`))
	if err != nil {
		t.Fatalf("decode chat: %v", err)
	}

	roomEvent := event.RoomEvent("p1", "Player")
	if roomEvent.Type != room.EventChat {
		t.Fatalf("type = %s, want %s", roomEvent.Type, room.EventChat)
	}
	if roomEvent.Message != "hello" {
		t.Fatalf("message = %q, want trimmed hello", roomEvent.Message)
	}
}

func TestDecodeClientEventRejectsExtraShapeFields(t *testing.T) {
	_, err := DecodeClientEvent(strings.NewReader(`{"type":"start_game","target_id":"p2"}`))
	if err == nil {
		t.Fatal("expected shape validation error")
	}
}

func TestDecodeClientEventMapsRoomLifecycleEvents(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		eventType  room.EventType
		targetID   string
		locked     bool
		maxPlayers int
	}{
		{
			name:      "transfer owner",
			payload:   `{"type":"transfer_owner","target_id":"p2"}`,
			eventType: room.EventTransferOwner,
			targetID:  "p2",
		},
		{
			name:      "kick participant",
			payload:   `{"type":"kick_participant","target_id":"p2"}`,
			eventType: room.EventKickParticipant,
			targetID:  "p2",
		},
		{
			name:      "lock room",
			payload:   `{"type":"set_room_locked","locked":true}`,
			eventType: room.EventSetRoomLocked,
			locked:    true,
		},
		{
			name:       "set player limit",
			payload:    `{"type":"set_player_limit","max_players":8}`,
			eventType:  room.EventSetPlayerLimit,
			maxPlayers: 8,
		},
		{
			name:      "return to waiting",
			payload:   `{"type":"return_to_waiting"}`,
			eventType: room.EventReturnToWaiting,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := DecodeClientEvent(strings.NewReader(test.payload))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			roomEvent := event.RoomEvent("p1", "Player")
			if roomEvent.Type != test.eventType || roomEvent.TargetID != test.targetID || roomEvent.Locked != test.locked || roomEvent.MaxPlayers != test.maxPlayers {
				t.Fatalf("room event = %#v", roomEvent)
			}
		})
	}
}

func TestDecodeClientEventRequiresRoomLifecycleFields(t *testing.T) {
	for _, payload := range []string{
		`{"type":"transfer_owner"}`,
		`{"type":"kick_participant"}`,
		`{"type":"set_room_locked"}`,
		`{"type":"set_player_limit"}`,
	} {
		if _, err := DecodeClientEvent(strings.NewReader(payload)); err == nil {
			t.Fatalf("payload %s should be rejected", payload)
		}
	}
}

func TestDecodeClientEventMapsGameSettings(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{
		"type":"set_game_settings",
		"night_duration":"45s",
		"day_discussion_duration":"2m",
		"day_voting_duration":"30s",
		"last_words_duration":"20s",
		"minimum_players":4,
		"reveal_roles_on_death":true,
		"killers":1,
		"detectives":1,
		"doctors":0,
		"escorts":1,
		"shooters":1
	}`))
	if err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	roomEvent := event.RoomEvent("owner", "Owner")
	settings := roomEvent.GameSettings
	if roomEvent.Type != room.EventSetGameSettings ||
		settings.Preset != room.GamePresetCustom ||
		settings.NightDuration != "45s" ||
		settings.DayDiscussionDuration != "2m" ||
		settings.DayVotingDuration != "30s" ||
		settings.LastWordsDuration != "20s" ||
		settings.MinimumPlayers != 4 ||
		!settings.RevealRolesOnDeath ||
		settings.Roles != (room.RoleConfiguration{Killers: 1, Detectives: 1, Escorts: 1, Shooters: 1}) {
		t.Fatalf("room settings event = %#v", roomEvent)
	}
}

func TestDecodeClientEventMapsGamePreset(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{"type":"set_game_preset","preset":"QUICK"}`))
	if err != nil {
		t.Fatalf("decode preset: %v", err)
	}
	roomEvent := event.RoomEvent("owner", "Owner")
	if roomEvent.Type != room.EventSetGamePreset || roomEvent.GamePreset != room.GamePresetQuick {
		t.Fatalf("room preset event = %#v", roomEvent)
	}
}

func TestDecodeClientEventMapsAdvancedGamePreset(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{"type":"set_game_preset","preset":"ADVANCED"}`))
	if err != nil {
		t.Fatalf("decode preset: %v", err)
	}
	if event.RoomEvent("owner", "Owner").GamePreset != room.GamePresetAdvanced {
		t.Fatalf("preset = %s, want %s", event.Preset, room.GamePresetAdvanced)
	}
}

func TestDecodeClientEventRequiresCompleteGameSettings(t *testing.T) {
	for _, payload := range []string{
		`{"type":"set_game_settings","night_duration":"45s"}`,
		`{"type":"set_game_preset"}`,
		`{"type":"set_game_preset","preset":"CUSTOM"}`,
	} {
		if _, err := DecodeClientEvent(strings.NewReader(payload)); err == nil {
			t.Fatalf("payload %s should be rejected", payload)
		}
	}
}

func TestDecodeClientEventRejectsGameSettingFieldsOnOtherEvents(t *testing.T) {
	if _, err := DecodeClientEvent(strings.NewReader(`{"type":"start_game","minimum_players":4}`)); err == nil {
		t.Fatal("start_game should reject minimum_players")
	}
}

func TestDecodeClientEventMapsSequenceAndPresence(t *testing.T) {
	event, err := DecodeClientEvent(strings.NewReader(`{"type":"presence","sequence":42,"afk":true}`))
	if err != nil {
		t.Fatalf("decode presence: %v", err)
	}
	roomEvent := event.RoomEvent("player", "Player")
	if roomEvent.Type != room.EventPresence || roomEvent.ClientSequence != 42 || !roomEvent.AFK {
		t.Fatalf("room presence event = %#v", roomEvent)
	}
}

func TestDecodeClientEventRejectsInvalidSequenceAndPresenceShape(t *testing.T) {
	for _, payload := range []string{
		`{"type":"ready","sequence":0,"ready":true}`,
		`{"type":"ready","sequence":9007199254740992,"ready":true}`,
		`{"type":"presence","sequence":1}`,
		`{"type":"start_game","afk":true}`,
	} {
		if _, err := DecodeClientEvent(strings.NewReader(payload)); err == nil {
			t.Fatalf("payload %s should be rejected", payload)
		}
	}
}

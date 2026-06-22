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

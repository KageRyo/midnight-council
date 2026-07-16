package moderation

import (
	"context"
	"testing"
)

func TestAllowAllChatPolicyAllowsMessageUnchanged(t *testing.T) {
	policy := AllowAllChat{}
	request := ChatRequest{
		RoomID:     "room-1",
		PlayerID:   "player-1",
		PlayerName: "Player",
		Message:    "hello",
	}

	decision, err := policy.ReviewChat(context.Background(), request)
	if err != nil {
		t.Fatalf("review chat: %v", err)
	}
	if decision.Action != ChatAllow {
		t.Fatalf("action = %s, want %s", decision.Action, ChatAllow)
	}
	if decision.Replacement != "" || decision.Reason != "" {
		t.Fatalf("unexpected allow decision fields: %#v", decision)
	}
}

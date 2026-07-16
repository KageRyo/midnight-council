package ws

import (
	"context"
	"errors"
	"strings"
	"testing"

	"midnight-council/internal/moderation"
	"midnight-council/internal/room"
)

func TestModerateChatAppliesPolicyDecision(t *testing.T) {
	request := moderation.ChatRequest{
		RoomID:     "room-1",
		PlayerID:   "player-1",
		PlayerName: "Player",
		Message:    "original",
	}
	tests := []struct {
		name            string
		decision        moderation.ChatDecision
		policyError     error
		wantMessage     string
		wantPublicError string
		wantInternalErr bool
	}{
		{
			name:        "allow preserves original",
			decision:    moderation.ChatDecision{Action: moderation.ChatAllow},
			wantMessage: "original",
		},
		{
			name:            "reject uses policy reason",
			decision:        moderation.ChatDecision{Action: moderation.ChatReject, Reason: "blocked by policy"},
			wantPublicError: "blocked by policy",
		},
		{
			name:            "reject uses default reason",
			decision:        moderation.ChatDecision{Action: moderation.ChatReject},
			wantPublicError: defaultChatRejectionError,
		},
		{
			name:        "replace trims replacement",
			decision:    moderation.ChatDecision{Action: moderation.ChatReplace, Replacement: " filtered "},
			wantMessage: "filtered",
		},
		{
			name:            "policy error fails closed",
			policyError:     errors.New("provider unavailable"),
			wantPublicError: chatModerationUnavailableError,
			wantInternalErr: true,
		},
		{
			name: "empty replacement fails closed",
			decision: moderation.ChatDecision{
				Action:      moderation.ChatReplace,
				Replacement: " ",
			},
			wantPublicError: chatModerationUnavailableError,
			wantInternalErr: true,
		},
		{
			name: "oversized replacement fails closed",
			decision: moderation.ChatDecision{
				Action:      moderation.ChatReplace,
				Replacement: strings.Repeat("x", room.MaxChatBytes+1),
			},
			wantPublicError: chatModerationUnavailableError,
			wantInternalErr: true,
		},
		{
			name:            "unknown action fails closed",
			decision:        moderation.ChatDecision{Action: "unknown"},
			wantPublicError: chatModerationUnavailableError,
			wantInternalErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := moderation.ChatPolicyFunc(func(context.Context, moderation.ChatRequest) (moderation.ChatDecision, error) {
				return test.decision, test.policyError
			})

			message, publicError, internalErr := moderateChat(context.Background(), policy, request)
			if message != test.wantMessage {
				t.Fatalf("message = %q, want %q", message, test.wantMessage)
			}
			if publicError != test.wantPublicError {
				t.Fatalf("public error = %q, want %q", publicError, test.wantPublicError)
			}
			if (internalErr != nil) != test.wantInternalErr {
				t.Fatalf("internal error = %v, want present %t", internalErr, test.wantInternalErr)
			}
		})
	}
}

func TestNewHandlerRejectsNilChatPolicy(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewHandler did not reject nil chat policy")
		}
	}()

	NewHandler(room.NewHub(), WithChatPolicy(nil))
}

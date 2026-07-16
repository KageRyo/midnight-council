package ws

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"midnight-council/internal/moderation"
	"midnight-council/internal/room"
)

const (
	defaultChatRejectionError      = "chat message rejected by moderation"
	chatModerationUnavailableError = "chat moderation unavailable; retry later"
)

func moderateChat(ctx context.Context, policy moderation.ChatPolicy, request moderation.ChatRequest) (string, string, error) {
	decision, err := policy.ReviewChat(ctx, request)
	if err != nil {
		return "", chatModerationUnavailableError, fmt.Errorf("review chat: %w", err)
	}

	switch decision.Action {
	case moderation.ChatAllow:
		return request.Message, "", nil
	case moderation.ChatReject:
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			reason = defaultChatRejectionError
		}
		return "", reason, nil
	case moderation.ChatReplace:
		replacement := strings.TrimSpace(decision.Replacement)
		switch {
		case replacement == "":
			return "", chatModerationUnavailableError, errors.New("chat moderation replacement is empty")
		case len([]byte(replacement)) > room.MaxChatBytes:
			return "", chatModerationUnavailableError, room.ErrMessageTooLong
		default:
			return replacement, "", nil
		}
	default:
		return "", chatModerationUnavailableError, fmt.Errorf("unsupported chat moderation action %q", decision.Action)
	}
}

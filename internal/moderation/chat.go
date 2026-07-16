package moderation

import "context"

type ChatAction string

const (
	ChatAllow   ChatAction = "allow"
	ChatReject  ChatAction = "reject"
	ChatReplace ChatAction = "replace"
)

type ChatRequest struct {
	RoomID     string
	PlayerID   string
	PlayerName string
	Message    string
}

type ChatDecision struct {
	Action      ChatAction
	Replacement string
	Reason      string
}

type ChatPolicy interface {
	ReviewChat(context.Context, ChatRequest) (ChatDecision, error)
}

type ChatPolicyFunc func(context.Context, ChatRequest) (ChatDecision, error)

func (f ChatPolicyFunc) ReviewChat(ctx context.Context, request ChatRequest) (ChatDecision, error) {
	return f(ctx, request)
}

type AllowAllChat struct{}

func (AllowAllChat) ReviewChat(context.Context, ChatRequest) (ChatDecision, error) {
	return ChatDecision{Action: ChatAllow}, nil
}

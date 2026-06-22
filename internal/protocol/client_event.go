package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-playground/validator/v10"

	"midnight-council/internal/room"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

type ClientEvent struct {
	Type     room.EventType `json:"type" validate:"required,oneof=ready start_game chat night_action night_pass start_vote vote shoot"`
	Ready    *bool          `json:"ready,omitempty"`
	Message  string         `json:"message,omitempty"`
	TargetID string         `json:"target_id,omitempty"`
}

func DecodeClientEvent(reader io.Reader) (ClientEvent, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var event ClientEvent
	if err := decoder.Decode(&event); err != nil {
		return ClientEvent{}, fmt.Errorf("invalid client event JSON: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ClientEvent{}, errors.New("invalid client event JSON: multiple JSON values")
		}
		return ClientEvent{}, fmt.Errorf("invalid client event JSON: %w", err)
	}

	if err := validate.Struct(event); err != nil {
		return ClientEvent{}, fmt.Errorf("invalid client event schema: %w", err)
	}
	if err := event.validateShape(); err != nil {
		return ClientEvent{}, err
	}

	return event, nil
}

func (e ClientEvent) RoomEvent(playerID, playerName string) room.Event {
	event := room.Event{
		Type:       e.Type,
		PlayerID:   playerID,
		PlayerName: playerName,
		Message:    strings.TrimSpace(e.Message),
		TargetID:   strings.TrimSpace(e.TargetID),
	}
	if e.Ready != nil {
		event.Ready = *e.Ready
	}
	return event
}

func (e ClientEvent) validateShape() error {
	switch e.Type {
	case room.EventReady:
		if e.Ready == nil {
			return errors.New("ready event requires ready")
		}
		return rejectFields(e, false, true, true)
	case room.EventChat:
		if strings.TrimSpace(e.Message) == "" {
			return errors.New("chat event requires message")
		}
		if len([]byte(strings.TrimSpace(e.Message))) > room.MaxChatBytes {
			return room.ErrMessageTooLong
		}
		return rejectFields(e, true, false, true)
	case room.EventNightAction, room.EventShoot:
		if strings.TrimSpace(e.TargetID) == "" {
			return fmt.Errorf("%s event requires target_id", e.Type)
		}
		return rejectFields(e, true, true, false)
	case room.EventVote:
		return rejectFields(e, true, true, false)
	case room.EventStartGame, room.EventNightPass, room.EventStartVote:
		return rejectFields(e, true, true, true)
	default:
		return fmt.Errorf("unsupported client event type: %s", e.Type)
	}
}

func rejectFields(e ClientEvent, rejectReady, rejectMessage, rejectTarget bool) error {
	if rejectReady && e.Ready != nil {
		return fmt.Errorf("%s event does not accept ready", e.Type)
	}
	if rejectMessage && strings.TrimSpace(e.Message) != "" {
		return fmt.Errorf("%s event does not accept message", e.Type)
	}
	if rejectTarget && strings.TrimSpace(e.TargetID) != "" {
		return fmt.Errorf("%s event does not accept target_id", e.Type)
	}
	return nil
}

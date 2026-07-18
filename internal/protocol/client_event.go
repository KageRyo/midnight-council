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
	Type                  room.EventType  `json:"type" validate:"required,oneof=ready start_game chat night_action night_pass start_vote vote shoot transfer_owner kick_participant set_room_locked set_player_limit set_game_settings set_game_preset presence return_to_waiting"`
	Sequence              *uint64         `json:"sequence,omitempty" validate:"omitempty,gte=1,lte=9007199254740991"`
	Ready                 *bool           `json:"ready,omitempty"`
	Message               string          `json:"message,omitempty"`
	TargetID              string          `json:"target_id,omitempty"`
	Locked                *bool           `json:"locked,omitempty"`
	MaxPlayers            *int            `json:"max_players,omitempty"`
	Preset                room.GamePreset `json:"preset,omitempty"`
	NightDuration         string          `json:"night_duration,omitempty"`
	DayDiscussionDuration string          `json:"day_discussion_duration,omitempty"`
	DayVotingDuration     string          `json:"day_voting_duration,omitempty"`
	MinimumPlayers        *int            `json:"minimum_players,omitempty"`
	RevealRolesOnDeath    *bool           `json:"reveal_roles_on_death,omitempty"`
	Killers               *int            `json:"killers,omitempty"`
	Detectives            *int            `json:"detectives,omitempty"`
	Doctors               *int            `json:"doctors,omitempty"`
	Escorts               *int            `json:"escorts,omitempty"`
	Shooters              *int            `json:"shooters,omitempty"`
	AFK                   *bool           `json:"afk,omitempty"`
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
		Type:           e.Type,
		PlayerID:       playerID,
		PlayerName:     playerName,
		ClientSequence: e.SequenceValue(),
		Message:        strings.TrimSpace(e.Message),
		TargetID:       strings.TrimSpace(e.TargetID),
		GamePreset:     e.Preset,
	}
	if e.Ready != nil {
		event.Ready = *e.Ready
	}
	if e.Locked != nil {
		event.Locked = *e.Locked
	}
	if e.MaxPlayers != nil {
		event.MaxPlayers = *e.MaxPlayers
	}
	if e.AFK != nil {
		event.AFK = *e.AFK
	}
	event.GameSettings = room.GameSettings{
		Preset:                room.GamePresetCustom,
		NightDuration:         strings.TrimSpace(e.NightDuration),
		DayDiscussionDuration: strings.TrimSpace(e.DayDiscussionDuration),
		DayVotingDuration:     strings.TrimSpace(e.DayVotingDuration),
		MinimumPlayers:        intValue(e.MinimumPlayers),
		RevealRolesOnDeath:    boolValue(e.RevealRolesOnDeath),
		Roles: room.RoleConfiguration{
			Killers:    intValue(e.Killers),
			Detectives: intValue(e.Detectives),
			Doctors:    intValue(e.Doctors),
			Escorts:    intValue(e.Escorts),
			Shooters:   intValue(e.Shooters),
		},
	}
	return event
}

func (e ClientEvent) validateShape() error {
	switch e.Type {
	case room.EventReady:
		if e.Ready == nil {
			return errors.New("ready event requires ready")
		}
		return rejectUnexpectedFields(e, fieldReady)
	case room.EventChat:
		if strings.TrimSpace(e.Message) == "" {
			return errors.New("chat event requires message")
		}
		if len([]byte(strings.TrimSpace(e.Message))) > room.MaxChatBytes {
			return room.ErrMessageTooLong
		}
		return rejectUnexpectedFields(e, fieldMessage)
	case room.EventNightAction, room.EventShoot:
		if strings.TrimSpace(e.TargetID) == "" {
			return fmt.Errorf("%s event requires target_id", e.Type)
		}
		return rejectUnexpectedFields(e, fieldTargetID)
	case room.EventVote:
		return rejectUnexpectedFields(e, fieldTargetID)
	case room.EventTransferOwner, room.EventKickParticipant:
		if strings.TrimSpace(e.TargetID) == "" {
			return fmt.Errorf("%s event requires target_id", e.Type)
		}
		return rejectUnexpectedFields(e, fieldTargetID)
	case room.EventSetRoomLocked:
		if e.Locked == nil {
			return errors.New("set_room_locked event requires locked")
		}
		return rejectUnexpectedFields(e, fieldLocked)
	case room.EventSetPlayerLimit:
		if e.MaxPlayers == nil {
			return errors.New("set_player_limit event requires max_players")
		}
		return rejectUnexpectedFields(e, fieldMaxPlayers)
	case room.EventSetGamePreset:
		if e.Preset == "" {
			return errors.New("set_game_preset event requires preset")
		}
		switch e.Preset {
		case room.GamePresetStandard, room.GamePresetQuick, room.GamePresetBeginner, room.GamePresetAdvanced, room.GamePresetMinimal:
		default:
			return room.ErrInvalidGamePreset
		}
		return rejectUnexpectedFields(e, fieldPreset)
	case room.EventSetGameSettings:
		if strings.TrimSpace(e.NightDuration) == "" ||
			strings.TrimSpace(e.DayDiscussionDuration) == "" ||
			strings.TrimSpace(e.DayVotingDuration) == "" ||
			e.MinimumPlayers == nil ||
			e.RevealRolesOnDeath == nil ||
			e.Killers == nil ||
			e.Detectives == nil ||
			e.Doctors == nil ||
			e.Escorts == nil ||
			e.Shooters == nil {
			return errors.New("set_game_settings event requires all game setting fields")
		}
		return rejectUnexpectedFields(
			e,
			fieldNightDuration,
			fieldDayDiscussionDuration,
			fieldDayVotingDuration,
			fieldMinimumPlayers,
			fieldRevealRolesOnDeath,
			fieldKillers,
			fieldDetectives,
			fieldDoctors,
			fieldEscorts,
			fieldShooters,
		)
	case room.EventPresence:
		if e.AFK == nil {
			return errors.New("presence event requires afk")
		}
		return rejectUnexpectedFields(e, fieldAFK)
	case room.EventStartGame, room.EventNightPass, room.EventStartVote, room.EventReturnToWaiting:
		return rejectUnexpectedFields(e)
	default:
		return fmt.Errorf("unsupported client event type: %s", e.Type)
	}
}

type clientField string

const (
	fieldReady                 clientField = "ready"
	fieldMessage               clientField = "message"
	fieldTargetID              clientField = "target_id"
	fieldLocked                clientField = "locked"
	fieldMaxPlayers            clientField = "max_players"
	fieldPreset                clientField = "preset"
	fieldNightDuration         clientField = "night_duration"
	fieldDayDiscussionDuration clientField = "day_discussion_duration"
	fieldDayVotingDuration     clientField = "day_voting_duration"
	fieldMinimumPlayers        clientField = "minimum_players"
	fieldRevealRolesOnDeath    clientField = "reveal_roles_on_death"
	fieldKillers               clientField = "killers"
	fieldDetectives            clientField = "detectives"
	fieldDoctors               clientField = "doctors"
	fieldEscorts               clientField = "escorts"
	fieldShooters              clientField = "shooters"
	fieldSequence              clientField = "sequence"
	fieldAFK                   clientField = "afk"
)

func rejectUnexpectedFields(e ClientEvent, allowedFields ...clientField) error {
	allowed := make(map[clientField]bool, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = true
	}
	allowed[fieldSequence] = true
	provided := []struct {
		field   clientField
		present bool
	}{
		{field: fieldReady, present: e.Ready != nil},
		{field: fieldMessage, present: strings.TrimSpace(e.Message) != ""},
		{field: fieldTargetID, present: strings.TrimSpace(e.TargetID) != ""},
		{field: fieldLocked, present: e.Locked != nil},
		{field: fieldMaxPlayers, present: e.MaxPlayers != nil},
		{field: fieldPreset, present: e.Preset != ""},
		{field: fieldNightDuration, present: strings.TrimSpace(e.NightDuration) != ""},
		{field: fieldDayDiscussionDuration, present: strings.TrimSpace(e.DayDiscussionDuration) != ""},
		{field: fieldDayVotingDuration, present: strings.TrimSpace(e.DayVotingDuration) != ""},
		{field: fieldMinimumPlayers, present: e.MinimumPlayers != nil},
		{field: fieldRevealRolesOnDeath, present: e.RevealRolesOnDeath != nil},
		{field: fieldKillers, present: e.Killers != nil},
		{field: fieldDetectives, present: e.Detectives != nil},
		{field: fieldDoctors, present: e.Doctors != nil},
		{field: fieldEscorts, present: e.Escorts != nil},
		{field: fieldShooters, present: e.Shooters != nil},
		{field: fieldSequence, present: e.Sequence != nil},
		{field: fieldAFK, present: e.AFK != nil},
	}
	for _, candidate := range provided {
		if candidate.present && !allowed[candidate.field] {
			return fmt.Errorf("%s event does not accept %s", e.Type, candidate.field)
		}
	}
	return nil
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func (e ClientEvent) SequenceValue() uint64 {
	if e.Sequence == nil {
		return 0
	}
	return *e.Sequence
}

package room

import (
	"errors"
	"time"
)

const (
	GamePresetStandard GamePreset = "STANDARD"
	GamePresetQuick    GamePreset = "QUICK"
	GamePresetBeginner GamePreset = "BEGINNER"
	GamePresetMinimal  GamePreset = "MINIMAL"
	GamePresetCustom   GamePreset = "CUSTOM"

	MinConfigurablePhaseDuration = time.Second
	MaxConfigurablePhaseDuration = time.Hour
)

var (
	ErrInvalidGamePreset        = errors.New("game preset is invalid")
	ErrInvalidPhaseDuration     = errors.New("phase duration is invalid")
	ErrMinimumPlayersOutOfRange = errors.New("minimum players is out of range")
	ErrMinimumPlayersAboveLimit = errors.New("minimum players cannot exceed the room player limit")
	ErrInvalidRoleConfiguration = errors.New("role configuration is invalid")
	ErrPlayerLimitBelowMinimum  = errors.New("player limit cannot be below the configured minimum players")
)

type GamePreset string

type RoleConfiguration struct {
	Killers    int `json:"killers"`
	Detectives int `json:"detectives"`
	Doctors    int `json:"doctors"`
	Shooters   int `json:"shooters"`
}

type GameSettings struct {
	Preset                GamePreset        `json:"preset"`
	NightDuration         string            `json:"night_duration"`
	DayDiscussionDuration string            `json:"day_discussion_duration"`
	DayVotingDuration     string            `json:"day_voting_duration"`
	MinimumPlayers        int               `json:"minimum_players"`
	RevealRolesOnDeath    bool              `json:"reveal_roles_on_death"`
	Roles                 RoleConfiguration `json:"roles"`
}

func StandardGameSettings(durations PhaseDurations) GameSettings {
	return GameSettings{
		Preset:                GamePresetStandard,
		NightDuration:         durations.Night.String(),
		DayDiscussionDuration: durations.DayDiscussion.String(),
		DayVotingDuration:     durations.DayVoting.String(),
		MinimumPlayers:        MinPlayersToStart,
		Roles: RoleConfiguration{
			Killers:    1,
			Detectives: 1,
			Doctors:    1,
			Shooters:   1,
		},
	}
}

func GameSettingsForPreset(preset GamePreset, standardDurations PhaseDurations) (GameSettings, error) {
	switch preset {
	case GamePresetStandard:
		return StandardGameSettings(standardDurations), nil
	case GamePresetQuick:
		return GameSettings{
			Preset:                GamePresetQuick,
			NightDuration:         "45s",
			DayDiscussionDuration: "2m",
			DayVotingDuration:     "45s",
			MinimumPlayers:        4,
			RevealRolesOnDeath:    true,
			Roles: RoleConfiguration{
				Killers:    1,
				Detectives: 1,
				Doctors:    1,
				Shooters:   1,
			},
		}, nil
	case GamePresetBeginner:
		return GameSettings{
			Preset:                GamePresetBeginner,
			NightDuration:         "2m",
			DayDiscussionDuration: "7m",
			DayVotingDuration:     "1m30s",
			MinimumPlayers:        5,
			RevealRolesOnDeath:    true,
			Roles: RoleConfiguration{
				Killers:    1,
				Detectives: 1,
				Doctors:    1,
				Shooters:   1,
			},
		}, nil
	case GamePresetMinimal:
		return GameSettings{
			Preset:                GamePresetMinimal,
			NightDuration:         "1m",
			DayDiscussionDuration: "2m",
			DayVotingDuration:     "45s",
			MinimumPlayers:        2,
			Roles: RoleConfiguration{
				Killers: 1,
			},
		}, nil
	default:
		return GameSettings{}, ErrInvalidGamePreset
	}
}

func GamePresetCatalog(standardDurations PhaseDurations) []GameSettings {
	presets := make([]GameSettings, 0, 4)
	for _, preset := range []GamePreset{
		GamePresetStandard,
		GamePresetQuick,
		GamePresetBeginner,
		GamePresetMinimal,
	} {
		settings, err := GameSettingsForPreset(preset, standardDurations)
		if err != nil {
			continue
		}
		presets = append(presets, settings)
	}
	return presets
}

func (s GameSettings) Validate(maxPlayers int) (PhaseDurations, error) {
	durations, err := s.phaseDurations()
	if err != nil {
		return PhaseDurations{}, err
	}
	for _, duration := range []time.Duration{
		durations.Night,
		durations.DayDiscussion,
		durations.DayVoting,
	} {
		if duration < MinConfigurablePhaseDuration || duration > MaxConfigurablePhaseDuration {
			return PhaseDurations{}, ErrInvalidPhaseDuration
		}
	}
	if err := s.validateRoomConstraints(maxPlayers); err != nil {
		return PhaseDurations{}, err
	}
	return durations, nil
}

func (s GameSettings) ValidateForStart(maxPlayers int, standardDurations PhaseDurations) (PhaseDurations, error) {
	if s != StandardGameSettings(standardDurations) {
		return s.Validate(maxPlayers)
	}
	durations, err := s.phaseDurations()
	if err != nil {
		return PhaseDurations{}, err
	}
	if err := s.validateRoomConstraints(maxPlayers); err != nil {
		return PhaseDurations{}, err
	}
	return durations, nil
}

func (s GameSettings) validateRoomConstraints(maxPlayers int) error {
	if s.MinimumPlayers < MinPlayersToStart || s.MinimumPlayers > MaxPlayersAllowed {
		return ErrMinimumPlayersOutOfRange
	}
	if s.MinimumPlayers > maxPlayers {
		return ErrMinimumPlayersAboveLimit
	}
	if s.Roles.Killers != 1 ||
		s.Roles.Detectives < 0 || s.Roles.Detectives > 1 ||
		s.Roles.Doctors < 0 || s.Roles.Doctors > 1 ||
		s.Roles.Shooters < 0 || s.Roles.Shooters > 1 {
		return ErrInvalidRoleConfiguration
	}
	return nil
}

func (s GameSettings) phaseDurations() (PhaseDurations, error) {
	night, err := time.ParseDuration(s.NightDuration)
	if err != nil {
		return PhaseDurations{}, ErrInvalidPhaseDuration
	}
	discussion, err := time.ParseDuration(s.DayDiscussionDuration)
	if err != nil {
		return PhaseDurations{}, ErrInvalidPhaseDuration
	}
	voting, err := time.ParseDuration(s.DayVotingDuration)
	if err != nil {
		return PhaseDurations{}, ErrInvalidPhaseDuration
	}
	durations := PhaseDurations{
		Night:         night,
		DayDiscussion: discussion,
		DayVoting:     voting,
	}
	if err := durations.Validate(); err != nil {
		return PhaseDurations{}, ErrInvalidPhaseDuration
	}
	return durations, nil
}

func roleDeckForSettings(playerCount int, roles RoleConfiguration) []Role {
	deck := make([]Role, 0, playerCount)
	deck = append(deck, RoleKiller)
	for _, candidate := range []struct {
		enabled bool
		role    Role
	}{
		{enabled: roles.Detectives == 1, role: RoleDetective},
		{enabled: roles.Doctors == 1, role: RoleDoctor},
		{enabled: roles.Shooters == 1, role: RoleShooter},
	} {
		if candidate.enabled && len(deck) < playerCount-1 {
			deck = append(deck, candidate.role)
		}
	}
	for len(deck) < playerCount {
		deck = append(deck, RoleVillager)
	}
	return deck
}

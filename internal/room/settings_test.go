package room

import (
	"errors"
	"testing"
	"time"
)

func TestGameSettingsValidateRejectsIllegalAndUnbalancedConfigurations(t *testing.T) {
	valid := StandardGameSettings(DefaultPhaseDurations())
	if _, err := valid.Validate(DefaultMaxPlayers); err != nil {
		t.Fatalf("validate standard settings: %v", err)
	}

	tests := []struct {
		name      string
		mutate    func(*GameSettings)
		max       int
		wantError error
	}{
		{
			name: "invalid duration",
			mutate: func(settings *GameSettings) {
				settings.NightDuration = "0s"
			},
			max:       DefaultMaxPlayers,
			wantError: ErrInvalidPhaseDuration,
		},
		{
			name: "minimum above room limit",
			mutate: func(settings *GameSettings) {
				settings.MinimumPlayers = 8
			},
			max:       6,
			wantError: ErrMinimumPlayersAboveLimit,
		},
		{
			name: "missing killer",
			mutate: func(settings *GameSettings) {
				settings.Roles.Killers = 0
			},
			max:       DefaultMaxPlayers,
			wantError: ErrInvalidRoleConfiguration,
		},
		{
			name: "duplicate singleton role",
			mutate: func(settings *GameSettings) {
				settings.Roles.Doctors = 2
			},
			max:       DefaultMaxPlayers,
			wantError: ErrInvalidRoleConfiguration,
		},
		{
			name: "duplicate escort role",
			mutate: func(settings *GameSettings) {
				settings.Roles.Escorts = 2
			},
			max:       DefaultMaxPlayers,
			wantError: ErrInvalidRoleConfiguration,
		},
		{
			name: "multiple killers are unsupported",
			mutate: func(settings *GameSettings) {
				settings.Roles.Killers = 2
			},
			max:       DefaultMaxPlayers,
			wantError: ErrInvalidRoleConfiguration,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := valid
			test.mutate(&settings)
			if _, err := settings.Validate(test.max); !errors.Is(err, test.wantError) {
				t.Fatalf("err = %v, want %v", err, test.wantError)
			}
		})
	}
}

func TestGamePresetCatalogContainsServerAuthoritativePresets(t *testing.T) {
	base := PhaseDurations{
		Night:         75 * time.Second,
		DayDiscussion: 4 * time.Minute,
		DayVoting:     50 * time.Second,
	}
	catalog := GamePresetCatalog(base)
	if len(catalog) != 5 {
		t.Fatalf("preset count = %d, want 5", len(catalog))
	}
	if catalog[0].Preset != GamePresetStandard ||
		catalog[0].NightDuration != "1m15s" ||
		catalog[0].DayDiscussionDuration != "4m0s" ||
		catalog[0].DayVotingDuration != "50s" {
		t.Fatalf("standard preset = %#v", catalog[0])
	}
	for _, settings := range catalog {
		if _, err := settings.Validate(DefaultMaxPlayers); err != nil {
			t.Fatalf("preset %s is invalid: %v", settings.Preset, err)
		}
	}
	advanced := catalog[3]
	if advanced.Preset != GamePresetAdvanced || advanced.MinimumPlayers != 6 || advanced.Roles.Escorts != 1 {
		t.Fatalf("advanced preset = %#v", advanced)
	}
}

func TestServerStandardPresetCanUsePositiveSubsecondTestDurations(t *testing.T) {
	base := PhaseDurations{
		Night:         30 * time.Millisecond,
		DayDiscussion: 40 * time.Millisecond,
		DayVoting:     20 * time.Millisecond,
	}
	settings := StandardGameSettings(base)
	if _, err := settings.Validate(DefaultMaxPlayers); !errors.Is(err, ErrInvalidPhaseDuration) {
		t.Fatalf("custom validation err = %v, want %v", err, ErrInvalidPhaseDuration)
	}
	durations, err := settings.ValidateForStart(DefaultMaxPlayers, base)
	if err != nil {
		t.Fatalf("validate trusted standard: %v", err)
	}
	if durations != base {
		t.Fatalf("durations = %#v, want %#v", durations, base)
	}
}

func TestRoleDeckReservesVillagerAndActivatesSpecialRolesInOrder(t *testing.T) {
	roles := RoleConfiguration{
		Killers:    1,
		Detectives: 1,
		Doctors:    1,
		Shooters:   1,
	}
	tests := []struct {
		players int
		want    []Role
	}{
		{players: 2, want: []Role{RoleKiller, RoleVillager}},
		{players: 3, want: []Role{RoleKiller, RoleDetective, RoleVillager}},
		{players: 4, want: []Role{RoleKiller, RoleDetective, RoleDoctor, RoleVillager}},
		{players: 5, want: []Role{RoleKiller, RoleDetective, RoleDoctor, RoleShooter, RoleVillager}},
	}
	for _, test := range tests {
		got := roleDeckForSettings(test.players, roles)
		if len(got) != len(test.want) {
			t.Fatalf("%d-player deck length = %d, want %d", test.players, len(got), len(test.want))
		}
		for index := range got {
			if got[index] != test.want[index] {
				t.Fatalf("%d-player deck = %#v, want %#v", test.players, got, test.want)
			}
		}
	}
}

func TestAdvancedRoleDeckIncludesEscortBeforeShooter(t *testing.T) {
	roles := RoleConfiguration{
		Killers:    1,
		Detectives: 1,
		Doctors:    1,
		Escorts:    1,
		Shooters:   1,
	}
	want := []Role{RoleKiller, RoleDetective, RoleDoctor, RoleEscort, RoleShooter, RoleVillager}
	got := roleDeckForSettings(6, roles)
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("advanced deck = %#v, want %#v", got, want)
		}
	}
}

func TestOwnerCanUpdateGameSettingsAndReadinessResets(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2", "guest-3"})
	settings := GameSettings{
		NightDuration:         "30s",
		DayDiscussionDuration: "90s",
		DayVotingDuration:     "20s",
		MinimumPlayers:        4,
		RevealRolesOnDeath:    true,
		Roles: RoleConfiguration{
			Killers:    1,
			Detectives: 1,
			Doctors:    0,
			Shooters:   0,
		},
	}

	envelope, err := state.Apply(Event{
		Type:         EventSetGameSettings,
		PlayerID:     "owner",
		GameSettings: settings,
	})
	if err != nil {
		t.Fatalf("set game settings: %v", err)
	}
	if envelope.State.GameSettings.Preset != GamePresetCustom ||
		envelope.State.GameSettings.NightDuration != "30s" ||
		!envelope.State.GameSettings.RevealRolesOnDeath {
		t.Fatalf("published settings = %#v", envelope.State.GameSettings)
	}
	for _, player := range state.players {
		if player.ID != "owner" && player.Ready {
			t.Fatalf("player %s remained ready after settings change", player.ID)
		}
	}
	if _, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"}); !errors.Is(err, ErrPlayersNotReady) {
		t.Fatalf("start err = %v, want %v", err, ErrPlayersNotReady)
	}

	for _, playerID := range []string{"guest-1", "guest-2", "guest-3"} {
		if _, err := state.Apply(Event{Type: EventReady, PlayerID: playerID, Ready: true}); err != nil {
			t.Fatalf("ready %s: %v", playerID, err)
		}
	}
	startedAt := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	started, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner", At: startedAt})
	if err != nil {
		t.Fatalf("start game: %v", err)
	}
	if started.State.PhaseDeadline == nil || !started.State.PhaseDeadline.Equal(startedAt.Add(30*time.Second)) {
		t.Fatalf("night deadline = %v", started.State.PhaseDeadline)
	}

	counts := make(map[Role]int)
	for _, player := range state.players {
		counts[player.Role]++
	}
	if counts[RoleKiller] != 1 || counts[RoleDetective] != 1 || counts[RoleDoctor] != 0 || counts[RoleShooter] != 0 || counts[RoleVillager] != 2 {
		t.Fatalf("assigned role counts = %#v", counts)
	}
}

func TestOwnerCanApplyPresetAndPlayerLimitCannotViolateMinimum(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2", "guest-3", "guest-4"})

	envelope, err := state.Apply(Event{
		Type:       EventSetGamePreset,
		PlayerID:   "owner",
		GamePreset: GamePresetBeginner,
	})
	if err != nil {
		t.Fatalf("apply beginner preset: %v", err)
	}
	if envelope.State.GameSettings.Preset != GamePresetBeginner ||
		envelope.State.GameSettings.MinimumPlayers != 5 ||
		!envelope.State.GameSettings.RevealRolesOnDeath {
		t.Fatalf("beginner settings = %#v", envelope.State.GameSettings)
	}
	if _, err := state.Apply(Event{
		Type:       EventSetPlayerLimit,
		PlayerID:   "owner",
		MaxPlayers: 4,
	}); !errors.Is(err, ErrPlayerLimitBelowCurrent) {
		t.Fatalf("occupied limit err = %v, want %v", err, ErrPlayerLimitBelowCurrent)
	}

	if _, err := state.Apply(Event{
		Type:     EventKickParticipant,
		PlayerID: "owner",
		TargetID: "guest-4",
	}); err != nil {
		t.Fatalf("kick guest: %v", err)
	}
	if _, err := state.Apply(Event{
		Type:       EventSetPlayerLimit,
		PlayerID:   "owner",
		MaxPlayers: 4,
	}); !errors.Is(err, ErrPlayerLimitBelowMinimum) {
		t.Fatalf("minimum limit err = %v, want %v", err, ErrPlayerLimitBelowMinimum)
	}
}

func TestRevealRolesOnDeathPublishesOnlyEliminatedRole(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"killer":  RoleKiller,
		"victim":  RoleVillager,
		"alive-1": RoleVillager,
		"alive-2": RoleVillager,
	})
	state.ownerID = "killer"
	state.gameSettings.RevealRolesOnDeath = true

	envelope, err := state.Apply(Event{
		Type:     EventNightAction,
		PlayerID: "killer",
		TargetID: "victim",
	})
	if err != nil {
		t.Fatalf("kill victim: %v", err)
	}
	if envelope.State.Phase != PhaseDayDiscussion {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayDiscussion)
	}
	if got := playerView(t, envelope.State, "victim").Role; got != RoleVillager {
		t.Fatalf("victim role = %s, want %s", got, RoleVillager)
	}
	for _, playerID := range []string{"killer", "alive-1", "alive-2"} {
		if got := playerView(t, envelope.State, playerID).Role; got != "" {
			t.Fatalf("living player %s leaked role %s", playerID, got)
		}
	}
}

func TestGameSettingsCannotChangeDuringActiveGame(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})
	settings := StandardGameSettings(DefaultPhaseDurations())
	if _, err := state.Apply(Event{
		Type:         EventSetGameSettings,
		PlayerID:     "owner",
		GameSettings: settings,
	}); !errors.Is(err, ErrWrongPhase) {
		t.Fatalf("active settings err = %v, want %v", err, ErrWrongPhase)
	}
}

func TestNonOwnerCannotChangeGameSettings(t *testing.T) {
	state := readyRoom(t)
	settings := StandardGameSettings(DefaultPhaseDurations())
	if _, err := state.Apply(Event{
		Type:         EventSetGameSettings,
		PlayerID:     "guest",
		GameSettings: settings,
	}); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("non-owner settings err = %v, want %v", err, ErrOwnerOnly)
	}
	if _, err := state.Apply(Event{
		Type:       EventSetGamePreset,
		PlayerID:   "guest",
		GamePreset: GamePresetQuick,
	}); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("non-owner preset err = %v, want %v", err, ErrOwnerOnly)
	}
}

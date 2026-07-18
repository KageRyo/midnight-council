package room

import (
	"errors"
	"testing"
)

func TestRuleSetCopiesDefinitionsAndRejectsDuplicateDeckRoles(t *testing.T) {
	action := &NightActionRule{Type: NightActionKill, Priority: 10}
	rules, err := NewRuleSet([]RoleRule{
		{Role: RoleVillager, Faction: FactionCouncil},
		{Role: RoleKiller, Faction: FactionKillers, NightAction: action},
	}, []Role{RoleKiller})
	if err != nil {
		t.Fatalf("new rule set: %v", err)
	}
	action.Priority = 99
	stored, ok := rules.NightActionForRole(RoleKiller)
	if !ok || stored.Priority != 10 {
		t.Fatalf("stored action = %#v, want copied priority 10", stored)
	}
	returned, _ := rules.Role(RoleKiller)
	returned.NightAction.Priority = 50
	stored, _ = rules.NightActionForRole(RoleKiller)
	if stored.Priority != 10 {
		t.Fatalf("mutating returned rule changed registry priority to %d", stored.Priority)
	}

	_, err = NewRuleSet([]RoleRule{
		{Role: RoleVillager, Faction: FactionCouncil},
		{Role: RoleKiller, Faction: FactionKillers},
	}, []Role{RoleKiller, RoleKiller})
	if !errors.Is(err, ErrInvalidRuleSet) {
		t.Fatalf("duplicate deck err = %v, want %v", err, ErrInvalidRuleSet)
	}
}

func TestRuleSetOrdersNightActionsByAbilityThenPlayerID(t *testing.T) {
	actions := map[string]NightAction{
		"killer":    {PlayerID: "killer", Type: NightActionKill},
		"detective": {PlayerID: "detective", Type: NightActionInvestigate},
		"doctor":    {PlayerID: "doctor", Type: NightActionProtect},
		"escort":    {PlayerID: "escort", Type: NightActionBlock},
	}
	ordered := DefaultRuleSet().OrderNightActions(actions)
	want := []NightActionType{
		NightActionBlock,
		NightActionProtect,
		NightActionKill,
		NightActionInvestigate,
	}
	for index := range want {
		if ordered[index].Type != want[index] {
			t.Fatalf("ordered actions = %#v, want types %#v", ordered, want)
		}
	}
}

func TestEscortBlocksEarlierSubmittedKillerBeforeNightResolution(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner":     RoleKiller,
		"detective": RoleDetective,
		"doctor":    RoleDoctor,
		"escort":    RoleEscort,
		"villager":  RoleVillager,
	})
	private := state.PrivateView("escort")
	if private == nil || !private.ActionRequired || len(private.Available) != 2 || private.Available[0] != EventNightAction {
		t.Fatalf("escort private view = %#v", private)
	}

	for _, event := range []Event{
		{Type: EventNightAction, PlayerID: "owner", TargetID: "villager"},
		{Type: EventNightAction, PlayerID: "detective", TargetID: "owner"},
		{Type: EventNightAction, PlayerID: "doctor", TargetID: "detective"},
	} {
		if _, err := state.Apply(event); err != nil {
			t.Fatalf("submit %s action: %v", event.PlayerID, err)
		}
	}
	envelope, err := state.Apply(Event{
		Type:     EventNightAction,
		PlayerID: "escort",
		TargetID: "owner",
	})
	if err != nil {
		t.Fatalf("submit escort action: %v", err)
	}
	if !state.players["villager"].Alive {
		t.Fatal("blocked killer still eliminated the villager")
	}
	if len(state.investigations["detective"]) != 1 || !state.investigations["detective"][0].Killer {
		t.Fatalf("detective result = %#v", state.investigations["detective"])
	}
	if envelope.State.Phase != PhaseDayDiscussion {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayDiscussion)
	}
}

func TestEscortCannotBlockSelf(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner":  RoleEscort,
		"killer": RoleKiller,
	})
	_, err := state.Apply(Event{
		Type:     EventNightAction,
		PlayerID: "owner",
		TargetID: "owner",
	})
	if !errors.Is(err, ErrSelfTargetNotAllowed) {
		t.Fatalf("self-target err = %v, want %v", err, ErrSelfTargetNotAllowed)
	}
}
